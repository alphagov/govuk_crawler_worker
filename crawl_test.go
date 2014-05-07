package govuk_crawler_worker_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Crawl", func() {
	Describe("RetryStatusCodes", func() {
		It("should return a fixed int array with values 429, 500..599", func() {
			statusCodes := RetryStatusCodes()

			Expect(len(statusCodes)).To(Equal(101))
			Expect(statusCodes[0]).To(Equal(429))
			Expect(statusCodes[1]).To(Equal(500))
			Expect(statusCodes[100]).To(Equal(599))
		})
	})
})
