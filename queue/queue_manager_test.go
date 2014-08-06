package queue_test

import (
	. "github.com/alphagov/govuk_crawler_worker/queue"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/alphagov/govuk_crawler_worker/util"
)

var _ = Describe("QueueManager", func() {
	amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")

	It("returns an error if passed a bad connection address", func() {
		queueManager, err := NewQueueManager(
			"amqp://guest:guest@localhost:50000/",
			"test-manager-exchange",
			"test-manager-queue")

		Expect(queueManager).To(BeNil())
		Expect(err).ToNot(BeNil())
	})

	It("provides a way of closing connections cleanly", func() {
		exchangeName := "govuk_crawler_worker-test-manager-exchange"
		queueName := "govuk_crawler_worker-test-manager-queue"

		queueManager, err := NewQueueManager(
			amqpAddr,
			exchangeName,
			queueName)

		queueManager.Consumer.HandleChannelClose = func(_ string) {}
		queueManager.Producer.HandleChannelClose = func(_ string) {}

		Expect(err).To(BeNil())
		Expect(queueManager).ToNot(BeNil())

		deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, false)
		Expect(err).To(BeNil())
		Expect(deleted).To(Equal(0))

		err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, false)
		Expect(err).To(BeNil())

		Expect(queueManager.Close()).To(BeNil())
	})

	Describe("working with an AMQP service", func() {
		var (
			queueManager    *QueueManager
			queueManagerErr error
		)

		exchangeName := "govuk_crawler_worker-test-handler-exchange"
		queueName := "govuk_crawler_worker-test-handler-queue"

		BeforeEach(func() {
			queueManager, queueManagerErr = NewQueueManager(
				amqpAddr,
				exchangeName,
				queueName)

			Expect(queueManagerErr).To(BeNil())
			Expect(queueManager).ToNot(BeNil())

			queueManager.Consumer.HandleChannelClose = func(_ string) {}
			queueManager.Producer.HandleChannelClose = func(_ string) {}
		})

		AfterEach(func() {
			// Consumer must Cancel() or Close() before deleting.
			queueManager.Consumer.Close()
			defer queueManager.Close()

			deleted, err := queueManager.Producer.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Producer.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())
		})

		It("can consume and publish to the AMQP service", func(done Done) {
			deliveries, err := queueManager.Consume()
			Expect(err).To(BeNil())

			err = queueManager.Publish("#", "text/plain", "foo")
			Expect(err).To(BeNil())

			item := <-deliveries
			Expect(string(item.Body)).To(Equal("foo"))
			item.Ack(false)
			close(done)
		})
	})
})
