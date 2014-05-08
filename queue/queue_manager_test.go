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
			"test-handler-exchange",
			"test-handler-queue")

		Expect(queueManager).To(BeNil())
		Expect(err).ToNot(BeNil())
	})

	It("provides a way of closing connections cleanly", func() {
		queueManager, err := NewQueueManager(
			"amqp://guest:guest@localhost:5672/",
			"test-handler-exchange",
			"test-handler-queue")

		Expect(err).To(BeNil())
		Expect(queueManager).ToNot(BeNil())
		Expect(queueManager.Close()).To(BeNil())
	})
})
