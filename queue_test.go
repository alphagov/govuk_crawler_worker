package govuk_crawler_worker_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Queue", func() {
	It("fails if it can't connect to an AMQP server", func() {
		connection, err := NewQueueConnection("amqp://guest:guest@localhost:50000/")

		Expect(err).ToNot(BeNil())
		Expect(connection).To(BeNil())
	})

	Describe("Connecting to a running AMQP service", func() {
		var (
			connection    *QueueConnection
			connectionErr error
		)

		BeforeEach(func() {
			connection, connectionErr = NewQueueConnection("amqp://guest:guest@localhost:5672/")
		})

		It("successfully connects to an AMQP service", func() {
			Expect(connectionErr).To(BeNil())
			Expect(connection).ToNot(BeNil())
		})
	})
})
