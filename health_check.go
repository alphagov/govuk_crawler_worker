package main

import (
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
)

type Status struct {
	AMQP  bool `json:"amqp"`
	Redis bool `json:"redis"`
}

func HealthCheck(queueManager *queue.QueueManager, ttlHashSet *ttl_hash_set.TTLHashSet) *Status {
	var amqpStatus, redisStatus bool

	pong, err := ttlHashSet.Ping()
	if err == nil && pong == "PONG" {
		redisStatus = true
	}

	inspect, err := queueManager.Consumer.Channel.QueueInspect(queueManager.QueueName)
	if err == nil && inspect.Name == queueManager.QueueName {
		amqpStatus = true
	}

	return &Status{
		AMQP:  amqpStatus,
		Redis: redisStatus,
	}
}
