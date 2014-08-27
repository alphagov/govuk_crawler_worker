package queue_test

import (
	. "github.com/alphagov/govuk_crawler_worker/queue"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"net/url"

	"github.com/alphagov/govuk_crawler_worker/util"
	"github.com/michaelklishin/rabbit-hole"
	"github.com/streadway/amqp"
)

var _ = Describe("QueueConnection", func() {
	amqpAddr := util.GetEnvDefault("AMQP_ADDRESS", "amqp://guest:guest@localhost:5672/")

	It("fails if it can't connect to an AMQP server", func() {
		connection, err := NewQueueConnection("amqp://guest:guest@localhost:50000/")

		Expect(err).ToNot(BeNil())
		Expect(connection).To(BeNil())
	})

	Describe("Connection errors", func() {
		var (
			connection       *QueueConnection
			proxy            *util.ProxyTCP
			proxyAddr        string           = "localhost:5673"
			queueName        string           = "govuk_crawler_worker-test-crawler-queue"
			fatalErrs        chan *amqp.Error = make(chan *amqp.Error)
			channelCloseMsgs chan string      = make(chan string)
		)

		BeforeEach(func() {
			proxyDest, err := addrFromURL(amqpAddr)
			Expect(err).To(BeNil())
			proxyURL, err := urlChangeAddr(amqpAddr, proxyAddr)
			Expect(err).To(BeNil())

			proxy, err = util.NewProxyTCP(proxyAddr, proxyDest)
			Expect(err).To(BeNil())
			Expect(proxy).ToNot(BeNil())

			connection, err = NewQueueConnection(proxyURL)
			Expect(err).To(BeNil())
			Expect(connection).ToNot(BeNil())

			connection.HandleFatalError = func(err *amqp.Error) {
				fatalErrs <- err
			}

			connection.HandleChannelClose = func(message string) {
				channelCloseMsgs <- message
			}

			_, err = connection.QueueDeclare(queueName)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			defer connection.Close()
			defer proxy.Close()

			// Assume existing connection is dead.
			connection.Close()
			connection, _ = NewQueueConnection(amqpAddr)

			deleted, err := connection.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))
		})

		It("should call connection.HandleChannelClose() on recoverable errors", func(done Done) {
			connection.Channel.Close()

			// check connection.HandleChannelClose is called
			message := <-channelCloseMsgs
			Expect(message).To(Equal("Channel closed"))

			// Connection no longer works
			_, err := connection.Channel.QueueInspect(queueName)
			Expect(err).To(Equal(amqp.ErrClosed))

			close(done)
		})

		It("should exit if server closes connection", func(done Done) {
			expectedError := "Exception (320) Reason: \"CONNECTION_FORCED - Closed via management plugin\""

			rmqc, err := rabbithole.NewClient("http://127.0.0.1:15672", "guest", "guest")
			Expect(err).To(BeNil())

			connections, err := rmqc.ListConnections()
			Expect(err).To(BeNil())

			for x := range connections {
				_, err := rmqc.CloseConnection(connections[x].Name)
				Expect(err).To(BeNil())
			}

			// We'd normally log.Fatalln() here to exit.
			amqpErr := <-fatalErrs
			Expect(amqpErr.Error()).To(Equal(expectedError))
			Expect(amqpErr.Recover).To(BeFalse())
			Expect(amqpErr.Server).To(BeTrue())

			// Connection no longer works
			_, err = connection.Channel.QueueInspect(queueName)
			Expect(err).To(Equal(amqp.ErrClosed))

			close(done)
		})

		It("should exit on non-recoverable errors", func(done Done) {
			expectedError := "Exception \\(501\\) Reason: \"EOF\"|connection reset by peer"

			proxy.KillConnected()

			_, err := connection.Channel.QueueInspect(queueName)
			Expect(err.Error()).To(MatchRegexp(expectedError))

			// We'd normally log.Fatalln() here to exit.
			amqpErr := <-fatalErrs
			Expect(amqpErr.Error()).To(MatchRegexp(expectedError))
			Expect(amqpErr.Recover).To(BeFalse())

			// Connection no longer works.
			_, err = connection.Channel.QueueInspect(queueName)
			Expect(err).To(Equal(amqp.ErrClosed))

			close(done)
		})
	})

	Describe("Connecting to a running AMQP service", func() {
		var (
			connection    *QueueConnection
			connectionErr error
		)

		BeforeEach(func() {
			connection, connectionErr = NewQueueConnection(amqpAddr)
			connection.HandleChannelClose = func(_ string) {}
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
			exchange := "govuk_crawler_worker-some-exchange"

			err = connection.ExchangeDeclare(exchange, "direct")
			Expect(err).To(BeNil())

			err = connection.Channel.ExchangeDelete(exchange, false, false)
			Expect(err).To(BeNil())
		})

		It("can declare a queue", func() {
			var (
				err   error
				queue amqp.Queue
				name  = "govuk_crawler_worker-some-queue"
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

			exchangeName := "govuk_crawler_worker-some-binding-exchange"
			queueName := "govuk_crawler_worker-some-binding-queue"

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

		exchangeName := "govuk_crawler_worker-test-crawler-exchange"
		queueName := "govuk_crawler_worker-test-crawler-queue"

		BeforeEach(func() {
			publisher, err = NewQueueConnection(amqpAddr)
			Expect(err).To(BeNil())
			Expect(publisher).ToNot(BeNil())

			consumer, err = NewQueueConnection(amqpAddr)
			Expect(err).To(BeNil())
			Expect(consumer).ToNot(BeNil())

			publisher.HandleChannelClose = func(_ string) {}
			consumer.HandleChannelClose = func(_ string) {}
		})

		AfterEach(func() {
			// Consumer must Cancel() or Close() before deleting.
			consumer.Close()
			defer publisher.Close()

			deleted, err := publisher.Channel.QueueDelete(queueName, false, false, false)
			Expect(err).To(BeNil())
			Expect(deleted).To(Equal(0))

			err = publisher.Channel.ExchangeDelete(exchangeName, false, false)
			Expect(err).To(BeNil())
		})

		It("should consume and publish messages onto the provided queue and exchange", func(done Done) {
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

			item := <-deliveries
			Expect(string(item.Body)).To(Equal("foo"))
			item.Ack(false)
			close(done)
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
