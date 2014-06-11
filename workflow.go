package main

import (
	"log"
	"time"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/streadway/amqp"
)

func AcknowledgeItem(inbound <-chan *CrawlerMessageItem, ttlHashSet *ttl_hash_set.TTLHashSet) {
	for item := range inbound {
		url := item.URL()

		_, err := ttlHashSet.Add(url)
		if err != nil {
			item.Reject(false)
			log.Println("Acknowledge failed (rejecting):", url, err)
			continue
		}

		item.Ack(false)
		log.Println("Acknowledged:", url)
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
			url := item.URL()
			log.Println("Crawling URL:", url)

			body, err := crawler.Crawl(url)
			if err != nil {
				if err == http_crawler.RetryRequestError {
					item.Reject(true)
					log.Println("Couldn't crawl (requeueing):", url, err)

					// Back off from crawling for a few seconds.
					time.Sleep(3 * time.Second)
				} else {
					item.Reject(false)
					log.Println("Couldn't crawl (rejecting):", url, err)
				}

				continue
			}

			item.HTMLBody = body

			if item.IsHTML() {
				extract <- item
			} else {
				item.Ack(false)
			}
		}
	}

	go crawlLoop(crawlChannel, extractChannel, crawler)
	go crawlLoop(crawlChannel, extractChannel, crawler)

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
			urls, err := item.ExtractURLs()
			if err != nil {
				item.Reject(false)
				log.Println("ExtractURLs (rejecting):", string(item.Body), err)
			}

			log.Println("Extracted URLs:", len(urls))

			for _, url := range urls {
				publish <- url
			}

			acknowledge <- item
		}
	}

	go extractLoop(extractChannel, publishChannel, acknowledgeChannel)

	return publishChannel, acknowledgeChannel
}

func PublishURLs(ttlHashSet *ttl_hash_set.TTLHashSet, queueManager *queue.QueueManager, publish <-chan string) {
	for url := range publish {
		exists, err := ttlHashSet.Exists(url)

		if err != nil {
			log.Println("Couldn't check existence of URL:", url, err)

			if err.Error() == "use of closed network connection" {
				log.Fatalln("No connection to Redis:", err)
			}
		}

		if !exists {
			err = queueManager.Publish("#", "text/plain", url)
			if err != nil {
				log.Fatalln("Delivery failed:", url, err)
			}
		}
	}
}

func ReadFromQueue(inboundChannel <-chan amqp.Delivery, ttlHashSet *ttl_hash_set.TTLHashSet, blacklistPaths []string) chan *CrawlerMessageItem {
	outboundChannel := make(chan *CrawlerMessageItem, 2)

	readLoop := func(
		inbound <-chan amqp.Delivery,
		outbound chan<- *CrawlerMessageItem,
		ttlHashSet *ttl_hash_set.TTLHashSet,
		blacklistPaths []string,
	) {
		for item := range inbound {
			message := NewCrawlerMessageItem(item, "", blacklistPaths)

			exists, err := ttlHashSet.Exists(message.URL())
			if err != nil {
				if err.Error() == "use of closed network connection" {
					log.Fatalln("No connection to Redis:", err)
				} else {
					item.Reject(true)
					log.Println("Couldn't check existence of (rejecting):", message.URL(), err)
					continue
				}
			}

			if !exists {
				outbound <- message
			} else {
				log.Println("URL already crawled:", message.URL())
				item.Ack(false)
			}
		}
	}

	go readLoop(inboundChannel, outboundChannel, ttlHashSet, blacklistPaths)

	return outboundChannel
}
