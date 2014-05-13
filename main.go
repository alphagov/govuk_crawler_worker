package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/streadway/amqp"
)

var (
	amqpAddr       = getEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
	exchangeName   = getEnvDefault("AMQP_EXCHANGE", "govuk_crawler_exchange")
	queueName      = getEnvDefault("AMQP_MESSAGE_QUEUE", "govuk_crawler_queue")
	redisAddr      = getEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
	redisKeyPrefix = getEnvDefault("REDIS_KEY_PREFIX", "govuk_crawler_worker")
	rootURL        = getEnvDefault("ROOT_URL", "https://www.gov.uk/")
)

func main() {
	if os.Getenv("GOMAXPROCS") == "" {
		// Use all available cores if not otherwise specified
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	log.Println(fmt.Sprintf("using GOMAXPROCS value of %d", runtime.NumCPU()))

	ttlHashSet, err := ttl_hash_set.NewTTLHashSet(redisKeyPrefix, redisAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer ttlHashSet.Close()
	log.Println("Connected to Redis service:", ttlHashSet)

	queueManager, err := queue.NewQueueManager(amqpAddr, exchangeName, queueName)
	if err != nil {
		log.Fatal(err)
	}
	defer queueManager.Close()
	log.Println("Connected to AMQP service:", queueManager)

	crawler, err := http_crawler.NewCrawler(rootURL)
	if err != nil {
		log.Fatal("Couldn't generate Crawler:", err)
	}
	log.Println("Generated crawler:", crawler)

	deliveries, err := queueManager.Consume()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Generated delivery (consumer) channel:", deliveries)

	dontQuit := make(chan int)

	crawlItems := readFromQueue(deliveries, ttlHashSet)
	acknowledge := crawlURL(crawlItems, crawler)

	go acknowledgeItem(acknowledge, ttlHashSet)

	<-dontQuit
}

func getEnvDefault(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	return val
}

func readFromQueue(inbound <-chan amqp.Delivery, ttlHashSet *ttl_hash_set.TTLHashSet) <-chan *CrawlerMessageItem {
	outbound := make(chan *CrawlerMessageItem, 1)

	go func() {
		for item := range inbound {
			message := NewCrawlerMessageItem(item, "", []string{})

			exists, err := ttlHashSet.Exists(message.URL())
			if err != nil {
				log.Println("Couldn't check existence of:", message.URL(), err)
				item.Reject(true)
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

func crawlURL(crawlChannel <-chan *CrawlerMessageItem, crawler *http_crawler.Crawler) <-chan *CrawlerMessageItem {
	extract := make(chan *CrawlerMessageItem, 1)

	go func() {
		for item := range crawlChannel {
			url := item.URL()
			log.Println("Crawling URL:", url)

			body, err := crawler.Crawl(url)
			if err != nil {
				item.Reject(false)
				log.Println("Couldn't crawl:", url, err)
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

func acknowledgeItem(inbound <-chan *CrawlerMessageItem, ttlHashSet *ttl_hash_set.TTLHashSet) {
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
