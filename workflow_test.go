package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/alphagov/govuk_crawler_worker"
	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/streadway/amqp"
)

var _ = Describe("Workflow", func() {
	Describe("Acknowledging items", func() {
		amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
		redisAddr := util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
		exchangeName := "govuk_crawler_worker-test-workflow-exchange"
		queueName := "govuk_crawler_worker-test-workflow-queue"
		prefix := "govuk_mirror_crawler_workflow_test"

		var (
			err             error
			mirrorRoot      string
			queueManager    *queue.Manager
			queueManagerErr error
			ttlHashSet      *ttl_hash_set.TTLHashSet
			ttlHashSetErr   error
			rootURL         *url.URL
		)

		BeforeEach(func() {
			mirrorRoot = os.Getenv("MIRROR_ROOT")
			if mirrorRoot == "" {
				mirrorRoot, err = ioutil.TempDir("", "workflow_test")
				Expect(err).To(BeNil())
			}

			rootURL, _ = url.Parse("https://www.gov.uk")

			ttlHashSet, ttlHashSetErr = ttl_hash_set.NewTTLHashSet(prefix, redisAddr, time.Hour)
			Expect(ttlHashSetErr).To(BeNil())

			queueManager, queueManagerErr = queue.NewManager(
				amqpAddr,
				exchangeName,
				queueName)

			Expect(queueManagerErr).To(BeNil())
			Expect(queueManager).ToNot(BeNil())

			queueManager.Consumer.HandleChannelClose = func(_ string) {}
			queueManager.Producer.HandleChannelClose = func(_ string) {}
		})

		AfterEach(func() {
			// Consumer must Cancel() or Close() before deleting.
			queueManager.Consumer.Close()
			defer queueManager.Close()

			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(PurgeAllKeys(prefix, redisAddr)).To(BeNil())

			deleted, err := queueManager.Producer.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Producer.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())

			DeleteMirrorFilesFromDisk(mirrorRoot)
		})

		Describe("AcknowledgeItem", func() {
			It("should read from a channel and add URLs to the hash set", func() {
				url := "https://www.gov.uk/foo"

				exists, err := ttlHashSet.Exists(url)
				Expect(err).To(BeNil())
				Expect(exists).To(Equal(false))

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				outbound := make(chan *main.CrawlerMessageItem, 1)

				err = queueManager.Publish("#", "text/plain", url)
				Expect(err).To(BeNil())

				for item := range deliveries {
					outbound <- main.NewCrawlerMessageItem(item, rootURL, []string{})
					item.Ack(false)
					break
				}

				Expect(len(outbound)).To(Equal(1))

				go main.AcknowledgeItem(outbound, ttlHashSet)

				Eventually(outbound).Should(HaveLen(0))
				Eventually(func() bool {
					exists, _ := ttlHashSet.Exists(url)
					return exists
				}).Should(BeTrue())

				// Close the channel to stop the goroutine for AcknowledgeItem.
				close(outbound)
			})
		})

		Describe("CrawlURL", func() {
			var crawler *http_crawler.Crawler

			BeforeEach(func() {
				rootURL, _ = url.Parse("http://127.0.0.1")
				crawler = http_crawler.NewCrawler(rootURL, "0.0.0", nil)
				Expect(crawler).ToNot(BeNil())
			})

			It("crawls a URL and assigns the body", func() {
				outbound := make(chan *main.CrawlerMessageItem, 1)

				body := `<a href="gov.uk">bar</a>`
				server := testServer(http.StatusOK, body)

				deliveryItem := &amqp.Delivery{Body: []byte(server.URL)}
				outbound <- main.NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})

				crawled := main.CrawlURL(ttlHashSet, outbound, crawler, 1, 1)

				Expect((<-crawled).Response.Body[0:24]).To(Equal([]byte(body)))

				server.Close()
				close(outbound)
			})

			It("doesn't crawl an item that has been retried too many times", func() {
				body := `<a href="gov.uk">bar</a>`
				server := testServer(http.StatusInternalServerError, body)

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				crawlChan := main.ReadFromQueue(deliveries, rootURL, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 4

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := main.CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
				Eventually(crawlChan).Should(HaveLen(0))

				Eventually(func() (int, error) {
					return ttlHashSet.Get(server.URL)
				}).Should(Equal(maxRetries))

				Eventually(func() (int, error) {
					queueInfo, err := queueManager.Producer.Channel.QueueInspect(queueManager.QueueName)
					return queueInfo.Messages, err
				}).Should(Equal(0))
				Expect(len(crawled)).To(Equal(0))

				server.Close()
				close(crawlChan)
			})

			It("adds a redirect URL to the TTLHashSet so we don't immediately retry it", func() {
				body := `<a href="gov.uk">bar</a>`
				server := testServer(http.StatusMovedPermanently, body)

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				crawlChan := main.ReadFromQueue(deliveries, rootURL, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 4

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := main.CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
				Eventually(crawlChan).Should(HaveLen(0))

				Eventually(func() (int, error) {
					return ttlHashSet.Get(server.URL)
				}).Should(Equal(main.AlreadyCrawled))

				Eventually(func() (int, error) {
					queueInfo, err := queueManager.Producer.Channel.QueueInspect(queueManager.QueueName)
					return queueInfo.Messages, err
				}).Should(Equal(0))
				Expect(len(crawled)).To(Equal(0))

				server.Close()
				close(crawlChan)
			})

			It("adds a non-HTML URL to the TTLHashSet so we don't immediately retry it", func() {
				body := `I am not HTML. No HTML, see?`
				server := testServer(http.StatusOK, body)

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				crawlChan := main.ReadFromQueue(deliveries, rootURL, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 4

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := main.CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
				Eventually(crawlChan).Should(HaveLen(0))

				Eventually(func() (int, error) {
					return ttlHashSet.Get(server.URL)
				}).Should(Equal(main.AlreadyCrawled))

				Eventually(func() (int, error) {
					queueInfo, err := queueManager.Producer.Channel.QueueInspect(queueManager.QueueName)
					return queueInfo.Messages, err
				}).Should(Equal(0))
				Expect(len(crawled)).To(Equal(0))

				server.Close()
				close(crawlChan)
			})

			It("expects the number of goroutines to run to be a positive integer", func() {
				outbound := make(chan *main.CrawlerMessageItem, 1)

				Expect(func() {
					main.CrawlURL(ttlHashSet, outbound, crawler, 0, 1)
				}).To(Panic())

				Expect(func() {
					main.CrawlURL(ttlHashSet, outbound, crawler, -1, 1)
				}).To(Panic())
			})
		})

		Describe("WriteItemToDisk", func() {
			It("wrote the item to disk", func() {
				url := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(url)}
				item := main.NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})
				item.Response = &http_crawler.CrawlerResponse{
					Body:        []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
					ContentType: http_crawler.HTML,
				}

				outbound := make(chan *main.CrawlerMessageItem, 1)
				extract := main.WriteItemToDisk(mirrorRoot, outbound)

				Expect(len(extract)).To(Equal(0))

				outbound <- item

				Expect(<-extract).To(Equal(item))

				relativeFilePath, _ := item.RelativeFilePath()
				filePath := path.Join(mirrorRoot, relativeFilePath)

				fileContent, err := ioutil.ReadFile(filePath)

				Expect(err).To(BeNil())
				Expect(fileContent).To(Equal(item.Response.Body))

				close(outbound)
			})

			It("doesn't forward the item for extraction if it's not HTML", func() {
				body := []byte(`{"a": 2}`)
				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				err = queueManager.Publish("#", "text/plain", "https://www.gov.uk/extract-some-urls.json")
				Expect(err).To(BeNil())

				rootURL, _ := url.Parse("https://www.gov.uk")
				item := main.NewCrawlerMessageItem((<-deliveries), rootURL, []string{})
				item.Response = &http_crawler.CrawlerResponse{
					Body:        body,
					ContentType: http_crawler.JSON,
				}

				outbound := make(chan *main.CrawlerMessageItem, 1)
				extract := main.WriteItemToDisk(mirrorRoot, outbound)
				Expect(len(extract)).To(Equal(0))

				outbound <- item
				relativeFilePath, _ := item.RelativeFilePath()
				filePath := path.Join(mirrorRoot, relativeFilePath)
				Eventually(func() []byte {
					content, _ := ioutil.ReadFile(filePath)
					return content
				}).Should(Equal(body))
				Expect(len(extract)).To(Equal(0))

				close(outbound)
			})
		})

		Describe("ExtractURLs", func() {
			It("extracts URLs from the HTML body and adds them to a new channel; acknowledging item", func() {
				url := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(url)}
				item := main.NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})
				item.Response = &http_crawler.CrawlerResponse{
					Body: []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
				}

				outbound := make(chan *main.CrawlerMessageItem, 1)
				publish, acknowledge := main.ExtractURLs(outbound)

				Expect(len(publish)).To(Equal(0))
				Expect(len(acknowledge)).To(Equal(0))

				outbound <- item

				Expect(<-publish).To(Equal("https://www.gov.uk/some-url"))
				Expect(<-acknowledge).To(Equal(item))

				close(outbound)
			})
		})

		Describe("PublishURLs", func() {
			It("doesn't publish URLs that have already been crawled", func() {
				url := "https://www.gov.uk/government/organisations"

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				err = ttlHashSet.Incr(url)
				Expect(err).To(BeNil())

				publish := make(chan string, 1)
				outbound := make(chan []byte, 1)

				go func() {
					for item := range deliveries {
						outbound <- item.Body
						item.Ack(false)
					}
				}()
				go main.PublishURLs(ttlHashSet, queueManager, publish)

				publish <- url
				Expect(len(publish)).To(Equal(1))

				Eventually(publish).Should(HaveLen(0))
				Eventually(outbound).Should(HaveLen(0))

				// Close the channel to stop the goroutine for PublishURLs.
				close(publish)
				close(outbound)
			})

			It("publishes URLs that haven't been crawled yet", func() {
				url := "https://www.gov.uk/government/foo"

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				publish := make(chan string, 1)
				outbound := make(chan []byte, 1)

				go func() {
					for item := range deliveries {
						outbound <- item.Body
						item.Ack(false)
					}
				}()
				go main.PublishURLs(ttlHashSet, queueManager, publish)

				publish <- url

				Expect(<-outbound).To(Equal([]byte(url)))
				Expect(len(publish)).To(Equal(0))

				close(publish)
				close(outbound)
			})
		})

		Describe("ReadFromQueue", func() {
			It("provides a way of converting AMQP bodies to CrawlerMessageItems", func() {
				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				outbound := main.ReadFromQueue(deliveries, rootURL, ttlHashSet, []string{}, 1)
				Expect(len(outbound)).To(Equal(0))

				url := "https://www.gov.uk/bar"
				err = queueManager.Publish("#", "text/plain", url)
				Expect(err).To(BeNil())

				item := <-outbound
				item.Ack(false)
				Expect(string(item.Body)).To(Equal(url))

				close(outbound)
			})

			It("drops CrawlerMessageItems containing a blacklisted URL", func() {
				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				url := "https://www.gov.uk/blacklisted"
				err = queueManager.Publish("#", "text/plain", url)
				Expect(err).To(BeNil())

				// Because the `deliveries` channel is unbuffered we're forcing
				// Go to move any items it knows of into our buffered channel
				// so that we can check its length.
				deliveriesBuffer := make(chan amqp.Delivery, 1)
				Expect(len(deliveriesBuffer)).To(Equal(0))
				go func() {
					select {
					case item := <-deliveries:
						deliveriesBuffer <- item
					}
				}()
				Eventually(deliveriesBuffer).Should(HaveLen(1))

				main.ReadFromQueue(deliveriesBuffer, rootURL, ttlHashSet, []string{"/blacklisted"}, 1)

				Eventually(func() (int, error) {
					queueInfo, err := queueManager.Producer.Channel.QueueInspect(queueManager.QueueName)
					return queueInfo.Messages, err
				}).Should(Equal(0))
			})
		})
	})
})

func testServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprintln(w, body)
	}))
}
