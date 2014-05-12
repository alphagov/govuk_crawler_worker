package main

import (
	"bytes"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

func (c *CrawlerMessageItem) ExtractURLs(host string) ([]string, error) {
	urls := []string{}

	document, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.HTMLBody))
	if err != nil {
		return urls, err
	}

	urlElementMatches := [][]string{
		[]string{"a", "href"},
		[]string{"img", "src"},
		[]string{"link", "href"},
		[]string{"script", "src"},
	}

	for _, attr := range urlElementMatches {
		element, attr := attr[0], attr[1]
		urls = append(urls, findByElementAttribute(document, host, element, attr)...)
	}

	return urls, err
}

func findByElementAttribute(document *goquery.Document, host string, element string, attr string) []string {
	urls := []string{}

	document.Find(element).Each(func(_ int, element *goquery.Selection) {
		href, exists := element.Attr(attr)
		unescapedHref, _ := url.QueryUnescape(href)
		trimmedHref := strings.TrimSpace(unescapedHref)

		u, err := url.Parse(trimmedHref)
		if err != nil {
			log.Fatal(err)
		}

		if exists && len(trimmedHref) > 0 {
			if u.Host == host {
				urls = append(urls, trimmedHref)
			}

			if u.Host == "" && trimmedHref[0] == '/' {
				urls = append(urls, "http://"+host+trimmedHref)
			}
		}
	})

	return urls
}
