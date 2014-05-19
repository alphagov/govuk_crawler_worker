package main

import (
	"log"

	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
)

func AcknowledgeItem(inbound <-chan *CrawlerMessageItem, ttlHashSet *ttl_hash_set.TTLHashSet) {
	for item := range inbound {
		url := item.URL()

		_, err := ttlHashSet.Add(url)
		if err != nil {
			item.Reject(false)
			log.Println("Acknowledge failed:", url, err)
			continue
		}

		item.Ack(false)
		log.Println("Acknowledged:", url)
	}
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
				log.Println("Delivery failed:", url, err)
			}
		}
	}
}
