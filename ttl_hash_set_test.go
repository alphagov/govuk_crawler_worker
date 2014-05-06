package govuk_crawler_worker_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TTLHashSet", func() {
	It("returns an error when asking for a TTLHashSet object that can't connect to redis", func() {
		ttlHashSet, err := NewTTLHashSet("govuk_mirror_crawler", "127.0.0.1:20000")

		Expect(err).ToNot(BeNil())
		Expect(ttlHashSet).To(BeNil())
	})

	Describe("Working with a redis service", func() {
		var (
			ttlHashSet    *TTLHashSet
			ttlHashSetErr error
		)

		BeforeEach(func() {
			ttlHashSet, ttlHashSetErr = NewTTLHashSet("govuk_mirror_crawler", "127.0.0.1:6379")
		})

		AfterEach(func() {
			Expect(ttlHashSet.Close()).To(BeNil())
		})

		It("should connect successfully with no errors", func() {
			Expect(ttlHashSetErr).To(BeNil())
			Expect(ttlHashSet).NotTo(BeNil())
		})
	})
})
