package main

import (
	"net/http"

	"github.com/streadway/amqp"
)

type CrawlerMessageItem struct {
	amqp.Delivery
	HTMLBody []byte
}

func NewCrawlerMessageItem(delivery amqp.Delivery) *CrawlerMessageItem {
	return &CrawlerMessageItem{Delivery: delivery}
}

func (c *CrawlerMessageItem) IsHTML() bool {
	return http.DetectContentType(c.HTMLBody) == "text/html; charset=utf-8"
}
