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
