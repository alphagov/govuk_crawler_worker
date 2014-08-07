package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

const (
	DEBUG int = iota
	INFO
	WARN
	ERROR
	FATAL
)

var (
	LogDebug   *log.Logger
	LogInfo    *log.Logger
	LogWarning *log.Logger
	LogError   *log.Logger
	LogFatal   *log.Logger

	debugHandle   io.Writer
	infoHandle    io.Writer
	warningHandle io.Writer
	errorHandle   io.Writer
	fatalHandle   io.Writer
)

const versionNumber string = "0.1.0"

func init() {
	versionFlag := flag.Bool("version", false, "show version and exit")
	logLevel := flag.Int("loglevel", ERROR, "logging level")
	flag.Parse()

	if *logLevel > DEBUG {
		debugHandle = ioutil.Discard
	} else {
		debugHandle = os.Stderr
	}
	if *logLevel > INFO {
		infoHandle = ioutil.Discard
	} else {
		infoHandle = os.Stdout
	}
	if *logLevel > WARN {
		warningHandle = ioutil.Discard
	} else {
		warningHandle = os.Stderr
	}
	if *logLevel > ERROR {
		errorHandle = ioutil.Discard
	} else {
		errorHandle = os.Stderr
	}
	if *logLevel > FATAL {
		fatalHandle = ioutil.Discard
	} else {
		fatalHandle = os.Stderr
	}

	LogDebug = log.New(debugHandle,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	LogInfo = log.New(infoHandle,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	LogWarning = log.New(warningHandle,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	LogError = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	LogFatal = log.New(fatalHandle,
		"FATAL: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	if *versionFlag {
		fmt.Println(versionNumber)
		os.Exit(0)
	}

	if os.Getenv("GOMAXPROCS") == "" {
		// Use all available cores if not otherwise specified
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	LogInfo.Println(fmt.Sprintf("using GOMAXPROCS value of %d", runtime.NumCPU()))
}

func main() {
	if mirrorRoot == "" {
		LogFatal.Fatalln("MIRROR_ROOT environment variable not set")
	}

	rootURL, err := url.Parse(rootURLString)
	if err != nil {
		LogFatal.Fatalln("Couldn't parse ROOT_URL:", rootURLString)
	}

	ttlHashSet, err := ttl_hash_set.NewTTLHashSet(redisKeyPrefix, redisAddr)
	if err != nil {
		LogFatal.Fatalln(err)
	}
	defer ttlHashSet.Close()
	LogInfo.Println("Connected to Redis service:", ttlHashSet)

	queueManager, err := queue.NewQueueManager(amqpAddr, exchangeName, queueName)
	if err != nil {
		LogFatal.Fatalln(err)
	}
	defer queueManager.Close()
	LogInfo.Println("Connected to AMQP service:", queueManager)

	var crawler *http_crawler.Crawler
	if basicAuthUsername != "" && basicAuthPassword != "" {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber,
			&http_crawler.BasicAuth{basicAuthUsername, basicAuthPassword})
	} else {
		crawler = http_crawler.NewCrawler(rootURL, versionNumber, nil)
	}
	LogInfo.Println("Generated crawler:", crawler)

	deliveries, err := queueManager.Consume()
	if err != nil {
		LogFatal.Fatalln(err)
	}
	LogInfo.Println("Generated delivery (consumer) channel:", deliveries)

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
	LogFatal.Fatalln(http.ListenAndServe(":"+httpPort, nil))

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
