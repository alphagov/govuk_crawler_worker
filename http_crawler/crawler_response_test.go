package http_crawler_test

import (
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"net/http"
)

var _ = Describe("CrawlerResponse", func() {
	Describe("AcceptedContentType", func() {
		var response *CrawlerResponse

		BeforeEach(func() {
			response = &CrawlerResponse{Header: make(http.Header)}
		})

		It("doesn't support audio content types", func() {
			response.Header.Set("Content-Type", "audio/mpeg")
			Expect(response.AcceptedContentType()).To(BeFalse())
		})

		It("accepts a known set of content types", func() {
			for _, contentType := range []string{
				// Provide one with a charset to be sure.
				"text/html; charset=utf-8",
				ATOM, CSV, DOCX, HTML, ICS, JSON, ODP, ODS, ODT, PDF, XLS, XLSX,
			} {
				response.Header.Set("Content-Type", contentType)
				Expect(response.AcceptedContentType()).To(BeTrue())
			}
		})
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
