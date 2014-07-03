package main_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
)

var _ = Describe("HealthCheck", func() {
	It("should show Redis and AMQP as being down", func() {
		amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
		exchangeName, queueName := "test-health-check-exchange", "test-health-check-queue"
		redisAddr := util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
		prefix := "govuk_mirror_crawler_health_check_test"

		queueManager, queueManagerErr := queue.NewQueueManager(
			amqpAddr, exchangeName, queueName)
		Expect(queueManagerErr).To(BeNil())

		ttlHashSet, ttlHashSetErr := ttl_hash_set.NewTTLHashSet(prefix, redisAddr)
		Expect(ttlHashSetErr).To(BeNil())

		deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
		Expect(err).To(BeNil())
		Expect(deleted).To(Equal(0))

		err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
		Expect(err).To(BeNil())

		// Close the connections to triggers errors in the response.
		queueManager.Close()
		ttlHashSet.Close()

		Expect(HealthCheck(queueManager, ttlHashSet)).To(Equal(&Status{
			AMQP:  false,
			Redis: false,
		}))
	})

	It("should return a status struct showing the status of RabbitMQ and Redis", func() {
		amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
		exchangeName, queueName := "test-health-check-exchange", "test-health-check-queue"
		redisAddr := util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
		prefix := "govuk_mirror_crawler_health_check_test"

		queueManager, queueManagerErr := queue.NewQueueManager(
			amqpAddr, exchangeName, queueName)
		Expect(queueManagerErr).To(BeNil())

		ttlHashSet, ttlHashSetErr := ttl_hash_set.NewTTLHashSet(prefix, redisAddr)
		Expect(ttlHashSetErr).To(BeNil())

		Expect(HealthCheck(queueManager, ttlHashSet)).To(Equal(&Status{
			AMQP:  true,
			Redis: true,
		}))

		Expect(ttlHashSet.Close()).To(BeNil())
		Expect(PurgeAllKeys(prefix, redisAddr)).To(BeNil())

		deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
		Expect(err).To(BeNil())
		Expect(deleted).To(Equal(0))

		err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
		Expect(err).To(BeNil())

		queueManager.Close()
	})
})
