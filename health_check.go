package main

import (
	"github.com/alphagov/govuk_crawler_worker/healthcheck"
	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
)

// NewHealthCheck creates a new healthcheck.healthCheck value with all relevant
// checks already defined and configured.
func NewHealthCheck(queueManager *queue.Manager, ttlHashSet *ttl_hash_set.TTLHashSet) *healthcheck.HealthCheck {
	return healthcheck.NewHealthCheck(
		redisChecker{ttlHashSet},
		rabbitConsumerChecker{queueManager},
		rabbitPublisherChecker{queueManager},
	)
}

// A healthcheck.Checker that reports if the application can talk to Redis.
type redisChecker struct {
	ttlHashSet *ttl_hash_set.TTLHashSet
}

func (r redisChecker) Name() string {
	return "redis"
}

func (r redisChecker) Check() (healthcheck.StatusEnum, error) {
	pong, err := r.ttlHashSet.Ping()
	status := healthcheck.Critical

	if err == nil && pong == "PONG" {
		status = healthcheck.OK
	}

	return status, err
}

// A healthcheck.Checker that reports if the application can consume messages
// from a RabbitMQ queue.
type rabbitConsumerChecker struct {
	queueManager *queue.Manager
}

func (r rabbitConsumerChecker) Name() string {
	return "rabbitmq_consumer"
}

func (r rabbitConsumerChecker) Check() (healthcheck.StatusEnum, error) {
	consumerInspect, err := r.queueManager.Consumer.Channel.QueueInspect(r.queueManager.QueueName)
	status := healthcheck.Critical

	if err == nil && consumerInspect.Name == r.queueManager.QueueName {
		status = healthcheck.OK
	}

	return status, err
}

// A healthcheck.Checker that reports if the application can publisher messages
// to a RabbitMQ exchange.
type rabbitPublisherChecker struct {
	queueManager *queue.Manager
}

func (r rabbitPublisherChecker) Name() string {
	return "rabbitmq_publisher"
}

func (r rabbitPublisherChecker) Check() (healthcheck.StatusEnum, error) {
	publisherInspect, err := r.queueManager.Producer.Channel.QueueInspect(r.queueManager.QueueName)
	status := healthcheck.Critical

	if err == nil && publisherInspect.Name == r.queueManager.QueueName {
		status = healthcheck.OK
	}

	return status, err
}
