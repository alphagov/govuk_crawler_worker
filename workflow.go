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

func ReadFromQueue(
	inboundChannel <-chan amqp.Delivery,
	rootURL *url.URL,
	ttlHashSet *ttl_hash_set.TTLHashSet,
	blacklistPaths []string,
	crawlerThreads int,
) chan *CrawlerMessageItem {
	outboundChannel := make(chan *CrawlerMessageItem, crawlerThreads)

	readLoop := func(
		inbound <-chan amqp.Delivery,
		outbound chan<- *CrawlerMessageItem,
		ttlHashSet *ttl_hash_set.TTLHashSet,
		blacklistPaths []string,
	) {
		for item := range inbound {
			start := time.Now()
			message := NewCrawlerMessageItem(item, rootURL, blacklistPaths)

			if message.IsBlacklisted() {
				item.Ack(false)
				log.Println("URL is blacklisted (acknowledging):", message.URL())
				continue
			}

			val, err := ttlHashSet.Get(message.URL())
			if err != nil {
				item.Reject(true)
				log.Println("Couldn't check existence of (rejecting):", message.URL(), err)
				continue
			}

			if val == -1 {
				log.Println("URL already crawled:", message.URL())
				if err = item.Ack(false); err != nil {
					log.Println("Ack failed (ReadFromQueue): ", message.URL())
				}
				continue
			}

			outbound <- message

			util.StatsDTiming("read_from_queue", start, time.Now())
		}
	}

	go readLoop(inboundChannel, outboundChannel, ttlHashSet, blacklistPaths)

	return outboundChannel
}

func CrawlURL(crawlChannel <-chan *CrawlerMessageItem, crawler *http_crawler.Crawler, crawlerThreads int) <-chan *CrawlerMessageItem {
	if crawlerThreads < 1 {
		panic("cannot start a negative or zero number of crawler threads")
	}

	extractChannel := make(chan *CrawlerMessageItem, 2)

	crawlLoop := func(
		crawl <-chan *CrawlerMessageItem,
		extract chan<- *CrawlerMessageItem,
		crawler *http_crawler.Crawler,
	) {
		for item := range crawl {
			start := time.Now()
			u, err := url.Parse(item.URL())
			if err != nil {
				item.Reject(false)
				log.Println("Couldn't crawl, invalid URL (rejecting):", item.URL(), err)
				continue
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

				continue
			}

			item.HTMLBody = body

			if item.IsHTML() {
				extract <- item
			} else {
				if err = item.Ack(false); err != nil {
					log.Println("Ack failed (CrawlURL): ", item.URL())
				}
			}

			util.StatsDTiming("crawl_url", start, time.Now())
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
			start := time.Now()
			relativeFilePath, err := item.RelativeFilePath()

			if err != nil {
				item.Reject(false)
				log.Println("Couldn't write to disk (rejecting):", err)
				continue
			}

			filePath := filepath.Join(basePath, relativeFilePath)
			basePath := filepath.Dir(filePath)
			err = os.MkdirAll(basePath, 0755)

			if err != nil {
				item.Reject(false)
				log.Println("Couldn't write to disk (rejecting):", filePath, err)
				continue
			}

			err = ioutil.WriteFile(filePath, item.HTMLBody, 0644)

			if err != nil {
				item.Reject(false)
				log.Println("Couldn't write to disk (rejecting):", filePath, err)
				continue
			}

			log.Println("Wrote URL body to disk for:", item.URL())
			extract <- item

			util.StatsDTiming("write_to_disk", start, time.Now())
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
			start := time.Now()
			urls, err := item.ExtractURLs()
			if err != nil {
				item.Reject(false)
				log.Println("ExtractURLs (rejecting):", string(item.Body), err)

				continue
			}

			log.Println("Extracted URLs:", len(urls))

			for _, u := range urls {
				publish <- u.String()
			}

			acknowledge <- item

			util.StatsDTiming("extract_urls", start, time.Now())
		}
	}

	go extractLoop(extractChannel, publishChannel, acknowledgeChannel)

	return publishChannel, acknowledgeChannel
}

func PublishURLs(ttlHashSet *ttl_hash_set.TTLHashSet, queueManager *queue.QueueManager, publish <-chan string) {
	for url := range publish {
		start := time.Now()
		val, err := ttlHashSet.Get(url)

		if err != nil {
			log.Println("Couldn't check existence of URL:", url, err)
			continue
		}

		if val == 0 {
			err = queueManager.Publish("#", "text/plain", url)
			if err != nil {
				log.Fatalln("Delivery failed:", url, err)
			}
		}

		util.StatsDGauge("publish_urls", int64(len(publish)))
		util.StatsDTiming("publish_urls", start, time.Now())
	}
}

func AcknowledgeItem(inbound <-chan *CrawlerMessageItem, ttlHashSet *ttl_hash_set.TTLHashSet) {
	for item := range inbound {
		start := time.Now()
		url := item.URL()

		_, err := ttlHashSet.Incr(url)
		if err != nil {
			item.Reject(false)
			log.Println("Acknowledge failed (rejecting):", url, err)
			continue
		}

		if err = item.Ack(false); err != nil {
			log.Println("Ack failed (AcknowledgeItem): ", item.URL())
		}
		log.Println("Acknowledged:", url)

		util.StatsDTiming("acknowledge_item", start, time.Now())
	}
}
