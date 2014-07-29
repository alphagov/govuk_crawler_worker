package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"

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
		exchangeName, queueName := "test-workflow-exchange", "test-workflow-queue"
		prefix := "govuk_mirror_crawler_workflow_test"

		var (
			err             error
			mirrorRoot      string
			queueManager    *QueueManager
			queueManagerErr error
			ttlHashSet      *TTLHashSet
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

			ttlHashSet, ttlHashSetErr = NewTTLHashSet(prefix, redisAddr)
			Expect(ttlHashSetErr).To(BeNil())

			queueManager, queueManagerErr = NewQueueManager(
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
				Expect(exists).To(BeFalse())

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())

				outbound := make(chan *CrawlerMessageItem, 1)

				err = queueManager.Publish("#", "text/plain", url)
				Expect(err).To(BeNil())

				for item := range deliveries {
					outbound <- NewCrawlerMessageItem(item, rootURL, []string{})
					item.Ack(false)
					break
				}

				Expect(len(outbound)).To(Equal(1))

				go AcknowledgeItem(outbound, ttlHashSet)

				Eventually(outbound).Should(HaveLen(0))

				exists, err = ttlHashSet.Exists(url)
				Expect(err).To(BeNil())
				Expect(exists).To(BeTrue())

				// Close the channel to stop the goroutine for AcknowledgeItem.
				close(outbound)
			})
		})

		Describe("CrawlURL", func() {
			var crawler *Crawler

			BeforeEach(func() {
				rootURL, _ = url.Parse("http://127.0.0.1")
				crawler = NewCrawler(rootURL, "0.0.0", nil)
				Expect(crawler).ToNot(BeNil())
			})

			It("crawls a URL and assigns the body", func() {
				outbound := make(chan *CrawlerMessageItem, 1)

				body := `<a href="gov.uk">bar</a>`
				server := testServer(200, body)

				deliveryItem := &amqp.Delivery{Body: []byte(server.URL)}
				outbound <- NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})

				crawled := CrawlURL(outbound, crawler, 1)

				Expect((<-crawled).HTMLBody[0:24]).To(Equal([]byte(body)))

				server.Close()
				close(outbound)
			})

			It("expects the number of goroutines to run to be a positive integer", func() {
				outbound := make(chan *CrawlerMessageItem, 1)

				Expect(func() {
					CrawlURL(outbound, crawler, 0)
				}).To(Panic())

				Expect(func() {
					CrawlURL(outbound, crawler, -1)
				}).To(Panic())
			})
		})

		Describe("WriteItemToDisk", func() {
			It("wrote the item to disk", func() {
				url := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(url)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})
				item.HTMLBody = []byte(`<a href="https://www.gov.uk/some-url">a link</a>`)

				outbound := make(chan *CrawlerMessageItem, 1)
				extract := WriteItemToDisk(mirrorRoot, outbound)

				Expect(len(extract)).To(Equal(0))

				outbound <- item

				Expect(<-extract).To(Equal(item))

				relativeFilePath, _ := item.RelativeFilePath()
				filePath := path.Join(mirrorRoot, relativeFilePath)

				fileContent, err := ioutil.ReadFile(filePath)

				Expect(err).To(BeNil())
				Expect(fileContent).To(Equal(item.HTMLBody))

				close(outbound)
			})
		})

		Describe("ExtractURLs", func() {
			It("extracts URLs from the HTML body and adds them to a new channel; acknowledging item", func() {
				url := "https://www.gov.uk/extract-some-urls"
				deliveryItem := &amqp.Delivery{Body: []byte(url)}
				item := NewCrawlerMessageItem(*deliveryItem, rootURL, []string{})
				item.HTMLBody = []byte(`<a href="https://www.gov.uk/some-url">a link</a>`)

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
				url := "https://www.gov.uk/government/organisations"

				deliveries, err := queueManager.Consume()
				Expect(err).To(BeNil())
				Expect(len(deliveries)).To(Equal(0))

				_, err = ttlHashSet.Incr(url)
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
				go PublishURLs(ttlHashSet, queueManager, publish)

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

				outbound := ReadFromQueue(deliveries, rootURL, ttlHashSet, []string{}, 1)
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

				ReadFromQueue(deliveriesBuffer, rootURL, ttlHashSet, []string{"/blacklisted"}, 1)

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
