package main_test

import (
	"io/ioutil"
	"net/url"

	. "github.com/alphagov/govuk_crawler_worker"
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"

	"github.com/streadway/amqp"
)

var _ = Describe("CrawlerMessageItem", func() {
	var (
		mirrorRoot                string
		delivery                  amqp.Delivery
		err                       error
		html                      []byte
		item                      *CrawlerMessageItem
		rootURLs                  []*url.URL
		testURL, urlA, urlB, urlC *url.URL
	)

	BeforeEach(func() {
		mirrorRoot = os.Getenv("MIRROR_ROOT")
		if mirrorRoot == "" {
			mirrorRoot, err = ioutil.TempDir("", "crawler_message_item_test")
			Expect(err).To(BeNil())
		}

		testURL = &url.URL{
			Scheme: "https",
			Host:   "www.gov.uk",
			Path:   "/government/organisations",
		}

		delivery = amqp.Delivery{Body: []byte(testURL.String())}
		item = NewCrawlerMessageItem(delivery, rootURLs, []string{})

		urlA = &url.URL{
			Scheme: "https",
			Host:   "www.gov.uk",
			Path:   "/",
		}
		urlB = &url.URL{
			Scheme: "https",
			Host:   "assets.digital.cabinet-office.gov.uk",
			Path:   "/",
		}
		urlC = &url.URL{
			Scheme: "https",
			Host:   "example.com",
			Path:   "/",
		}
		rootURLs = []*url.URL{urlA, urlB, urlC}

		html = []byte(`<html>
<head><title>test</title</head>
<body><h1>TEST</h1></body>
</html>
`)
		item.Response = &CrawlerResponse{
			Body:        html,
			ContentType: HTML,
			URL:         urlA,
		}
	})

	AfterEach(func() {
		DeleteMirrorFilesFromDisk(mirrorRoot)
	})

	It("generates a CrawlerMessageItem object", func() {
		Expect(NewCrawlerMessageItem(delivery, rootURLs, []string{})).ToNot(BeNil())
	})

	It("can get the Response.Request.URL of the crawled URL", func() {
		Expect(item.Response.URL).To(Equal(urlA))
	})

	Describe("getting and setting the Response.Body", func() {
		It("can get the Response.Body of the crawled URL", func() {
			Expect(item.Response.Body).To(Equal(html))
		})

		It("can set the Response.Body of the crawled URL", func() {
			item := NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{Body: []byte("foo")}

			Expect(item.Response.Body).To(Equal([]byte("foo")))
		})
	})

	It("detects when a URL is blacklisted", func() {
		delivery = amqp.Delivery{Body: []byte("https://www.example.com/blacklisted")}
		item := NewCrawlerMessageItem(delivery, rootURLs, []string{"/blacklisted"})
		Expect(item.IsBlacklisted()).To(BeTrue())
	})

	It("returns its URL", func() {
		Expect(item.URL()).To(Equal(testURL.String()))
	})

	Describe("generating a sane filename", func() {
		It("strips out the domain, protocol, auth and ports", func() {
			testURL = &url.URL{
				Scheme: "https",
				User:   url.UserPassword("user", "pass"),
				Host:   "example.com:8080",
				Path:   "/test/url",
			}

			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("example.com/test/url.html"))
		})

		It("strips preceeding path traversals and resolves the remaining path", func() {
			testURL.Path = "/../../one/./two/../three"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/one/three.html"))
		})

		It("preserves case sensitivity", func() {
			testURL.Path = "/test/UPPER/MiXeD"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/test/UPPER/MiXeD.html"))
		})

		It("preserves non-alphanumeric characters", func() {
			testURL.Path = "/test/!T@e£s$t/U^R*L(){}"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/test/!T@e£s$t/U^R*L(){}.html"))
		})

		It("preserves multiple dashes", func() {
			testURL.Path = "/test/one-two--three---"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/test/one-two--three---.html"))
		})

		It("preserves non-latin chars and not URL encode them", func() {
			testURL.Path = "/test/如何在香港申請英國簽證"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/test/如何在香港申請英國簽證.html"))
		})

		It("adds an index.html suffix when URL references a directory", func() {
			testURL.Path = "/this/url/has/a/trailing/slash/"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/this/url/has/a/trailing/slash/index.html"))
		})

		It("adds an index.html suffix when URL has no path and no trailing slash", func() {
			testURL.Path = "/"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/index.html"))
		})

		It("omits URL query parameters", func() {
			delivery := amqp.Delivery{Body: []byte(testURL.String() + "?foo=bar")}
			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})

			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/government/organisations.html"))
		})

		It("omits URL fragments", func() {
			delivery := amqp.Delivery{Body: []byte(testURL.String() + "#foo")}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{
				Body:        []byte("foo"),
				ContentType: HTML,
			}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/government/organisations.html"))
		})

		It("supports ATOM URLs", func() {
			testURL.Path = "/things.atom"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{Body: []byte(""), ContentType: ATOM}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/things.atom"))
		})

		It("supports JSON URLs", func() {
			testURL.Path = "/api.json"
			delivery = amqp.Delivery{Body: []byte(testURL.String())}

			item = NewCrawlerMessageItem(delivery, rootURLs, []string{})
			item.Response = &CrawlerResponse{Body: []byte(""), ContentType: JSON}

			Expect(item.RelativeFilePath()).To(Equal("www.gov.uk/api.json"))
		})
	})

	Describe("ExtractURLs", func() {
		It("should extract all URLs that match rootURLs in a given HTML document", func() {
			item.Response.Body = []byte(`
                          <img src="https://assets.digital.cabinet-office.gov.uk/image.png">
                          <a href="https://www.gov.uk/foo">Test</a>
                          <script src="https://example.com/jq.js"></script>
                          <script src="https://google.com/another.js"></script>
                        `)

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(HaveLen(3))
		})

		It("should return an empty array if it can't find any matching URLs", func() {
			item.Response.Body = []byte("")
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(Equal([]*url.URL{}))
		})

		It("should extract all a[@href] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<div><a href="https://www.gov.uk/"></a></div>`)
			urls, err := item.ExtractURLs()
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/",
			}

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("should extract all img[@src] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<div><img src="https://www.gov.uk/image.png" /></div>`)
			urls, err := item.ExtractURLs()

			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/image.png",
			}

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("should extract all link[@href] URLs from a given HTML document", func() {
			item.Response.Body = []byte(`<head><link rel="icon" href="https://www.gov.uk/favicon.ico"></head>`)
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/favicon.ico",
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("should extract all script[@src] URLs from a given HTML document", func() {
			item.Response.Body = []byte(
				`<head><script type="text/javascript" src="https://www.gov.uk/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/jq.js",
			}

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("successfully extracts multiple matching URLs from the provided DOM", func() {
			item.Response.Body = []byte(
				`<head>
<script type="text/javascript" src="https://www.gov.uk/jq.js"></script>
<link rel="icon" href="https://www.gov.uk/favicon.ico">
</head>`)
			urls, err := item.ExtractURLs()
			expectedURL1 := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/jq.js",
			}
			expectedURL2 := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/favicon.ico",
			}

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL1))
			Expect(urls).To(ContainElement(expectedURL2))
		})

		It("will not provide URLs that don't match the provided prefix rootURL", func() {
			item.Response.Body = []byte(
				`<head><script type="text/javascript" src="https://www.foobar.com/jq.js"></script></head>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(BeEmpty())
		})

		It("will unescape URLs", func() {
			item.Response.Body = []byte(`<div><a href="https://www.gov.uk/bar%20"></a></div>`)
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/bar",
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("should extract relative URLs", func() {
			item.Response.Body = []byte(`<div><a href="/foo/bar">a</a><a href="mailto:c@d.com">b</a></div>`)
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/foo/bar",
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("should remove the #fragment when extracting URLs", func() {
			item.Response.Body = []byte(`<div><a href="https://www.gov.uk/#germany"></a></div>`)
			expectedURL := &url.URL{
				Scheme: "https",
				Host:   "www.gov.uk",
				Path:   "/",
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(ContainElement(expectedURL))
		})

		It("removes paths that are blacklisted", func() {
			item := NewCrawlerMessageItem(delivery, rootURLs, []string{"/trade-tariff"})
			item.Response = &CrawlerResponse{
				Body:        []byte(`<div><a href="/foo/bar">a</a><a href="/trade-tariff">b</a></div>`),
				ContentType: HTML,
				URL:         urlA,
			}

			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(1))
		})

		It("should only return unique URLs", func() {
			item.Response.Body = []byte(`<a href="https://www.gov.uk/foo">a</a><a href="https://www.gov.uk/foo">b</a>`)
			urls, err := item.ExtractURLs()

			Expect(err).To(BeNil())
			Expect(urls).To(HaveLen(1))
		})
	})

	Describe("ParseURLs", func() {
		It("should return all valid URLs regardless of order", func() {
			var urls []*url.URL

			hrefA := "https://www.gov.uk/tax-disc"
			hrefB := "https://bad hostname.com/"
			hrefC := "https://example.com/"
			hrefs := []string{hrefA, hrefB, hrefC}

			urls, err = parseURLs(hrefs)

			Expect(err).To(BeNil())
			Expect(len(urls)).To(Equal(2))
		})
	})
})
