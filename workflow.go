package main

import (
	"log"

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
