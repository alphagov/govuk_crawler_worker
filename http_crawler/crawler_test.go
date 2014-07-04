package http_crawler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testServer(status int, body string) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprintln(w, body)
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}

var _ = Describe("Crawl", func() {
	var crawler *Crawler

	BeforeEach(func() {
		rootURL, _ := url.Parse("http://127.0.0.1")
		crawler = NewCrawler(rootURL)

		Expect(crawler).ToNot(BeNil())
	})

	Describe("RetryStatusCodes", func() {
		It("should return a fixed int array with values 429, 500..599", func() {
			statusCodes := RetryStatusCodes()

			Expect(len(statusCodes)).To(Equal(101))
			Expect(statusCodes[0]).To(Equal(429))
			Expect(statusCodes[1]).To(Equal(500))
			Expect(statusCodes[100]).To(Equal(599))
		})
	})

	Describe("NewCrawler()", func() {
		It("provides a new crawler that accepts the provided host", func() {
			rootURL, _ := url.Parse("https://www.gov.uk/")
			GOVUKCrawler := NewCrawler(rootURL)

			Expect(GOVUKCrawler.RootURL.Host).To(Equal("www.gov.uk"))
		})
	})

	Describe("Crawler.Crawl()", func() {
		It("specifies a user agent when making a request", func() {
			userAgentTestServer := func(httpStatus int) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(httpStatus)
					fmt.Fprintln(w, r.UserAgent())
				}))
			}

			ts := userAgentTestServer(http.StatusOK)
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			body, err := crawler.Crawl(testURL)

			Expect(err).To(BeNil())
			Expect(string(body)).Should(MatchRegexp("GOV.UK Crawler Worker"))
		})

		It("returns a body with no errors for 200 OK responses", func() {
			ts := testServer(http.StatusOK, "Hello world")
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			body, err := crawler.Crawl(testURL)

			Expect(err).To(BeNil())
			Expect(strings.TrimSpace(string(body))).To(Equal("Hello world"))
		})

		It("doesn't allow crawling a URL that doesn't match the root URL", func() {
			testURL, _ := url.Parse("http://www.google.com/foo")
			body, err := crawler.Crawl(testURL)

			Expect(err).To(Equal(CannotCrawlURL))
			Expect(body).To(Equal([]byte{}))
		})

		Describe("returning a retry error", func() {
			It("returns a retry error if we get a response code of Too Many Requests", func() {
				ts := testServer(429, "Too Many Requests")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				body, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(RetryRequestError))
				Expect(body).To(Equal([]byte{}))
			})

			It("returns a retry error if we get a response code of Internal Server Error", func() {
				ts := testServer(http.StatusInternalServerError, "Internal Server Error")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				body, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(RetryRequestError))
				Expect(body).To(Equal([]byte{}))
			})

			It("returns a retry error if we get a response code of Gateway Timeout", func() {
				ts := testServer(http.StatusGatewayTimeout, "Gateway Timeout")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				body, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(RetryRequestError))
				Expect(body).To(Equal([]byte{}))
			})
		})
	})
})
