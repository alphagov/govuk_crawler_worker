package main_test

import (
	"io/ioutil"
	"net/http"
	"net/url"

	. "github.com/alphagov/govuk_crawler_worker"
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"

	"github.com/streadway/amqp"
)

var _ = Describe("CrawlerMessageItem", func() {
	var mirrorRoot, testUrl, urlPath string
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

		delivery = amqp.Delivery{Body: []byte(testUrl)}
		item = NewCrawlerMessageItem(delivery, rootURL, []string{})

		html = []byte(`<html>
<head><title>test</title</head>
<body><h1>TEST</h1></body>
</html>
`)
		item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
	})

	AfterEach(func() {
		DeleteMirrorFilesFromDisk(mirrorRoot)
	})

	It("generates a CrawlerMessageItem object", func() {
		Expect(NewCrawlerMessageItem(delivery, rootURL, []string{})).ToNot(BeNil())
	})

	Describe("getting and setting the Response.Body", func() {
		It("can get the Response.Body of the crawled URL", func() {
			Expect(item.Response.Body).To(Equal(html))
		})

		It("can set the Response.Body of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Body: []byte("foo")}

			Expect(item.Response.Body).To(Equal([]byte("foo")))
		})
	})

	It("detects when a URL is blacklisted", func() {
		delivery = amqp.Delivery{Body: []byte("https://www.example.com/blacklisted")}
		item := NewCrawlerMessageItem(delivery, rootURL, []string{"/blacklisted"})
		Expect(item.IsBlacklisted()).To(BeTrue())
	})

	It("returns its URL", func() {
		Expect(item.URL()).To(Equal(testUrl))
	})

	Describe("generating a sane filename", func() {
		It("strips out the domain, protocol, auth and ports", func() {
			testUrl = "https://user:pass@example.com:8080/test/url"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("test/url.html"))
		})

		It("strips preceeding path traversals and resolves the remaining path", func() {
			testUrl = rootURL.String() + "/../../one/./two/../three"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("one/three.html"))
		})

		It("preserves case sensitivity", func() {
			testUrl = rootURL.String() + "/test/UPPER/MiXeD"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("test/UPPER/MiXeD.html"))
		})

		It("preserves non-alphanumeric characters", func() {
			testUrl = rootURL.String() + "/test/!T@e£s$t/U^R*L(){}"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("test/!T@e£s$t/U^R*L(){}.html"))
		})

		It("preserves multiple dashes", func() {
			testUrl = rootURL.String() + "/test/one-two--three---"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("test/one-two--three---.html"))
		})

		It("preserves non-latin chars and not URL encode them", func() {
			testUrl = rootURL.String() + `/test/如何在香港申請英國簽證`
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal(`test/如何在香港申請英國簽證.html`))
		})

		It("adds an index.html suffix when URL references a directory", func() {
			testUrl = rootURL.String() + "/this/url/has/a/trailing/slash/"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("this/url/has/a/trailing/slash/index.html"))
		})

		It("adds an index.html suffix when URL has no path and no trailing slash", func() {
			testUrl = rootURL.String() + "/"
			delivery = amqp.Delivery{Body: []byte(testUrl)}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("index.html"))
		})

		It("omits URL query parameters", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "?foo=bar")}
			item = NewCrawlerMessageItem(delivery, rootURL, []string{})

			item.Response = &CrawlerResponse{Body: html}
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("government/organisations.html"))
		})

		It("omits URL fragments", func() {
			delivery := amqp.Delivery{Body: []byte(testUrl + "#foo")}

			item = NewCrawlerMessageItem(delivery, rootURL, []string{})
			item.Response = &CrawlerResponse{Header: make(http.Header), Body: html}
			item.Response.Header.Set("Content-Type", HTML)

			Expect(item.RelativeFilePath()).To(Equal("government/organisations.html"))
		})
	})

	Describe("ExtractURLs", func() {
		It("should return an empty array if it can't find any matching URLs", func() {
			item.Response.Body = []byte("")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(Equal([]*url.URL{}))
		})

		It("should extract all a[@href] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<div><a href="https://www.gov.uk/"></a></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all img[@src] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<div><img src="https://www.gov.uk/image.png" /></div>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/image.png")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all link[@href] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<head><link rel="icon" href="https://www.gov.uk/favicon.ico"></head>`)
			expectedUrl, _ := url.Parse("https://www.gov.uk/favicon.ico")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract all script[@src] URLs from a given HTML document", func() {
			item.Response.Body = []byte(
				`<head><script type="text/javascript" src="https://www.gov.uk/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()
			expectedUrl, _ := url.Parse("https://www.gov.uk/jq.js")

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("successfully extracts multiple matching URLs from the provided DOM", func() {
			item.Response.Body = []byte(
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
			item.Response.Body = []byte(
				`<head><script type="text/javascript" src="https://www.foobar.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(BeEmpty())
		})

		It("will unescape URLs", func() {
			item.Response.Body = []byte(`<div><a href="http://www.gov.uk/bar%20"></a></div>`)
			expectedUrl, _ := url.Parse("http://www.gov.uk/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should extract relative URLs", func() {
			item.Response.Body = []byte(`<div><a href="/foo/bar">a</a><a href="mailto:c@d.com">b</a></div>`)
			expectedUrl, _ := url.Parse("https://www.gov.uk/foo/bar")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("should remove the #fragment when extracting URLs", func() {
			item.Response.Body = []byte(`<div><a href="http://www.gov.uk/#germany"></a></div>`)
			expectedUrl, _ := url.Parse("http://www.gov.uk/")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedUrl))
		})

		It("removes paths that are blacklisted", func() {
			item := NewCrawlerMessageItem(delivery, rootURL, []string{"/trade-tariff"})
			item.Response = &CrawlerResponse{
				Body: []byte(`<div><a href="/foo/bar">a</a><a href="/trade-tariff">b</a></div>`),
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
		})

		It("should only return unique URLs", func() {
			item.Response.Body = []byte(`<a href="http://www.gov.uk/foo">a</a><a href="http://www.gov.uk/foo">b</a>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(HaveLen(1))
		})
	})
})
