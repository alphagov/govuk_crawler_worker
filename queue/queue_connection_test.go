package queue_test

import (
	. "github.com/alphagov/govuk_crawler_worker/queue"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"net/url"

	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/streadway/amqp"
)

var _ = Describe("QueueConnection", func() {
	amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")

	It("fails if it can't connect to an AMQP server", func() {
		connection, err := NewQueueConnection("amqp://guest:guest@localhost:50000/")

		Expect(err).ToNot(BeNil())
		Expect(connection).To(BeNil())
	})

	Describe("Reconnects", func() {
		var (
			publisher    *QueueConnection
			consumer     *QueueConnection
			proxy        *util.ProxyTCP
			proxyAddr    string = "localhost:5673"
			exchangeName string = "test-crawler-exchange"
			queueName    string = "test-crawler-queue"
		)

		BeforeEach(func() {
			proxyDest, err := addrFromURL(amqpAddr)
			Expect(err).To(BeNil())
			proxyURL, err := urlChangeAddr(amqpAddr, proxyAddr)
			Expect(err).To(BeNil())

			proxy, err = util.NewProxyTCP(proxyAddr, proxyDest)
			Expect(err).To(BeNil())
			Expect(proxy).ToNot(BeNil())

			publisher, err = NewQueueConnection(proxyURL)
			Expect(err).To(BeNil())
			Expect(publisher).ToNot(BeNil())

			consumer, err = NewQueueConnection(proxyURL)
			Expect(err).To(BeNil())
			Expect(consumer).ToNot(BeNil())
		})

		AfterEach(func() {
			defer consumer.Close()
			defer publisher.Close()
			defer proxy.Close()

			deleted, err := consumer.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			// Consumer cannot delete exchange unless we Cancel() or Close()
			err = publisher.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())
		})

		It("should reconnect on errors", func() {
			var err error

			err = consumer.ExchangeDeclare(exchangeName, "direct")
			Expect(err).To(BeNil())

			_, err = consumer.QueueDeclare(queueName)
			Expect(err).To(BeNil())

			err = consumer.BindQueueToExchange(queueName, exchangeName)
			Expect(err).To(BeNil())

			deliveries, err := consumer.Consume(queueName)
			Expect(err).To(BeNil())

			err = publisher.Publish(exchangeName, "#", "text/plain", "foo")
			Expect(err).To(BeNil())

			//proxy.KillConnected()

			for d := range deliveries {
				Expect(string(d.Body)).To(Equal("foo"))
				d.Ack(false)
				break
			}
		})
	})

	Describe("Connecting to a running AMQP service", func() {
		var (
			connection    *QueueConnection
			connectionErr error
		)

		BeforeEach(func() {
			connection, connectionErr = NewQueueConnection(amqpAddr)
		})

		AfterEach(func() {
			defer connection.Close()
		})

		It("successfully connects to an AMQP service", func() {
			Expect(connectionErr).To(BeNil())
			Expect(connection).ToNot(BeNil())
		})

		It("can close the connection without errors", func() {
			Expect(connection.Close()).To(BeNil())
		})

		It("can declare an exchange", func() {
			var err error
			exchange := "some-exchange"

			err = connection.ExchangeDeclare(exchange, "direct")
			Expect(err).To(BeNil())

			err = connection.Channel.ExchangeDelete(exchange, false, false)
			Expect(err).To(BeNil())
		})

		It("can declare a queue", func() {
			var (
				err   error
				queue amqp.Queue
				name  = "some-queue"
			)

			queue, err = connection.QueueDeclare(name)
			Expect(err).To(BeNil())
			Expect(queue.Name).To(Equal(name))

			deleted, err := connection.Channel.QueueDelete(name, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))
		})

		It("can bind a queue to an exchange", func() {
			var err error

			exchangeName := "some-binding-exchange"
			queueName := "some-binding-queue"

			err = connection.ExchangeDeclare(exchangeName, "direct")
			Expect(err).To(BeNil())

			_, err = connection.QueueDeclare(queueName)
			Expect(err).To(BeNil())

			err = connection.BindQueueToExchange(queueName, exchangeName)
			Expect(err).To(BeNil())

			deleted, err := connection.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = connection.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())
		})
	})

	Describe("working with messages on the queue", func() {
		var (
			publisher *QueueConnection
			consumer  *QueueConnection
			err       error
		)

		exchangeName := "test-crawler-exchange"
		queueName := "test-crawler-queue"

		BeforeEach(func() {
			publisher, err = NewQueueConnection(amqpAddr)
			Expect(err).To(BeNil())
			Expect(publisher).ToNot(BeNil())

			consumer, err = NewQueueConnection(amqpAddr)
			Expect(err).To(BeNil())
			Expect(consumer).ToNot(BeNil())
		})

		AfterEach(func() {
			defer consumer.Close()
			defer publisher.Close()

			deleted, err := consumer.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			// Consumer cannot delete exchange unless we Cancel() or Close()
			err = publisher.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())
		})

		It("should consume and publish messages onto the provided queue and exchange", func() {
			err = consumer.ExchangeDeclare(exchangeName, "direct")
			Expect(err).To(BeNil())

			_, err = consumer.QueueDeclare(queueName)
			Expect(err).To(BeNil())

			err = consumer.BindQueueToExchange(queueName, exchangeName)
			Expect(err).To(BeNil())

			deliveries, err := consumer.Consume(queueName)
			Expect(err).To(BeNil())

			err = publisher.Publish(exchangeName, "#", "text/plain", "foo")
			Expect(err).To(BeNil())

			for d := range deliveries {
				Expect(string(d.Body)).To(Equal("foo"))
				d.Ack(false)
				break
			}
		})
	})
})

// addrFromURL extracts the addr (host:port) from a URL string.
func addrFromURL(URL string) (string, error) {
	parsedURL, err := url.Parse(URL)
	if err != nil {
		return "", err
	}

	return parsedURL.Host, nil
}

// urlChangeAddr changes the addr (host:port) of a URL string.
func urlChangeAddr(origURL, newHost string) (string, error) {
	parsedURL, err := url.Parse(origURL)
	if err != nil {
		return "", err
	}

	parsedURL.Host = newHost
	return parsedURL.String(), nil
}
