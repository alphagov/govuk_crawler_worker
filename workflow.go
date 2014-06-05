package main

import (
	"log"

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
	extract := make(chan *CrawlerMessageItem, 2)

	for i := 0; i < 2; i++ {
		go func() {
			for item := range crawlChannel {
				url := item.URL()
				log.Println("Crawling URL:", url)

				body, err := crawler.Crawl(url)
				if err != nil {
					item.Reject(false)
					log.Println("Couldn't crawl (rejecting):", url, err)
					continue
				}

				item.HTMLBody = body

				if item.IsHTML() {
					extract <- item
				} else {
					item.Ack(false)
				}
			}
		}()
	}

	return extract
}

func ExtractURLs(extract <-chan *CrawlerMessageItem) (<-chan string, <-chan *CrawlerMessageItem) {
	publishChannel := make(chan string, 100)
	acknowledgeChannel := make(chan *CrawlerMessageItem, 1)

	go func() {
		for item := range extract {
			urls, err := item.ExtractURLs()
			if err != nil {
				item.Reject(false)
				log.Println("ExtractURLs (rejecting):", string(item.Body), err)
			}

			log.Println("Extracted URLs:", len(urls))

			for _, url := range urls {
				publishChannel <- url
			}

			acknowledgeChannel <- item
		}
	}()

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

func ReadFromQueue(inbound <-chan amqp.Delivery, ttlHashSet *ttl_hash_set.TTLHashSet, blacklistPaths []string) chan *CrawlerMessageItem {
	outbound := make(chan *CrawlerMessageItem, 2)

	go func() {
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
	}()

	return outbound
}
