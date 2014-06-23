package main_test

import (
	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/streadway/amqp"
	"os"
)

var _ = Describe("CrawlerMessageItem", func() {
	var baseUrl, expectedFileName, host, mirrorRoot, testUrl, urlPath string
	var delivery amqp.Delivery
	var html []byte
	var item *CrawlerMessageItem

	BeforeEach(func() {
		mirrorRoot = os.Getenv("MIRROR_ROOT")
		host = "www.gov.uk"
		baseUrl = "https://" + host

		testUrl = baseUrl + urlPath
		urlPath = "/government/organisations"
		expectedFileName = mirrorRoot + urlPath + ".html"

		delivery = amqp.Delivery{Body: []byte(testUrl)}
		item = NewCrawlerMessageItem(delivery, host, []string{})

		html = []byte(`<html>
<head><title>test</title</head>
<body><h1>TEST</h1></body>
</html>
`)
		item.HTMLBody = html
	})

	It("generates a CrawlerMessageItem object", func() {
		Expect(NewCrawlerMessageItem(delivery, host, []string{})).
			ToNot(BeNil())
	})

	Describe("getting and setting the HTMLBody", func() {
		It("can get the HTMLBody of the crawled URL", func() {
			Expect(item.HTMLBody).To(Equal(html))
		})

		It("can set the HTMLBody of the crawled URL", func() {
			item.HTMLBody = []byte("foo")

			Expect(item.HTMLBody).To(Equal([]byte("foo")))
		})
	})

	It("is able to state whether the content type is HTML", func() {
		Expect(item.IsHTML()).To(BeTrue())
	})

	It("returns its URL", func() {
		item := NewCrawlerMessageItem(delivery, host, []string{})
		Expect(item.URL()).To(Equal(testUrl))
	})

	Describe("writing crawled content to disk", func() {
		It("wrote something to disk", func() {
			fileName, _ := item.WriteToDisk()
			Expect(fileName).To(Equal(expectedFileName))
		})
	})

	Describe("generating a sane filename", func() {
		It("strips out the domain, protocol, auth and ports", func() {
			testUrl = "https://user:pass@example.com:8080/test/url"
			expectedFileName = mirrorRoot + "/test/url"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, host, []string{})

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
		It("strips illegal characters", func() {
			testUrl = baseUrl + "/../!t@e£s$t/u^r*l(){}"
			expectedFileName = mirrorRoot + "/test/url"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, host, []string{})

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
		It("adds an index.html suffix when URL references a directory", func() {
			testUrl = baseUrl + "/this/url/has/a/trailing/slash/"
			expectedFileName = mirrorRoot + "/this/url/has/a/trailing/slash/index.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, host, []string{})
			item.HTMLBody = html

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
		It("adds an index.html suffix when URL has no path and no trailing slash", func() {
			testUrl = baseUrl
			expectedFileName = mirrorRoot + "/index.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, host, []string{})
			item.HTMLBody = html

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
		It("omits URL query parameters", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "?foo=bar")}
			item = NewCrawlerMessageItem(delivery, host, []string{})
			item.HTMLBody = html

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
		It("omits URL fragments", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "#foo")}
			item = NewCrawlerMessageItem(delivery, host, []string{})
			item.HTMLBody = html

			Expect(item.FileName()).To(Equal(expectedFileName))
		})
	})

	Describe("ExtractURLs", func() {
		var item *CrawlerMessageItem

		BeforeEach(func() {
			delivery := amqp.Delivery{Body: []byte("https://www.foo.com/")}
			item = NewCrawlerMessageItem(delivery, "www.foo.com", []string{})
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

	It("removes paths that are blacklisted", func() {
		item := NewCrawlerMessageItem(delivery, host, []string{"/trade-tariff"})
		item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="/trade-tariff">b</a></div>`)

		urls, err := item.ExtractURLs()

		Expect(err).To(BeNil())
		Expect(len(urls)).To(Equal(1))
	})
})
