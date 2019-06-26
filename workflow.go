package main

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/streadway/amqp"
)

const ReadyToEnqueue int = 0
const Enqueued int = 1

func ReadFromQueue(
	inboundChannel <-chan amqp.Delivery,
	rootURLs []*url.URL,
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
			message := NewCrawlerMessageItem(item, rootURLs, blacklistPaths)

			if message.IsBlacklisted() {
				item.Ack(false)
				log.Debugln("URL is blacklisted (acknowledging):", message.URL())
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
				log.Warningln("Couldn't crawl, invalid URL (rejecting):", item.URL(), err)
				continue
			}

			crawlCount, err := ttlHashSet.Get(u.String())
			if err != nil {
				item.Reject(false)
				log.Errorln("Couldn't confirm existence of URL (rejecting):", u.String(), err)
				continue
			}

			if crawlCount > maxCrawlRetries {
				item.Reject(false)
				log.Errorf("Aborting crawl of URL which has been retried %d times (rejecting): %s", maxCrawlRetries, u.String())

				continue
			}

			log.Debugln("Starting crawl of URL:", u)
			response, err := crawler.Crawl(u)
			if err != nil {
				switch err {
				case http_crawler.ErrRetryRequest5XX, http_crawler.ErrRetryRequest429:
					switch err {
					case http_crawler.ErrRetryRequest5XX:
						ttlHashSet.Incr(u.String())
					case http_crawler.ErrRetryRequest429:
						sleepTime := 5 * time.Second

						// Back off from crawling for a few seconds.
						log.Warningf("Sleeping for: %v. Received 429 HTTP status", sleepTime)
						time.Sleep(sleepTime)
					}

					item.Reject(true)

					log.Warningln("Couldn't crawl (requeueing):", u.String(), err)
				default:
					item.Reject(false)
					log.Warningln("Couldn't crawl (rejecting):", u.String(), err)
				}

				continue
			}

			item.Response = response

			if item.Response.AcceptedContentType() {
				extract <- item
			} else {
				if err = item.Ack(false); err != nil {
					log.Errorln("Ack failed (CrawlURL): ", item.URL())
				}

				err = ttlHashSet.Set(item.URL(), ReadyToEnqueue)
				if err != nil {
					log.Errorln("Couldn't mark item as already crawled:", item.URL(), err)
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

			if item.hasParams() {
				log.Debugln("Skipping write to disk as query params exists:", item.URL(), err)
			} else {
				if err != nil {
					item.Reject(false)
					log.Errorln("Couldn't retrieve relative file path for item (rejecting):", item.URL(), err)
					continue
				}

				filePath := filepath.Join(basePath, relativeFilePath)
				dirPath := filepath.Dir(filePath)
				err = os.MkdirAll(dirPath, 0755)

				if err != nil {
					item.Reject(false)
					log.Errorln("Couldn't create directories for item (rejecting):", filePath, err)
					continue
				}

				err = ioutil.WriteFile(filePath, item.Response.Body, 0644)

				if err != nil {
					item.Reject(false)
					log.Errorln("Couldn't write to disk (rejecting):", filePath, err)
					continue
				}

				log.Debugln("Wrote URL body to disk for:", item.URL())
			}

			contentType, err := item.Response.ParseContentType()
			if err != nil {
				log.Errorln("Couldn't determine Content-Type for item (rejecting):", item, err)
				item.Reject(false)
				continue
			}

			// Only send HTML pages for URL extraction. All other
			// pages should be written directly to disk and acknowledged.
			if contentType == http_crawler.HTML {
				extract <- item
			} else {
				item.Ack(false)
			}

			util.StatsDTiming("write_to_disk", start, time.Now())
		}
	}

	go writeLoop(crawlChannel, extractChannel)

	return extractChannel
}

func ExtractURLs(extractChannel <-chan *CrawlerMessageItem) (<-chan *url.URL, <-chan *CrawlerMessageItem) {
	publishChannel := make(chan *url.URL, 100)
	acknowledgeChannel := make(chan *CrawlerMessageItem, 1)

	extractLoop := func(
		extract <-chan *CrawlerMessageItem,
		publish chan<- *url.URL,
		acknowledge chan<- *CrawlerMessageItem,
	) {
		for item := range extract {
			start := time.Now()
			urls, err := item.ExtractURLs()
			if err != nil {
				item.Reject(false)
				log.Errorln("ExtractURLs (rejecting):", string(item.Body), err)

				continue
			}

			log.Debugln("Extracted URLs:", len(urls))

			for _, u := range urls {
				publish <- u
			}

			acknowledge <- item

			util.StatsDTiming("extract_urls", start, time.Now())
		}
	}

	go extractLoop(extractChannel, publishChannel, acknowledgeChannel)

	return publishChannel, acknowledgeChannel
}

func PublishURLs(ttlHashSet *ttl_hash_set.TTLHashSet, queueManager *queue.Manager, publish <-chan *url.URL) {
	for uu := range publish {

		u := uu.String()

		if uu.RawQuery != "" {
			values, err := url.ParseQuery(uu.RawQuery)
			if err != nil {
				log.Debugln("Skipping URL unable to decode query params:", u)
				continue
			}
			values.Del("page") // ignore pagination params. This ensures we process URL from every page of our search pages
			if values.Encode() != "" {
				log.Debugln("Skipping URL as it has query params:", u)
				continue
			}
		}

		start := time.Now()
		queueStatus, err := ttlHashSet.Get(u)

		if err != nil {
			log.Errorln("Couldn't check existence of URL:", u, err)
			continue
		}

		if queueStatus == Enqueued {
			log.Debugln("URL is already in the queue:", u)
		} else if queueStatus > Enqueued {
			log.Debugln("URL is already in the queue (reporting 5XX's):", u)
		} else {
			ttlHashSet.Set(u, Enqueued)

			err = queueManager.Publish("#", "text/plain", u)
			if err != nil {
				log.Fatalln("Delivery failed:", u, err)
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

		err := item.Ack(false)
		if err != nil {
			log.Errorln("Ack failed (AcknowledgeItem): ", item.URL())
		}
		log.Debugln("Acknowledged:", url)

		util.StatsDTiming("acknowledge_item", start, time.Now())
	}
}
