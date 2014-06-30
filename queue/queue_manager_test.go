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
		exchangeName, queueName := "test-manager-exchange", "test-manager-queue"
		queueManager, err := NewQueueManager(
			amqpAddr,
			exchangeName,
			queueName)

		Expect(err).To(BeNil())
		Expect(queueManager).ToNot(BeNil())

		deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
		Expect(err).To(BeNil())
		Expect(deleted).To(Equal(0))

		err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
		Expect(err).To(BeNil())

		Expect(queueManager.Close()).To(BeNil())
	})

	Describe("working with an AMQP service", func() {
		var (
			queueManager    *QueueManager
			queueManagerErr error
		)

		exchangeName, queueName := "test-handler-exchange", "test-handler-queue"

		BeforeEach(func() {
			queueManager, queueManagerErr = NewQueueManager(
				amqpAddr,
				exchangeName,
				queueName)

			Expect(queueManagerErr).To(BeNil())
			Expect(queueManager).ToNot(BeNil())
		})

		AfterEach(func() {
			deleted, err := queueManager.Consumer.Channel.QueueDelete(queueName, false, false, true)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = queueManager.Consumer.Channel.ExchangeDelete(exchangeName, false, true)
			Expect(err).To(BeNil())

			queueManager.Close()
		})

		It("can consume and publish to the AMQP service", func() {
			deliveries, err := queueManager.Consume()
			Expect(err).To(BeNil())

			err = queueManager.Publish("#", "text/plain", "foo")
			Expect(err).To(BeNil())

			for d := range deliveries {
				Expect(string(d.Body)).To(Equal("foo"))
				d.Ack(false)
				break
			}
		})
	})
})
