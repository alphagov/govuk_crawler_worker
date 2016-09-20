package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alphagov/govuk_crawler_worker/queue"
	"github.com/alphagov/govuk_crawler_worker/ttl_hash_set"
)

const (
	// OK is the status of a service when it is operating correctly.
	OK = "ok"
	// Warning is the status of a service when it is still available but some
	// systems may be unavailable or in an incorrect state.
	Warning = "warning"
	// Critical is the status of a service in a critical state.
	Critical = "critical"
)

// Status represents the overall status of the application.
type Status struct {
	Status string           `json:"status"`
	Checks map[string]Check `json:"checks,omitEmpty"`
}

// Check is a single check which is performed as part of the overall system
// status.  All individual check statuses must be `ok` for the overall system
// status to be `ok`.
type Check struct {
	Name   string `json:"-"`
	Status string `json:"status"`
}

// HealthCheck encapsulates the external connections references that are
// required for the application to function, and facilitates checking of the
// status of those connections and, therefore, the overall application health.
type HealthCheck struct {
	port         string
	queueManager *queue.Manager
	ttlHashSet   *ttl_hash_set.TTLHashSet
}

// NewHealthCheck creates a new HealthCheck value.
func NewHealthCheck(queueManager *queue.Manager, ttlHashSet *ttl_hash_set.TTLHashSet) *HealthCheck {
	return &HealthCheck{
		queueManager: queueManager,
		ttlHashSet:   ttlHashSet,
	}
}

// Status returns the overall health status of the application.
func (h *HealthCheck) Status() Status {
	checks := map[string]Check{
		"redis":              h.redisCheck(),
		"rabbitmq_consumer":  h.rabbitConsumerCheck(),
		"rabbitmq_publisher": h.rabbitPublisherCheck(),
	}

	status := OK
	for _, c := range checks {
		if c.Status != OK {
			status = Critical
		}
	}

	return Status{
		Status: status,
		Checks: checks,
	}
}

func (h *HealthCheck) redisCheck() Check {
	pong, err := h.ttlHashSet.Ping()
	status := Critical

	if err == nil && pong == "PONG" {
		status = OK
	}

	return Check{
		Name:   "redis",
		Status: status,
	}
}

func (h *HealthCheck) rabbitConsumerCheck() Check {
	consumerInspect, err := h.queueManager.Consumer.Channel.QueueInspect(h.queueManager.QueueName)
	status := Critical

	if err == nil && consumerInspect.Name == h.queueManager.QueueName {
		status = OK
	}

	return Check{
		Name:   "rabbitmq_consumer",
		Status: status,
	}
}

func (h *HealthCheck) rabbitPublisherCheck() Check {
	publisherInspect, err := h.queueManager.Producer.Channel.QueueInspect(h.queueManager.QueueName)
	status := Critical

	if err == nil && publisherInspect.Name == h.queueManager.QueueName {
		status = OK
	}

	return Check{
		Name:   "rabbitmq_publisher",
		Status: status,
	}
}

// HTTPHandler is a handler function for serving up the application healthcheck
// status.
func (h *HealthCheck) HTTPHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		status := h.Status()
		encoder := json.NewEncoder(w)

		err := encoder.Encode(&status)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot encode response data: %v", err),
				http.StatusInternalServerError)
		}
	}
}
