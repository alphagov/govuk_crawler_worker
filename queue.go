package govuk_crawler_worker

import (
	"github.com/streadway/amqp"
)

type QueueConnection struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
}

func NewQueueConnection(amqpURI string) (*QueueConnection, error) {
	connection, err := amqp.Dial(amqpURI)
	if err != nil {
		return nil, err
	}

	channel, err := connection.Channel()
	if err != nil {
		return nil, err
	}

	return &QueueConnection{
		Connection: connection,
		Channel: channel,
	}, nil
}

func (c *QueueConnection) Close() error {
	return c.Connection.Close()
}

func (c *QueueConnection) ExchangeDeclare(exchangeName string, exchangeType string) error {
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

func (c *QueueConnection) QueueDeclare(name string) (amqp.Queue, error) {
	queue, err := c.Channel.QueueDeclare(
		name,  // name of the queue
		true,  // durable
		false, // delete when usused
		false, // exclusive
		false, // noWait
		map[string]string{"": ""})
	if err != nil {
		return amqp.Queue{
			Name: name,
		}, err
	}

	return queue, nil
}
