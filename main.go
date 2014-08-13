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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
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
	ttlExpireString   = util.GetEnvDefault("TTL_EXPIRE_TIME", "12h")
	mirrorRoot        = os.Getenv("MIRROR_ROOT")
)

const versionNumber string = "0.1.0"

func init() {
	debugFlag := flag.Bool("debug", false, "debug logging")
	quietFlag := flag.Bool("quiet", false, "surpress all logging except errors")
	verboseFlag := flag.Bool("verbose", false, "verbose logging")
	versionFlag := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	switch {
	case *debugFlag:
		log.SetLevel(log.DebugLevel)
	case *quietFlag:
		log.SetLevel(log.ErrorLevel)
	case *verboseFlag:
		log.SetLevel(log.InfoLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}

	log.SetOutput(os.Stderr)

	if *versionFlag {
		fmt.Println(versionNumber)
		os.Exit(0)
	}

	if os.Getenv("GOMAXPROCS") == "" {
		// Use all available cores if not otherwise specified
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	log.Infoln(fmt.Sprintf("using GOMAXPROCS value of %d", runtime.NumCPU()))
}

func main() {
	if mirrorRoot == "" {
		log.Fatalln("MIRROR_ROOT environment variable not set")
	}

	rootURL, err := url.Parse(rootURLString)
	if err != nil {
		log.Fatalln("Couldn't parse ROOT_URL:", rootURLString)
	}

	ttlExpireTime, err := time.ParseDuration(ttlExpireString)
	if err != nil {
		log.Fatalln("Couldn't parse TTL_EXPIRE_TIME:", ttlExpireString)
	}

	ttlHashSet, err := ttl_hash_set.NewTTLHashSet(redisKeyPrefix, redisAddr, ttlExpireTime)
	if err != nil {
		log.Fatalln(err)
	}
	defer ttlHashSet.Close()
	log.Infoln("Connected to Redis service:", ttlHashSet)

	queueManager, err := queue.NewQueueManager(amqpAddr, exchangeName, queueName)
	if err != nil {
		log.Fatalln(err)
	}
	defer queueManager.Close()
	log.Infoln("Connected to AMQP service:", queueManager)

	var crawler *http_crawler.Crawler
	if basicAuthUsername != "" && basicAuthPassword != "" {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber,
			&http_crawler.BasicAuth{basicAuthUsername, basicAuthPassword})
	} else {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber, nil)
	}
	log.Infoln("Generated crawler:", crawler)

	deliveries, err := queueManager.Consume()
	if err != nil {
		log.Fatalln(err)
	}
	log.Infoln("Generated delivery (consumer) channel:", deliveries)

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
	log.Fatalln(http.ListenAndServe(":"+httpPort, nil))

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
