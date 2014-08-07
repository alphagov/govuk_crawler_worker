package main

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/golang/glog"
	"github.com/streadway/amqp"
)

const NotRecentlyCrawled int = 0
const AlreadyCrawled int = -1

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
				glog.Infoln("URL is blacklisted (acknowledging):", message.URL())
				continue
			}

			crawlCount, err := ttlHashSet.Get(message.URL())
			if err != nil {
				item.Reject(true)
				glog.Errorln("Couldn't check existence of (rejecting):", message.URL(), err)
				continue
			}

			if crawlCount == AlreadyCrawled {
				glog.Infoln("URL read from queue already crawled:", message.URL())
				if err = item.Ack(false); err != nil {
					glog.Errorln("Ack failed (ReadFromQueue): ", message.URL())
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

func CrawlURL(
	ttlHashSet *ttl_hash_set.TTLHashSet,
	crawlChannel <-chan *CrawlerMessageItem,
	crawler *http_crawler.Crawler,
	crawlerThreads int,
	maxCrawlRetries int,
) <-chan *CrawlerMessageItem {
	if crawlerThreads < 1 {
		panic("cannot start a negative or zero number of crawler threads")
	}

	extractChannel := make(chan *CrawlerMessageItem, 2)

	crawlLoop := func(
		ttlHashSet *ttl_hash_set.TTLHashSet,
		crawl <-chan *CrawlerMessageItem,
		extract chan<- *CrawlerMessageItem,
		crawler *http_crawler.Crawler,
		maxCrawlRetries int,
	) {
		for item := range crawl {
			start := time.Now()
			u, err := url.Parse(item.URL())
			if err != nil {
				item.Reject(false)
				glog.Warningln("Couldn't crawl, invalid URL (rejecting):", item.URL(), err)
				continue
			}
			glog.Infoln("Crawling URL:", u)

			crawlCount, err := ttlHashSet.Get(u.String())
			if err != nil {
				item.Reject(false)
				glog.Errorln("Couldn't confirm existence of URL (rejecting):", u.String(), err)
				continue
			}

			if crawlCount == maxCrawlRetries {
				item.Reject(false)
				glog.Warningln("Aborting crawl of URL which has been retried %d times (rejecting): %s", maxCrawlRetries, u.String())
				continue
			}

			body, err := crawler.Crawl(u)
			if err != nil {
				if err == http_crawler.RetryRequest5XXError || err == http_crawler.RetryRequest429Error {
					if err == http_crawler.RetryRequest5XXError {
						ttlHashSet.Incr(u.String())
					} else if err == http_crawler.RetryRequest429Error {
						sleepTime := 5 * time.Second

						// Back off from crawling for a few seconds.
						glog.Warningln("Sleeping for: ", sleepTime, " seconds. Received 429 HTTP status")
						time.Sleep(sleepTime)
					}

					item.Reject(true)
					glog.Errorln("Couldn't crawl (requeueing):", u.String(), err)
				} else {
					item.Reject(false)
					glog.Errorln("Couldn't crawl (rejecting):", u.String(), err)
				}

				continue
			}

			item.HTMLBody = body

			if item.IsHTML() {
				extract <- item
			} else {
				if err = item.Ack(false); err != nil {
					glog.Errorln("Ack failed (CrawlURL): ", item.URL())
				}

				err = ttlHashSet.Set(item.URL(), AlreadyCrawled)
				if err != nil {
					log.Println("Couldn't mark item as already crawled:", item.URL(), err)
				}
			}

			util.StatsDTiming("crawl_url", start, time.Now())
		}
	}

	for i := 1; i <= crawlerThreads; i++ {
		go crawlLoop(ttlHashSet, crawlChannel, extractChannel, crawler, maxCrawlRetries)
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
				glog.Errorln("Couldn't write to disk (rejecting):", err)
				continue
			}

			filePath := filepath.Join(basePath, relativeFilePath)
			basePath := filepath.Dir(filePath)
			err = os.MkdirAll(basePath, 0755)

			if err != nil {
				item.Reject(false)
				glog.Errorln("Couldn't write to disk (rejecting):", filePath, err)
				continue
			}

			err = ioutil.WriteFile(filePath, item.HTMLBody, 0644)

			if err != nil {
				item.Reject(false)
				glog.Errorln("Couldn't write to disk (rejecting):", filePath, err)
				continue
			}

			glog.Infoln("Wrote URL body to disk for:", item.URL())
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
				glog.Errorln("ExtractURLs (rejecting):", string(item.Body), err)

				continue
			}

			glog.Infoln("Extracted URLs:", len(urls))

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
		crawlCount, err := ttlHashSet.Get(url)

		if err != nil {
			glog.Errorln("Couldn't check existence of URL:", url, err)
			continue
		}

		if crawlCount == AlreadyCrawled {
			glog.Infoln("URL extracted from page already crawled:", url)
		} else if crawlCount == NotRecentlyCrawled {
			err = queueManager.Publish("#", "text/plain", url)
			if err != nil {
				glog.Fatalln("Delivery failed:", url, err)
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

		err := ttlHashSet.Set(url, AlreadyCrawled)
		if err != nil {
			item.Reject(false)
			glog.Errorln("Acknowledge failed (rejecting):", url, err)
			continue
		}

		if err = item.Ack(false); err != nil {
			glog.Errorln("Ack failed (AcknowledgeItem): ", item.URL())
		}
		glog.Infoln("Acknowledged:", url)

		util.StatsDTiming("acknowledge_item", start, time.Now())
	}
}
