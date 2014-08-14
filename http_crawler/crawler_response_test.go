package http_crawler_test

import (
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CrawlerResponse", func() {
	var response *CrawlerResponse

	BeforeEach(func() {
		response = &CrawlerResponse{}
	})

	Describe("AcceptedContentType", func() {
		It("doesn't support audio content types", func() {
			response.ContentType = "audio/mpeg"
			Expect(response.AcceptedContentType()).To(BeFalse())
		})

		It("accepts a known set of content types", func() {
			for _, contentType := range []string{
				// Provide one with a charset to be sure.
				"text/html; charset=utf-8",
				ATOM, CSS, CSV, DOCX, GIF, HTML, ICO, ICS, JAVASCRIPT,
				JPEG, JSON, ODP, ODS, ODT, PDF, PNG, XLS, XLSX,
			} {
				response.ContentType = contentType
				Expect(response.AcceptedContentType()).To(BeTrue())
			}
		})
	})

	Describe("ContentType", func() {
		It("returns the error if we can't parse the content type", func() {
			response := &CrawlerResponse{}
			mime, err := response.ParseContentType()

			Expect(mime).To(BeEmpty())
			Expect(err).ToNot(BeNil())
		})

		It("returns the simplified mime type of the HTTP Content-Type value", func() {
			response := &CrawlerResponse{ContentType: "application/json; charset=utf-8"}

			mime, err := response.ParseContentType()

			Expect(mime).To(Equal(JSON))
			Expect(err).To(BeNil())
		})
	})
})
