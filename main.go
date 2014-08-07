package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/golang/glog"
)

var (
	amqpAddr          = util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
	basicAuthPassword = util.GetEnvDefault("BASIC_AUTH_PASSWORD", "")
	basicAuthUsername = util.GetEnvDefault("BASIC_AUTH_USERNAME", "")
	blacklistPaths    = util.GetEnvDefault("BLACKLIST_PATHS", "/search,/government/uploads")
	crawlerThreads    = util.GetEnvDefault("CRAWLER_THREADS", "4")
	exchangeName      = util.GetEnvDefault("AMQP_EXCHANGE", "govuk_crawler_exchange")
	httpPort          = util.GetEnvDefault("HTTP_PORT", "8080")
	maxCrawlRetries   = util.GetEnvDefault("MAX_CRAWL_RETRIES", "4")
	queueName         = util.GetEnvDefault("AMQP_MESSAGE_QUEUE", "govuk_crawler_queue")
	redisAddr         = util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
	redisKeyPrefix    = util.GetEnvDefault("REDIS_KEY_PREFIX", "govuk_crawler_worker")
	rootURLString     = util.GetEnvDefault("ROOT_URL", "https://www.gov.uk/")
	mirrorRoot        = os.Getenv("MIRROR_ROOT")
)

const versionNumber string = "0.1.0"

func main() {
	versionFlag := flag.Bool("version", false, "show version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(versionNumber)
		os.Exit(0)
	}
	if mirrorRoot == "" {
		glog.Fatalln("MIRROR_ROOT environment variable not set")
	}

	rootURL, err := url.Parse(rootURLString)
	if err != nil {
		glog.Fatalln("Couldn't parse ROOT_URL:", rootURLString)
	}

	if os.Getenv("GOMAXPROCS") == "" {
		// Use all available cores if not otherwise specified
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	glog.Infoln(fmt.Sprintf("using GOMAXPROCS value of %d", runtime.NumCPU()))

	ttlHashSet, err := ttl_hash_set.NewTTLHashSet(redisKeyPrefix, redisAddr)
	if err != nil {
		glog.Fatalln(err)
	}
	defer ttlHashSet.Close()
	glog.Infoln("Connected to Redis service:", ttlHashSet)

	queueManager, err := queue.NewQueueManager(amqpAddr, exchangeName, queueName)
	if err != nil {
		glog.Fatalln(err)
	}
	defer queueManager.Close()
	glog.Infoln("Connected to AMQP service:", queueManager)

	var crawler *http_crawler.Crawler
	if basicAuthUsername != "" && basicAuthPassword != "" {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber,
			&http_crawler.BasicAuth{basicAuthUsername, basicAuthPassword})
	} else {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber, nil)
	}
	glog.Infoln("Generated crawler:", crawler)

	deliveries, err := queueManager.Consume()
	if err != nil {
		glog.Fatalln(err)
	}
	glog.Infoln("Generated delivery (consumer) channel:", deliveries)

	dontQuit := make(chan struct{})

	var acknowledgeChan, crawlChan, persistChan, parseChan <-chan *CrawlerMessageItem
	publishChan := make(<-chan string, 100)

	var crawlerThreadsInt int
	crawlerThreadsInt, err = strconv.Atoi(crawlerThreads)
	if err != nil {
		crawlerThreadsInt = 1
	}

	var maxCrawlRetriesInt int
	maxCrawlRetriesInt, err = strconv.Atoi(maxCrawlRetries)
	if err != nil {
		maxCrawlRetriesInt = 4
	}

	crawlChan = ReadFromQueue(deliveries, rootURL, ttlHashSet, splitPaths(blacklistPaths), crawlerThreadsInt)
	persistChan = CrawlURL(ttlHashSet, crawlChan, crawler, crawlerThreadsInt, maxCrawlRetriesInt)
	parseChan = WriteItemToDisk(mirrorRoot, persistChan)
	publishChan, acknowledgeChan = ExtractURLs(parseChan)

	go PublishURLs(ttlHashSet, queueManager, publishChan)
	go AcknowledgeItem(acknowledgeChan, ttlHashSet)

	healthCheck := NewHealthCheck(queueManager, ttlHashSet)
	http.HandleFunc("/healthcheck", healthCheck.HTTPHandler())
	glog.Fatalln(http.ListenAndServe(":"+httpPort, nil))

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
