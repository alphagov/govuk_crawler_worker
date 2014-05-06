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
})
