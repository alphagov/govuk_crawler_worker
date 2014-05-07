package ttl_hash_set_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestTtlHashSet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TtlHashSet Suite")
}
