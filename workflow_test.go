package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/alphagov/govuk_crawler_worker"
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"
	. "github.com/alphagov/govuk_crawler_worker/queue"
	. "github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
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
			err, queueManagerErr, ttlHashSetErr error

			mirrorRoot   string
			queueManager *Manager
			ttlHashSet   *TTLHashSet
			rootURLs     []*url.URL
			testURL      *url.URL
			urlA         *url.URL
			urlB         *url.URL
			token        string
		)

		BeforeEach(func() {
			mirrorRoot = os.Getenv("MIRROR_ROOT")
			if mirrorRoot == "" {
				mirrorRoot, err = ioutil.TempDir("", "workflow_test")
				Expect(err).To(BeNil())
			}

			urlA = &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/",
			}
			urlB = &url.URL{
				Scheme: "https",
				Host:   "assets.digital.cabinet-office.gov.uk",
				Path:   "/",
			}
			rootURLs = []*url.URL{urlA, urlB}
			token = "cho1coociexei7aech8Zah1rageef2SheewaiQuilaeze1lawoobahcohtheWeik"

			testURL = &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
			}

			ttlHashSet, ttlHashSetErr = NewTTLHashSet(prefix, redisAddr, time.Hour)
			Expect(ttlHashSetErr).To(BeNil())

			queueManager, queueManagerErr = NewManager(
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
				u := "https://www.gov.uk/foo"

				exists, err := ttlHashSet.Exists(u)
				Expect(err).To(BeNil())
				Expect(exists).To(Equal(false))

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				outbound := make(chan *CrawlerMessageItem, 1)

				err = queueManager.Publish("#", "text/plain", u)
				Expect(err).To(BeNil())

				for item := range deliveries {
					outbound <- NewCrawlerMessageItem(item, rootURLs, []string{})
					item.Ack(false)
					break
				}

				Expect(len(outbound)).To(Equal(1))

				go AcknowledgeItem(outbound, ttlHashSet)

				Eventually(outbound).Should(HaveLen(0))
				Eventually(func() bool {
					exists, _ := ttlHashSet.Exists(u)
					return exists
				}).Should(BeTrue())

				// Close the channel to stop the goroutine for AcknowledgeItem.
				close(outbound)
			})
		})

		Describe("CrawlURL", func() {
			var crawler *Crawler
			var rootURLs []*url.URL

			BeforeEach(func() {
				urlA, _ := url.Parse("http://127.0.0.1")
				urlB, _ := url.Parse("http://127.0.0.2")
				rootURLs = []*url.URL{urlA, urlB}
				crawler = NewCrawler(rootURLs, "0.0.0", token, nil)
				Expect(crawler).ToNot(BeNil())
			})

			It("crawls a URL and assigns the body", func() {
				outbound := make(chan *CrawlerMessageItem, 1)

				body := `<a href="gov.uk">bar</a>`
				server := testServer(http.StatusOK, body)

				deliveryItem := &amqp.Delivery{Body: []byte(server.URL)}
				outbound <- NewCrawlerMessageItem(*deliveryItem, rootURLs, []string{})

				crawled := CrawlURL(ttlHashSet, outbound, crawler, 1, 1)

				Expect((<-crawled).Response.Body[0:24]).To(Equal([]byte(body)))

				server.Close()
				close(outbound)
			})

			It("doesn't crawl an item that has been retried too many times", func() {
				body := `<a href="gov.uk">bar</a>`
				server := testServer(http.StatusInternalServerError, body)

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				crawlChan := ReadFromQueue(deliveries, rootURLs, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 5

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
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

				crawlChan := ReadFromQueue(deliveries, rootURLs, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 4

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
				Eventually(crawlChan).Should(HaveLen(0))

				Eventually(func() (int, error) {
					return ttlHashSet.Get(server.URL)
				}).Should(Equal(AlreadyCrawled))

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

				crawlChan := ReadFromQueue(deliveries, rootURLs, ttlHashSet, []string{}, 1)
				Expect(len(crawlChan)).To(Equal(0))

				maxRetries := 4

				err = queueManager.Publish("#", "text/plain", server.URL)
				Expect(err).To(BeNil())
				Eventually(crawlChan).Should(HaveLen(1))

				crawled := CrawlURL(ttlHashSet, crawlChan, crawler, 1, maxRetries)
				Eventually(crawlChan).Should(HaveLen(0))

				Eventually(func() (int, error) {
					return ttlHashSet.Get(server.URL)
				}).Should(Equal(AlreadyCrawled))

				Eventually(func() (int, error) {
					queueInfo, err := queueManager.Producer.Channel.QueueInspect(queueManager.QueueName)
					return queueInfo.Messages, err
				}).Should(Equal(0))
				Expect(len(crawled)).To(Equal(0))

				server.Close()
				close(crawlChan)
			})

			It("expects the number of goroutines to run to be a positive integer", func() {
				outbound := make(chan *CrawlerMessageItem, 1)

				Expect(func() {
					CrawlURL(ttlHashSet, outbound, crawler, 0, 1)
				}).To(Panic())

				Expect(func() {
					CrawlURL(ttlHashSet, outbound, crawler, -1, 1)
				}).To(Panic())
			})
		})

		Describe("WriteItemToDisk", func() {
			It("wrote the item to disk", func() {
				u := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(u)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURLs, []string{})
				item.Response = &CrawlerResponse{
					Body:        []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
					ContentType: HTML,
					URL:         testURL,
				}

				outbound := make(chan *CrawlerMessageItem, 1)
				extract := WriteItemToDisk(mirrorRoot, outbound)

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

			It("did not write files we already have", func() {
				u := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(u)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURLs, []string{})
				item.Response = &CrawlerResponse{
					Body:        []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
					ContentType: HTML,
					URL:         testURL,
				}

				// Manually create the file & get its mod time
				relativeFilePath, _ := item.RelativeFilePath()
				filePath := path.Join(mirrorRoot, relativeFilePath)
				dirPath := filepath.Dir(filePath)
				err := os.MkdirAll(dirPath, 0755)
				err = ioutil.WriteFile(filePath, item.Response.Body, 0644)
				fileInfo, err := os.Stat(filePath)
				initialWriteTime := fileInfo.ModTime().Nanosecond()

				// Make sure there's an appreciable delay between writes
				time.Sleep(time.Second)

				// Run the crawler
				outbound := make(chan *CrawlerMessageItem, 1)
				extract := WriteItemToDisk(mirrorRoot, outbound)
				outbound <- item
				Expect(<-extract).To(Equal(item))

				// Get the values we want to test against
				fileContent, err := ioutil.ReadFile(filePath)
				fileInfo, err = os.Stat(filePath)
				finalWriteTime := fileInfo.ModTime().Nanosecond()

				// The file should be unchanged from when we created it
				Expect(err).To(BeNil())
				Expect(fileContent).To(Equal(item.Response.Body))
				Expect(finalWriteTime).To(Equal(initialWriteTime))

				close(outbound)
			})

			It("wrote files with new contents", func() {
				u := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(u)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURLs, []string{})
				item.Response = &CrawlerResponse{
					Body:        []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
					ContentType: HTML,
					URL:         testURL,
				}

				// Manually create the file so it's not written
				relativeFilePath, _ := item.RelativeFilePath()
				filePath := path.Join(mirrorRoot, relativeFilePath)
				dirPath := filepath.Dir(filePath)
				err := os.MkdirAll(dirPath, 0755)
				err = ioutil.WriteFile(filePath, []byte(`Stale contents`), 0644)

				// Make sure there's an appreciable delay between writes
				time.Sleep(time.Second)

				// Run the crawler
				outbound := make(chan *CrawlerMessageItem, 1)
				extract := WriteItemToDisk(mirrorRoot, outbound)
				outbound <- item
				Expect(<-extract).To(Equal(item))

				// The file should have been updated
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

				item := NewCrawlerMessageItem((<-deliveries), rootURLs, []string{})
				item.Response = &CrawlerResponse{
					Body:        body,
					ContentType: JSON,
					URL:         testURL,
				}

				outbound := make(chan *CrawlerMessageItem, 1)
				extract := WriteItemToDisk(mirrorRoot, outbound)
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
				u := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(u)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURLs, []string{})
				item.Response = &CrawlerResponse{
					Body: []byte(`<a href="https://www.gov.uk/some-url">a link</a>`),
					URL:  testURL,
				}

				outbound := make(chan *CrawlerMessageItem, 1)
				publish, acknowledge := ExtractURLs(outbound)

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
				u := "https://www.gov.uk/government/organisations"

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				err = ttlHashSet.Incr(u)
				Expect(err).To(BeNil())

				publish := make(chan string, 1)
				outbound := make(chan []byte, 1)

				go func() {
					for item := range deliveries {
						outbound <- item.Body
						item.Ack(false)
					}
				}()
				go PublishURLs(ttlHashSet, queueManager, publish)

				publish <- u
				Eventually(publish).Should(HaveLen(1))

				Eventually(publish).Should(HaveLen(0))
				Eventually(outbound).Should(HaveLen(0))

				// Close the channel to stop the goroutine for PublishURLs.
				close(publish)
				close(outbound)
			})

			It("publishes URLs that haven't been crawled yet", func() {
				u := "https://www.gov.uk/government/foo"

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
				go PublishURLs(ttlHashSet, queueManager, publish)

				publish <- u

				Expect(<-outbound).To(Equal([]byte(u)))
				Expect(len(publish)).To(Equal(0))

				close(publish)
				close(outbound)
			})
		})

		Describe("ReadFromQueue", func() {
			It("provides a way of converting AMQP bodies to CrawlerMessageItems", func() {
				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				outbound := ReadFromQueue(deliveries, rootURLs, ttlHashSet, []string{}, 1)
				Expect(len(outbound)).To(Equal(0))

				u := "https://www.gov.uk/bar"
				err = queueManager.Publish("#", "text/plain", u)
				Expect(err).To(BeNil())

				item := <-outbound
				item.Ack(false)
				Expect(string(item.Body)).To(Equal(u))

				close(outbound)
			})

			It("drops CrawlerMessageItems containing a blacklisted URL", func() {
				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				u := "https://www.gov.uk/blacklisted"
				err = queueManager.Publish("#", "text/plain", u)
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

				ReadFromQueue(deliveriesBuffer, rootURLs, ttlHashSet, []string{"/blacklisted"}, 1)

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
