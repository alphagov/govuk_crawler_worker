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
		start := time.Now()
		url := item.URL()

		_, err := ttlHashSet.Add(url)
		if err != nil {
			item.Reject(false)
			log.Println("Acknowledge failed (rejecting):", url, err)
			continue
		}

		item.Ack(false)
		log.Println("Acknowledged:", url)

		util.StatsDTiming("acknowledge_item", start, time.Now())
	}
}

func CrawlURL(crawlChannel <-chan *CrawlerMessageItem, crawler *http_crawler.Crawler) <-chan *CrawlerMessageItem {
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
			}
			log.Println("Crawling URL:", u)

			body, err := crawler.Crawl(u)
			if err != nil {
				if err == http_crawler.RetryRequestError {
					item.Reject(true)
					log.Println("Couldn't crawl (requeueing):", u.String(), err)

					// Back off from crawling for a few seconds.
					time.Sleep(3 * time.Second)
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
				item.Ack(false)
			}

			util.StatsDTiming("crawl_url", start, time.Now())
		}
	}

	go crawlLoop(crawlChannel, extractChannel, crawler)
	go crawlLoop(crawlChannel, extractChannel, crawler)

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
		exists, err := ttlHashSet.Exists(url)

		if err != nil {
			log.Println("Couldn't check existence of URL:", url, err)
		}

		if !exists {
			err = queueManager.Publish("#", "text/plain", url)
			if err != nil {
				log.Fatalln("Delivery failed:", url, err)
			}
		}

		util.StatsDTiming("publish_urls", start, time.Now())
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
			start := time.Now()
			message := NewCrawlerMessageItem(item, rootURL, blacklistPaths)
			log.Println(rootURL)

			exists, err := ttlHashSet.Exists(message.URL())
			if err != nil {
				item.Reject(true)
				log.Println("Couldn't check existence of (rejecting):", message.URL(), err)
				continue
			}

			if !exists {
				outbound <- message
			} else {
				log.Println("URL already crawled:", message.URL())
				item.Ack(false)
			}

			util.StatsDTiming("read_from_queue", start, time.Now())
		}
	}

	go readLoop(inboundChannel, outboundChannel, ttlHashSet, blacklistPaths)

	return outboundChannel
}
