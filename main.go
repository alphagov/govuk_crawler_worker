package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
)

var (
	amqpAddr       = util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
	exchangeName   = util.GetEnvDefault("AMQP_EXCHANGE", "govuk_crawler_exchange")
	queueName      = util.GetEnvDefault("AMQP_MESSAGE_QUEUE", "govuk_crawler_queue")
	redisAddr      = util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
	redisKeyPrefix = util.GetEnvDefault("REDIS_KEY_PREFIX", "govuk_crawler_worker")
	rootURLString  = util.GetEnvDefault("ROOT_URL", "https://www.gov.uk/")
	blacklistPaths = util.GetEnvDefault("BLACKLIST_PATHS", "/search,/government/uploads")
	mirrorRoot     = os.Getenv("MIRROR_ROOT")
)

func main() {
	if mirrorRoot == "" {
		log.Fatal("MIRROR_ROOT environment variable not set")
	}

	rootURL, err := url.Parse(rootURLString)
	if err != nil {
		log.Fatal("Couldn't parse ROOT_URL:", rootURLString)
	}

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

	crawler := http_crawler.NewCrawler(rootURL)
	log.Println("Generated crawler:", crawler)

	deliveries, err := queueManager.Consume()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Generated delivery (consumer) channel:", deliveries)

	dontQuit := make(chan int)

	var acknowledgeChan, crawlChan, persistChan, parseChan <-chan *CrawlerMessageItem
	publishChan := make(<-chan string, 100)

	crawlChan = ReadFromQueue(deliveries, rootURL, ttlHashSet, splitPaths(blacklistPaths))
	persistChan = CrawlURL(crawlChan, crawler)
	parseChan = WriteItemToDisk(mirrorRoot, persistChan)
	publishChan, acknowledgeChan = ExtractURLs(parseChan)

	go PublishURLs(ttlHashSet, queueManager, publishChan)
	go AcknowledgeItem(acknowledgeChan, ttlHashSet)

	<-dontQuit
}

func splitPaths(paths string) []string {
	if !strings.Contains(paths, ",") {
		return []string{paths}
	}

	splitPaths := strings.Split(paths, ",")
	trimmedPaths := make([]string, len(splitPaths))

	for i, v := range splitPaths {
		trimmedPaths[i] = v
	}

	return trimmedPaths
}
