package http_crawler_test

import (
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"net/http"
)

var _ = Describe("CrawlerResponse", func() {
	It("exposes a way to check if the response body is HTML", func() {
		response := &CrawlerResponse{Body: []byte(`<html><body><p>hi</p></body></html>`)}
		Expect(response.IsBodyHTML()).To(BeTrue())
	})

	Describe("ContentType", func() {
		It("returns the error if we can't parse the content type", func() {
			response := &CrawlerResponse{}
			mime, err := response.ContentType()

			Expect(mime).To(BeEmpty())
			Expect(err).ToNot(BeNil())
		})

		It("returns the simplified mime type of the HTTP Content-Type value", func() {
			response := &CrawlerResponse{Header: make(http.Header)}
			response.Header.Set("Content-Type", "application/json; charset=utf-8")

			mime, err := response.ContentType()

			Expect(mime).To(Equal(JSON))
			Expect(err).To(BeNil())
		})
	})
})
