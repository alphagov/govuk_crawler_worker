package main

import (
	"github.com/streadway/amqp"
)

type CrawlerMessageItem struct {
	*amqp.Delivery
	HTMLBody  []byte
	URLsFound []string
}

func NewCrawlerMessageItem(delivery *amqp.Delivery) *CrawlerMessageItem {
	return &CrawlerMessageItem{Delivery: delivery}
}
