package queue

type QueueManager struct {
	ExchangeName string
	QueueName    string

	consumer *QueueConnection
	producer *QueueConnection
}

func NewQueueManager(amqpAddr string, exchangeName string, queueName string) (*QueueManager, error) {
	consumer, err := NewQueueConnection(amqpAddr)
	if err != nil {
		return nil, err
	}

	producer, err := NewQueueConnection(amqpAddr)
	if err != nil {
		return nil, err
	}

	return &QueueManager{
		ExchangeName: exchangeName,
		QueueName:    queueName,

		consumer: consumer,
		producer: producer,
	}, nil
}

func (h *QueueManager) Close() error {
	err := h.producer.Close()
	if err != nil {
		defer h.consumer.Close()
		return err
	}

	return h.consumer.Close()
}
