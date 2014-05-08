package queue_test

import (
	. "github.com/alphagov/govuk_crawler_worker/queue"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("QueueManager", func() {
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
			"amqp://guest:guest@localhost:5672/",
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
})
