package main

import (
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/streadway/amqp"
)

func AcknowledgeItem(inbound <-chan *CrawlerMessageItem, ttlHashSet *ttl_hash_set.TTLHashSet) {
	for item := range inbound {
		func() {
			start := time.Now()
			defer util.StatsDTiming("acknowledge_item", start, time.Now())

			url := item.URL()

			_, err := ttlHashSet.Add(url)
			if err != nil {
				item.Reject(false)
				log.Println("Acknowledge failed (rejecting):", url, err)
				return
			}

			item.Ack(false)
			log.Println("Acknowledged:", url)
			return
		}()
	}
}

func CrawlURL(crawlChannel <-chan *CrawlerMessageItem, crawler *http_crawler.Crawler, crawlerThreads int) <-chan *CrawlerMessageItem {
	if crawlerThreads <= 0 {
		panic("cannot start a negative or zero number of crawler threads")
	}

	extractChannel := make(chan *CrawlerMessageItem, 2)

	crawlLoop := func(
		crawl <-chan *CrawlerMessageItem,
		extract chan<- *CrawlerMessageItem,
		crawler *http_crawler.Crawler,
	) {
		for item := range crawl {
			func() {
				start := time.Now()
				defer util.StatsDTiming("crawl_url", start, time.Now())

				u, err := url.Parse(item.URL())
				if err != nil {
					item.Reject(false)
					log.Println("Couldn't crawl, invalid URL (rejecting):", item.URL(), err)
					return
				}
				log.Println("Crawling URL:", u)

				body, err := crawler.Crawl(u)
				if err != nil {
					if err == http_crawler.RetryRequest5XXError || err == http_crawler.RetryRequest429Error {
						item.Reject(true)
						log.Println("Couldn't crawl (requeueing):", u.String(), err)

						if err == http_crawler.RetryRequest429Error {
							sleepTime := 5 * time.Second

							// Back off from crawling for a few seconds.
							log.Println("Sleeping for: ", sleepTime, " seconds. Received 429 HTTP status")
							time.Sleep(sleepTime)
						}
					} else {
						item.Reject(false)
						log.Println("Couldn't crawl (rejecting):", u.String(), err)
					}

					return
				}

				item.HTMLBody = body

				if item.IsHTML() {
					extract <- item
				} else {
					item.Ack(false)
				}
				return
			}()
		}
	}

	for i := 1; i <= crawlerThreads; i++ {
		go crawlLoop(crawlChannel, extractChannel, crawler)
	}

	return extractChannel
}

func WriteItemToDisk(basePath string, crawlChannel <-chan *CrawlerMessageItem) <-chan *CrawlerMessageItem {
	extractChannel := make(chan *CrawlerMessageItem, 2)

	writeLoop := func(
		crawl <-chan *CrawlerMessageItem,
		extract chan<- *CrawlerMessageItem,
	) {
		for item := range crawl {
			func() {
				start := time.Now()
				defer util.StatsDTiming("write_to_disk", start, time.Now())

				relativeFilePath, err := item.RelativeFilePath()

				if err != nil {
					item.Reject(false)
					log.Println("Couldn't write to disk (rejecting):", err)
					return
				}

				filePath := filepath.Join(basePath, relativeFilePath)
				basePath := filepath.Dir(filePath)
				err = os.MkdirAll(basePath, 0755)

				if err != nil {
					item.Reject(false)
					log.Println("Couldn't write to disk (rejecting):", filePath, err)
					return
				}

				err = ioutil.WriteFile(filePath, item.HTMLBody, 0644)

				if err != nil {
					item.Reject(false)
					log.Println("Couldn't write to disk (rejecting):", filePath, err)
					return
				}

				log.Println("Wrote URL body to disk for:", item.URL())
				extract <- item
				return
			}()
		}
	}

	go writeLoop(crawlChannel, extractChannel)

	return extractChannel
}

func ExtractURLs(extractChannel <-chan *CrawlerMessageItem) (<-chan string, <-chan *CrawlerMessageItem) {
	publishChannel := make(chan string, 100)
	acknowledgeChannel := make(chan *CrawlerMessageItem, 1)

	extractLoop := func(
		extract <-chan *CrawlerMessageItem,
		publish chan<- string,
		acknowledge chan<- *CrawlerMessageItem,
	) {
		for item := range extract {
			func() {
				start := time.Now()
				defer util.StatsDTiming("extract_urls", start, time.Now())

				urls, err := item.ExtractURLs()
				if err != nil {
					item.Reject(false)
					log.Println("ExtractURLs (rejecting):", string(item.Body), err)

					return
				}

				log.Println("Extracted URLs:", len(urls))

				for _, u := range urls {
					publish <- u.String()
				}

				acknowledge <- item
				return
			}()
		}
	}

	go extractLoop(extractChannel, publishChannel, acknowledgeChannel)

	return publishChannel, acknowledgeChannel
}

func PublishURLs(ttlHashSet *ttl_hash_set.TTLHashSet, queueManager *queue.QueueManager, publish <-chan string) {
	for url := range publish {
		func() {
			start := time.Now()
			defer util.StatsDGauge("publish_urls", int64(len(publish)))
			defer util.StatsDTiming("publish_urls", start, time.Now())

			exists, err := ttlHashSet.Exists(url)

			if err != nil {
				log.Println("Couldn't check existence of URL:", url, err)
				return
			}

			if !exists {
				err = queueManager.Publish("#", "text/plain", url)
				if err != nil {
					log.Fatalln("Delivery failed:", url, err)
				}
			}

			return
		}()
	}
}

func ReadFromQueue(inboundChannel <-chan amqp.Delivery, rootURL *url.URL, ttlHashSet *ttl_hash_set.TTLHashSet, blacklistPaths []string) chan *CrawlerMessageItem {
	outboundChannel := make(chan *CrawlerMessageItem, 2)

	readLoop := func(
		inbound <-chan amqp.Delivery,
		outbound chan<- *CrawlerMessageItem,
		ttlHashSet *ttl_hash_set.TTLHashSet,
		blacklistPaths []string,
	) {
		for item := range inbound {
			func() {
				start := time.Now()
				defer util.StatsDTiming("read_from_queue", start, time.Now())

				message := NewCrawlerMessageItem(item, rootURL, blacklistPaths)

				exists, err := ttlHashSet.Exists(message.URL())
				if err != nil {
					item.Reject(true)
					log.Println("Couldn't check existence of (rejecting):", message.URL(), err)
					return
				}

				if exists {
					log.Println("URL already crawled:", message.URL())
					item.Ack(false)
					return
				}

				outbound <- message
				return
			}()
		}
	}

	go readLoop(inboundChannel, outboundChannel, ttlHashSet, blacklistPaths)

	return outboundChannel
}
