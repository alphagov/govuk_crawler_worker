package main_test

import (
	"io/ioutil"
	"net/url"

	. "github.com/alphagov/govuk_crawler_worker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"

	"github.com/streadway/amqp"
)

var _ = Describe("CrawlerMessageItem", func() {
	var rootURL, expectedFilePath, mirrorRoot, testUrl, urlPath string
	var delivery amqp.Delivery
	var err error
	var html []byte
	var item *CrawlerMessageItem

	BeforeEach(func() {
		mirrorRoot = os.Getenv("MIRROR_ROOT")
		if mirrorRoot == "" {
			mirrorRoot, err = ioutil.TempDir("", "crawler_message_item_test")
			Expect(err).To(BeNil())
		}

		rootURL = "https://www.gov.uk"

		testUrl = baseUrl + urlPath
		urlPath = "/government/organisations"
		expectedFilePath = "government/organisations.html"

		delivery = amqp.Delivery{Body: []byte(testUrl)}
		item = NewCrawlerMessageItem(delivery, baseUrl, []string{})

		html = []byte(`<html>
<head><title>test</title</head>
<body><h1>TEST</h1></body>
</html>
`)
		item.HTMLBody = html
	})

	AfterEach(func() {
		DeleteMirrorFilesFromDisk(mirrorRoot)
	})

	It("generates a CrawlerMessageItem object", func() {
		Expect(NewCrawlerMessageItem(delivery, baseUrl, []string{})).
			ToNot(BeNil())
	})

	Describe("getting and setting the HTMLBody", func() {
		It("can get the HTMLBody of the crawled URL", func() {
			Expect(item.HTMLBody).To(Equal(html))
		})

		It("can set the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, baseUrl, []string{})
			item.HTMLBody = []byte("foo")

			Expect(item.HTMLBody).To(Equal([]byte("foo")))
		})
	})

	It("is able to state whether the content type is HTML", func() {
		Expect(item.IsHTML()).To(BeTrue())
	})

	It("returns its URL", func() {
		Expect(item.URL()).To(Equal(testUrl))
	})

	Describe("generating a sane filename", func() {
		It("strips out the domain, protocol, auth and ports", func() {
			testUrl = "https://user:pass@example.com:8080/test/url"
			expectedFilePath = "test/url.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("strips illegal characters", func() {
			testUrl = baseUrl + "/../!T@e£s$t/U^R*L(){}"
			expectedFilePath = "test/url.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("adds an index.html suffix when URL references a directory", func() {
			testUrl = baseUrl + "/this/url/has/a/trailing/slash/"
			expectedFilePath = "this/url/has/a/trailing/slash/index.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("adds an index.html suffix when URL has no path and no trailing slash", func() {
			testUrl = baseUrl + "/"
			expectedFilePath = "index.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("omits URL query parameters", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "?foo=bar")}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("omits URL fragments", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "#foo")}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
	})

	Describe("ExtractURLs", func() {
		var item *CrawlerMessageItem

		BeforeEach(func() {
			delivery := amqp.Delivery{Body: []byte("https://www.foo.com/")}
			item = NewCrawlerMessageItem(delivery, "https://www.foo.com/", []string{})
		})

		It("should return an empty array if it can't find any matching URLs", func() {
			item.HTMLBody = []byte("")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(Equal([]*url.URL{}))
		})

		It("should extract all a[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><a href="https://www.foo.com/"></a></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.foo.com/")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all img[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><img src="https://www.foo.com/image.png" /></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.foo.com/image.png")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all link[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<head><link rel="icon" href="https://www.foo.com/favicon.ico"></head>`)
			expectedUrl, _ := url.Parse("https://www.foo.com/favicon.ico")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all script[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(
				`<head><script type="text/javascript" src="https://www.foo.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.foo.com/jq.js")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("successfully extracts multiple matching URLs from the provided DOM", func() {
			item.HTMLBody = []byte(
				`<head>
<script type="text/javascript" src="https://www.foo.com/jq.js"></script>
<link rel="icon" href="https://www.foo.com/favicon.ico">
</head>`)
			urls, err := item.ExtractURLs()
			expectedUrl1, _ := url.Parse("https://www.foo.com/jq.js")
			expectedUrl2, _ := url.Parse("https://www.foo.com/favicon.ico")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl1))
			Expect(urls).To(ContainElement(expectedUrl2))
		})

		It("will not provide URLs that don't match the provided prefix rootURL", func() {
			item.HTMLBody = []byte(
				`<head><script type="text/javascript" src="https://www.foobar.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(BeEmpty())
		})

		It("will unescape URLs", func() {
			item.HTMLBody = []byte(`<div><a href="http://www.foo.com/bar%20"></a></div>`)
			expectedUrl, _ := url.Parse("http://www.foo.com/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract relative URLs", func() {
			item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="mailto:c@d.com">b</a></div>`)
			expectedUrl, _ := url.Parse("https://www.foo.com/foo/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
			Expect(urls).To(ContainElement(expectedUrl))
		})
	})

	It("removes paths that are blacklisted", func() {
		item := NewCrawlerMessageItem(delivery, baseUrl, []string{"/trade-tariff"})
		item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="/trade-tariff">b</a></div>`)

		urls, err := item.ExtractURLs()

		Expect(err).To(BeNil())
		Expect(len(urls)).To(Equal(1))
	})
})
