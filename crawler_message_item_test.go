package main_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/streadway/amqp"
)

var _ = Describe("CrawlerMessageItem", func() {
	delivery := amqp.Delivery{Body: []byte("https://www.gov.uk/")}

	It("generates a CrawlerMessageItem object", func() {
		Expect(NewCrawlerMessageItem(delivery)).
			ToNot(BeNil())
	})

	Describe("getting and setting the HTMLBody", func() {
		It("can get the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery)
			Expect(item.HTMLBody).To(BeNil())
		})

		It("can set the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery)
			item.HTMLBody = []byte("foo")

			Expect(item.HTMLBody).To(Equal([]byte("foo")))
		})
	})
})
