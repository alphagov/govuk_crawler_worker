package queue

import (
	"log"

	"github.com/streadway/amqp"
)

type Connection struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel

	HandleChannelClose func(message string)
	HandleFatalError   func(err *amqp.Error)

	notifyClose chan *amqp.Error
}

func NewConnection(amqpURI string) (*Connection, error) {
	connection, err := amqp.Dial(amqpURI)
	if err != nil {
		return nil, err
	}

	channel, err := connection.Channel()
	if err != nil {
		return nil, err
	}

	err = channel.Qos(5, 0, false)
	if err != nil {
		return nil, err
	}

	queueConnection := &Connection{
		Connection:         connection,
		Channel:            channel,
		HandleChannelClose: func(message string) { log.Fatalln(message) },
		HandleFatalError:   func(err *amqp.Error) { log.Fatalln(err) },
		notifyClose:        channel.NotifyClose(make(chan *amqp.Error)),
	}

	go func() {
		select {
		case e, ok := <-queueConnection.notifyClose:
			if e != nil && !e.Recover {
				queueConnection.HandleFatalError(e)
			}

			if !ok {
				queueConnection.HandleChannelClose("Channel closed")
			}
		}
	}()

	return queueConnection, nil
}

func (c *Connection) Close() error {
	err := c.Channel.Close()
	if err != nil {
		return err
	}

	return c.Connection.Close()
}

func (c *Connection) Consume(queueName string) (<-chan amqp.Delivery, error) {
	return c.Channel.Consume(
		queueName,
		"",
		false, // autoAck
		false, // this won't be the sole consumer
		true,  // don't deliver messages from same connection
		false, // the broker owns when consumption can begin
		nil)   // arguments
}

func (c *Connection) ExchangeDeclare(exchangeName string, exchangeType string) error {
	return c.Channel.ExchangeDeclare(
		exchangeName, // name of the exchange
		exchangeType, // type
		true,         // durable
		false,        // delete when complete
		false,        // internal
		false,        // noWait
		nil,          // arguments
	)
}

func (c *Connection) QueueDeclare(queueName string) (amqp.Queue, error) {
	queue, err := c.Channel.QueueDeclare(
		queueName, // name of the queue
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // noWait
		nil)       // arguments
	if err != nil {
		return amqp.Queue{
			Name: queueName,
		}, err
	}

	return queue, nil
}

func (c *Connection) BindQueueToExchange(queueName string, exchangeName string) error {
	return c.Channel.QueueBind(
		queueName,
		"#", // key to marshall with
		exchangeName,
		true, // noWait
		nil)  // arguments
}

func (c *Connection) Publish(exchangeName string, routingKey string, contentType string, body string) error {
	return c.Channel.Publish(
		exchangeName, // publish to an exchange
		routingKey,   // routing to 0 or more queues
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     contentType,
			ContentEncoding: "",
			Body:            []byte(body),
			DeliveryMode:    amqp.Persistent,
			Priority:        0, // 0-9
		})
}
