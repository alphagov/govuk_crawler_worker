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

	log.Fatal("Nothing to see here yet.")
}

func getEnvDefault(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	return val
}
