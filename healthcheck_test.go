package main_test

import (
	"time"

	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/alphagov/govuk_crawler_worker/healthcheck"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
	"github.com/alphagov/govuk_crawler_worker/util"
)

var _ = Describe("HealthCheck", func() {
	var (
		amqpAddr, exchangeName, prefix, queueName, redisAddr string
		queueManagerErr, ttlHashSetErr                       error
		queueManager                                         *queue.Manager
		ttlHashSet                                           *ttl_hash_set.TTLHashSet
	)

	BeforeEach(func() {
		amqpAddr = util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")
		exchangeName = "govuk_crawler_worker-test-health-check-exchange"
		queueName = "govuk_crawler_worker-test-health-check-queue"
		redisAddr = util.GetEnvDefault("REDIS_ADDRESS", "127.0.0.1:6379")
		prefix = "govuk_mirror_crawler_health_check_test"
	})

	It("should show Redis and AMQP as being down if they're not connected", func() {
		queueManager, queueManagerErr := queue.NewManager(
			amqpAddr, exchangeName, queueName)
		Expect(queueManagerErr).To(BeNil())

		queueManager.Consumer.HandleChannelClose = func(_ string) {}
		queueManager.Producer.HandleChannelClose = func(_ string) {}

		ttlHashSet, ttlHashSetErr := ttl_hash_set.NewTTLHashSet(prefix, redisAddr, time.Hour, time.Minute)
		Expect(ttlHashSetErr).To(BeNil())

		deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
		Expect(err).To(BeNil())
		Expect(deleted).To(Equal(0))

		err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
		Expect(err).To(BeNil())

		// Close the connections to triggers errors in the response.
		queueManager.Close()
		ttlHashSet.Close()

		healthCheck := NewHealthCheck(queueManager, ttlHashSet)

		// Overall healthcheck status should be critical
		Expect(healthCheck.Status().Status).To(Equal(healthcheck.Critical))

		// Individual statuses should also be critical
		for _, check := range healthCheck.Status().Checks {
			Expect(check.Status).To(Equal(healthcheck.Critical))
		}
	})

	Describe("working with valid Redis and AMQP connections", func() {
		BeforeEach(func() {
			queueManager, queueManagerErr = queue.NewManager(
				amqpAddr, exchangeName, queueName)
			Expect(queueManagerErr).To(BeNil())

			queueManager.Consumer.HandleChannelClose = func(_ string) {}
			queueManager.Producer.HandleChannelClose = func(_ string) {}

			ttlHashSet, ttlHashSetErr = ttl_hash_set.NewTTLHashSet(prefix, redisAddr, time.Hour, time.Minute)
			Expect(ttlHashSetErr).To(BeNil())
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(PurgeAllKeys(prefix, redisAddr)).To(BeNil())

			deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
			Expect(err).To(BeNil())

			queueManager.Close()
		})

		It("reports that the health of the system is OK", func() {
			healthCheck := NewHealthCheck(queueManager, ttlHashSet)
			Expect(healthCheck.Status().Status).To(Equal(healthcheck.OK))
		})

		It("reports that the health of each check is OK", func() {
			healthCheck := NewHealthCheck(queueManager, ttlHashSet)
			for _, check := range healthCheck.Status().Checks {
				Expect(check.Status).To(Equal(healthcheck.OK))
			}
		})
	})

	Describe("Independently closing the Producer and Consumer connections", func() {
		BeforeEach(func() {
			queueManager, queueManagerErr = queue.NewManager(
				amqpAddr, exchangeName, queueName)
			Expect(queueManagerErr).To(BeNil())

			queueManager.Consumer.HandleChannelClose = func(_ string) {}
			queueManager.Producer.HandleChannelClose = func(_ string) {}

			ttlHashSet, ttlHashSetErr = ttl_hash_set.NewTTLHashSet(prefix, redisAddr, time.Hour, time.Minute)
			Expect(ttlHashSetErr).To(BeNil())
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
			Expect(PurgeAllKeys(prefix, redisAddr)).To(BeNil())
		})

		It("should show AMQP as down if the Producer is down", func() {
			healthCheck := NewHealthCheck(queueManager, ttlHashSet)
			queueManager.Producer.Close()

			Expect(healthCheck.Status().Status).To(Equal(healthcheck.Critical))
			Expect(healthCheck.Status().Checks["rabbitmq_publisher"].Status).To(Equal(healthcheck.Critical))

			// Clean up using the consumer.
			deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
			Expect(err).To(BeNil())

			queueManager.Close()
		})

		It("should show AMQP as down if the Consumer is down", func() {
			healthCheck := NewHealthCheck(queueManager, ttlHashSet)
			queueManager.Consumer.Close()

			Expect(healthCheck.Status().Status).To(Equal(healthcheck.Critical))
			Expect(healthCheck.Status().Checks["rabbitmq_consumer"].Status).To(Equal(healthcheck.Critical))

			// Clean up using the producer.
			deleted, err := queueManager.Producer.Channel.QueueDelete(queueName, false, false, true)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Producer.Channel.ExchangeDelete(exchangeName, false, true)
			Expect(err).To(BeNil())

			queueManager.Close()
		})
	})
})
