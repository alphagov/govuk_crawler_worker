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
				log.Debugln("URL is blacklisted (acknowledging):", message.URL())
				continue
			}

			crawlCount, err := ttlHashSet.Get(message.URL())
			if err != nil {
				item.Reject(true)
				log.Errorln("Couldn't check existence of (rejecting):", message.URL(), err)
				continue
			}

			if crawlCount == AlreadyCrawled {
				log.Debugln("URL read from queue already crawled:", message.URL())
				if err = item.Ack(false); err != nil {
					log.Errorln("Ack failed (ReadFromQueue): ", message.URL())
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
				log.Warningln("Couldn't crawl, invalid URL (rejecting):", item.URL(), err)
				continue
			}

			crawlCount, err := ttlHashSet.Get(u.String())
			if err != nil {
				item.Reject(false)
				log.Errorln("Couldn't confirm existence of URL (rejecting):", u.String(), err)
				continue
			}

			if crawlCount == maxCrawlRetries {
				item.Reject(false)
				log.Errorf("Aborting crawl of URL which has been retried %d times (rejecting): %s", maxCrawlRetries, u.String())
				continue
			}

			log.Debugln("Starting crawl of URL:", u)
			response, err := crawler.Crawl(u)
			if err != nil {
				switch err {
				case http_crawler.RetryRequest5XXError, http_crawler.RetryRequest429Error:
					switch err {
					case http_crawler.RetryRequest5XXError:
						ttlHashSet.Incr(u.String())
					case http_crawler.RetryRequest429Error:
						sleepTime := 5 * time.Second

						// Back off from crawling for a few seconds.
						log.Warningf("Sleeping for: %v. Received 429 HTTP status", sleepTime)
						time.Sleep(sleepTime)
					}

					item.Reject(true)
					log.Warningln("Couldn't crawl (requeueing):", u.String(), err)
				case http_crawler.RedirectError:
					setErr := ttlHashSet.Set(u.String(), AlreadyCrawled)
					if setErr != nil {
						log.Errorln("Couldn't mark item as already crawled:", u.String(), setErr)
					}

					item.Reject(false)
					// log at INFO because redirect URLs are not a concern
					log.Debugln("Couldn't crawl (rejecting):", u.String(), err)
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

				err = ttlHashSet.Set(item.URL(), AlreadyCrawled)
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

			if err != nil {
				item.Reject(false)
				log.Errorln("Couldn't retrieve relative file path for item (rejecting):", item.URL(), err)
				continue
			}

			filePath := filepath.Join(basePath, relativeFilePath)
			basePath := filepath.Dir(filePath)
			err = os.MkdirAll(basePath, 0755)

			if err != nil {
				item.Reject(false)
				log.Errorln("Couldn't created directories for item (rejecting):", filePath, err)
				continue
			}

			err = ioutil.WriteFile(filePath, item.Response.Body, 0644)

			if err != nil {
				item.Reject(false)
				log.Errorln("Couldn't write to disk (rejecting):", filePath, err)
				continue
			}

			log.Debugln("Wrote URL body to disk for:", item.URL())

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
				log.Errorln("ExtractURLs (rejecting):", string(item.Body), err)

				continue
			}

			log.Debugln("Extracted URLs:", len(urls))

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
			log.Errorln("Couldn't check existence of URL:", url, err)
			continue
		}

		if crawlCount == AlreadyCrawled {
			log.Debugln("URL extracted from page already crawled:", url)
		} else if crawlCount == NotRecentlyCrawled {
			err = queueManager.Publish("#", "text/plain", url)
			if err != nil {
				log.Fatalln("Delivery failed:", url, err)
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
			log.Errorln("Acknowledge failed (rejecting):", url, err)
			continue
		}

		if err = item.Ack(false); err != nil {
			log.Errorln("Ack failed (AcknowledgeItem): ", item.URL())
		}
		log.Debugln("Acknowledged:", url)

		util.StatsDTiming("acknowledge_item", start, time.Now())
	}
}
