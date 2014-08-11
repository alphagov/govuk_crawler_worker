package http_crawler_test

import (
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CrawlerResponse", func() {
	It("exposes a way to check if the response body is HTML", func() {
		response := &CrawlerResponse{Body: []byte(`<html><body><p>hi</p></body></html>`)}
		Expect(response.IsBodyHTML()).To(BeTrue())
	})
})
