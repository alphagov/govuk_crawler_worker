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
	var expectedFilePath, mirrorRoot, testUrl, urlPath string
	var delivery amqp.Delivery
	var err error
	var html []byte
	var item *CrawlerMessageItem
	var rootURL *url.URL

	BeforeEach(func() {
		mirrorRoot = os.Getenv("MIRROR_ROOT")
		if mirrorRoot == "" {
			mirrorRoot, err = ioutil.TempDir("", "crawler_message_item_test")
			Expect(err).To(BeNil())
		}

		rootURL, _ = url.Parse("https://www.gov.uk")
		testUrl = rootURL.String() + urlPath
		urlPath = "/government/organisations"
		expectedFilePath = "government/organisations.html"

		delivery = amqp.Delivery{Body: []byte(testUrl)}
		item = NewCrawlerMessageItem(delivery, rootURL, []string{})

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
		Expect(NewCrawlerMessageItem(delivery, rootURL, []string{})).ToNot(BeNil())
	})

	Describe("getting and setting the HTMLBody", func() {
		It("can get the HTMLBody of the crawled URL", func() {
			Expect(item.HTMLBody).To(Equal(html))
		})

		It("can set the HTMLBody of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, rootURL, []string{})
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
			testUrl = rootURL.String() + "/../!T@e£s$t/U^R*L(){}"
			expectedFilePath = "test/url.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("adds an index.html suffix when URL references a directory", func() {
			testUrl = rootURL.String() + "/this/url/has/a/trailing/slash/"
			expectedFilePath = "this/url/has/a/trailing/slash/index.html"
			delivery = amqp.Delivery{Body: []byte(testUrl)}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.HTMLBody = html

			Expect(item.RelativeFilePath()).To(Equal(expectedFilePath))
		})
		It("adds an index.html suffix when URL has no path and no trailing slash", func() {
			testUrl = rootURL.String() + "/"
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
		It("should return an empty array if it can't find any matching URLs", func() {
			item.HTMLBody = []byte("")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(Equal([]*url.URL{}))
		})

		It("should extract all a[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><a href="https://www.gov.uk/"></a></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all img[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<div><img src="https://www.gov.uk/image.png" /></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/image.png")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all link[@href] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(`<head><link rel="icon" href="https://www.gov.uk/favicon.ico"></head>`)
			expectedUrl, _ := url.Parse("https://www.gov.uk/favicon.ico")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all script[@src] URLs from a given HTML document", func() {
			item.HTMLBody = []byte(
				`<head><script type="text/javascript" src="https://www.gov.uk/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/jq.js")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("successfully extracts multiple matching URLs from the provided DOM", func() {
			item.HTMLBody = []byte(
				`<head>
<script type="text/javascript" src="https://www.gov.uk/jq.js"></script>
<link rel="icon" href="https://www.gov.uk/favicon.ico">
</head>`)
			urls, err := item.ExtractURLs()
			expectedUrl1, _ := url.Parse("https://www.gov.uk/jq.js")
			expectedUrl2, _ := url.Parse("https://www.gov.uk/favicon.ico")

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
			item.HTMLBody = []byte(`<div><a href="http://www.gov.uk/bar%20"></a></div>`)
			expectedUrl, _ := url.Parse("http://www.gov.uk/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract relative URLs", func() {
			item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="mailto:c@d.com">b</a></div>`)
			expectedUrl, _ := url.Parse("https://www.gov.uk/foo/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should remove the #fragment when extracting URLs", func() {
			item.HTMLBody = []byte(`<div><a href="http://www.gov.uk/#germany"></a></div>`)
			expectedUrl, _ := url.Parse("http://www.gov.uk/")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})
	})

	It("removes paths that are blacklisted", func() {
		item := NewCrawlerMessageItem(delivery, rootURL, []string{"/trade-tariff"})
		item.HTMLBody = []byte(`<div><a href="/foo/bar">a</a><a href="/trade-tariff">b</a></div>`)

		urls, err := item.ExtractURLs()

		Expect(err).To(BeNil())
		Expect(len(urls)).To(Equal(1))
	})
})
