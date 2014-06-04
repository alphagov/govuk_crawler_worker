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
	extract := make(chan *CrawlerMessageItem, 1)

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

	return extract
}

func ExtractURLs(extract <-chan *CrawlerMessageItem) (<-chan string, <-chan *CrawlerMessageItem) {
	publishChannel := make(chan string, 1000)
	acknowledgeChannel := make(chan *CrawlerMessageItem, 1)

	go func() {
		for item := range extract {
			urls, err := item.ExtractURLs()
			if err != nil {
				item.Reject(false)
				log.Println("ExtractURLs (rejecting):", string(item.Body), err)
			}

			log.Println("Extracted URLs:", len(urls))

			acknowledgeChannel <- item

			for _, url := range urls {
				publishChannel <- url
			}
		}
	}()

	return publishChannel, acknowledgeChannel
}

func PublishURLs(ttlHashSet *ttl_hash_set.TTLHashSet, queueManager *queue.QueueManager, publish <-chan string) {
	for url := range publish {
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
	}
}

func ReadFromQueue(inbound <-chan amqp.Delivery, ttlHashSet *ttl_hash_set.TTLHashSet) chan *CrawlerMessageItem {
	outbound := make(chan *CrawlerMessageItem, 1)

	go func() {
		for item := range inbound {
			// TODO: Fill out the blacklisted URLs. Maybe using ENV vars?
			message := NewCrawlerMessageItem(item, "", []string{})

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
		}
	}()

	return outbound
}
