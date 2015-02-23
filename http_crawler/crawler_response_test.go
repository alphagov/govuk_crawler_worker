package http_crawler_test

import (
	"github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CrawlerResponse", func() {
	var response *http_crawler.CrawlerResponse

	BeforeEach(func() {
		response = &http_crawler.CrawlerResponse{}
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
				http_crawler.ATOM,
				http_crawler.CSS,
				http_crawler.CSV,
				http_crawler.DOCX,
				http_crawler.GIF,
				http_crawler.HTML,
				http_crawler.ICO,
				http_crawler.ICS,
				http_crawler.JAVASCRIPT,
				http_crawler.JPEG,
				http_crawler.JSON,
				http_crawler.ODP,
				http_crawler.ODS,
				http_crawler.ODT,
				http_crawler.PDF,
				http_crawler.PNG,
				http_crawler.XLS,
				http_crawler.XLSX,
			} {
				response.ContentType = contentType
				Expect(response.AcceptedContentType()).To(BeTrue())
			}
		})
	})

	Describe("ContentType", func() {
		It("returns the error if we can't parse the content type", func() {
			response := &http_crawler.CrawlerResponse{}
			mime, err := response.ParseContentType()

			Expect(mime).To(BeEmpty())
			Expect(err).ToNot(BeNil())
		})

		It("returns the simplified mime type of the HTTP Content-Type value", func() {
			response := &http_crawler.CrawlerResponse{ContentType: "application/json; charset=utf-8"}

			mime, err := response.ParseContentType()

			Expect(mime).To(Equal(http_crawler.JSON))
			Expect(err).To(BeNil())
		})
	})
})
