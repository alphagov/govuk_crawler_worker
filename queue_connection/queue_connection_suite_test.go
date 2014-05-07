package queue_connection_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestQueueConnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "QueueConnection Suite")
}
