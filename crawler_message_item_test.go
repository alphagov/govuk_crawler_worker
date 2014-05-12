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
		Expect(NewCrawlerMessageItem(delivery, "www.gov.uk")).
			ToNot(BeNil())
	})

	Describe("getting and setting the HTMLBody", func() {
		It("can get the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, "www.gov.uk")
			Expect(item.HTMLBody).To(BeNil())
		})

		It("can set the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, "www.gov.uk")
			item.HTMLBody = []byte("foo")

			Expect(item.HTMLBody).To(Equal([]byte("foo")))
		})
	})

	It("is able to state whether the content type is HTML", func() {
		item := NewCrawlerMessageItem(delivery, "www.gov.uk")
		item.HTMLBody = []byte(`
<html>
<head><title>test</title</head>
<body><h1>TEST</h1></body>
</html>
`)

		Expect(item.IsHTML()).To(BeTrue())
	})

	Describe("ExtractURLs", func() {
		var item *CrawlerMessageItem

		BeforeEach(func() {
			delivery := amqp.Delivery{Body: []byte("https://www.foo.com/")}
			item = NewCrawlerMessageItem(delivery, "www.foo.com")
		})

		It("should return an empty array if it can't find any matching URLs", func() {
			item.HTMLBody = []byte("")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(Equal([]string{}))
		})

		It("should extract all a[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><a href="https://www.foo.com/"></a></div>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("https://www.foo.com/"))
		})

		It("should extract all img[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><img src="https://www.foo.com/image.png" /></div>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("https://www.foo.com/image.png"))
		})

		It("should extract all link[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<head><link rel="icon" href="https://www.foo.com/favicon.ico"></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("https://www.foo.com/favicon.ico"))
		})

		It("should extract all script[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(
				`<head><script type="text/javascript" src="https://www.foo.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("https://www.foo.com/jq.js"))
		})

		It("successfully extracts multiple matching URLs from the provided DOM", func() {
			item.HTMLBody = []byte(
				`<head>
<script type="text/javascript" src="https://www.foo.com/jq.js"></script>
<link rel="icon" href="https://www.foo.com/favicon.ico">
</head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("https://www.foo.com/jq.js"))
			Expect(urls).To(ContainElement("https://www.foo.com/favicon.ico"))
		})

		It("will not provide URLs that don't match the provided prefix host", func() {
			item.HTMLBody = []byte(
				`<head><script type="text/javascript" src="https://www.foobar.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(BeEmpty())
		})

		It("will unescape URLs", func() {
			item.HTMLBody = []byte(`<div><a href="http://www.foo.com/bar%20"></a></div>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement("http://www.foo.com/bar"))
		})

		It("should extract relative URLs", func() {
			item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="mailto:c@d.com">b</a></div>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
			Expect(urls).To(ContainElement("http://www.foo.com/foo/bar"))
		})
	})
})
