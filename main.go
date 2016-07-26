package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/airbrake"
	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
)

var (
	airbrakeAPIKey      = os.Getenv("AIRBRAKE_API_KEY")
	airbrakeEnvironment = os.Getenv("AIRBRAKE_ENV")
	airbrakeEndpoint    = os.Getenv("AIRBRAKE_ENDPOINT")

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
	rootURLs          []*url.URL
	rootURLString     = util.GetEnvDefault("ROOT_URLS", "https://www.gov.uk/")
	ttlExpireString   = util.GetEnvDefault("TTL_EXPIRE_TIME", "12h")
	mirrorRoot        = os.Getenv("MIRROR_ROOT")
	rateLimitToken    = os.Getenv("RATE_LIMIT_TOKEN")
)

const versionNumber string = "0.1.0"

func init() {
	jsonFlag := flag.Bool("json", false, "output logs as JSON")

	quietFlag := flag.Bool("quiet", false, "surpress all logging except errors")
	verboseFlag := flag.Bool("verbose", false, "verbose logging showing debug messages")
	versionFlag := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	switch {
	case *quietFlag:
		log.SetLevel(log.ErrorLevel)
	case *verboseFlag:
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	log.SetOutput(os.Stderr)

	if *jsonFlag {
		log.SetFormatter(new(log.JSONFormatter))
	}

	if airbrakeAPIKey != "" && airbrakeEndpoint != "" && airbrakeEnvironment != "" {
		log.AddHook(airbrake.NewHook(airbrakeEndpoint, airbrakeAPIKey, airbrakeEnvironment))
		log.Infof("Logging exceptions to Airbrake endpoint %s", airbrakeEndpoint)
	}

	if *versionFlag {
		fmt.Println(versionNumber)
		os.Exit(0)
	}
}

func main() {
	if mirrorRoot == "" {
		log.Fatalln("MIRROR_ROOT environment variable not set")
	}

	rootURLStrings := strings.Split(rootURLString, ",")

	for _, u := range rootURLStrings {
		rootURL, err := url.Parse(u)

		if err != nil {
			log.Fatalln("Couldn't parse ROOT_URL:", u)
		}

		rootURLs = append(rootURLs, rootURL)
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

	queueManager, err := queue.NewManager(amqpAddr, exchangeName, queueName)
	if err != nil {
		log.Fatalln(err)
	}
	defer queueManager.Close()
	log.Infoln("Connected to AMQP service:", queueManager)

	var crawler *http_crawler.Crawler
	if basicAuthUsername != "" && basicAuthPassword != "" {
		crawler = http_crawler.NewCrawler(rootURLs, versionNumber, rateLimitToken,
			&http_crawler.BasicAuth{basicAuthUsername, basicAuthPassword})
	} else {
		crawler = http_crawler.NewCrawler(rootURLs, versionNumber, rateLimitToken, nil)
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

	crawlChan = ReadFromQueue(deliveries, rootURLs, ttlHashSet, splitPaths(blacklistPaths), crawlerThreadsInt)
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
