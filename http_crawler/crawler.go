package http_crawler

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

var (
	CannotCrawlURL       = errors.New("Cannot crawl URLs that don't live under the provided root URL")
	NotFoundError        = errors.New("404 Not Found")
	RedirectError        = errors.New("HTTP redirect encountered")
	RetryRequest5XXError = errors.New("Retry request: 5XX HTTP Response returned")
	RetryRequest429Error = errors.New("Retry request: 429 HTTP Response returned (back off)")

	redirectStatusCodes = []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect}

	statusCodes []int
	once        sync.Once
)

type BasicAuth struct {
	Username string
	Password string
}

type Crawler struct {
	RootURL *url.URL

	basicAuth *BasicAuth
	version   string
}

func NewCrawler(rootURL *url.URL, versionNumber string, basicAuth *BasicAuth) *Crawler {
	return &Crawler{
		RootURL: rootURL,

		basicAuth: basicAuth,
		version:   versionNumber,
	}
}

func (c *Crawler) Crawl(crawlURL *url.URL) (*CrawlerResponse, error) {
	if !strings.HasPrefix(crawlURL.Host, c.RootURL.Host) {
		return nil, CannotCrawlURL
	}

	req, err := http.NewRequest("GET", crawlURL.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.basicAuth != nil {
		req.SetBasicAuth(c.basicAuth.Username, c.basicAuth.Password)
	}

	hostname, _ := os.Hostname()

	req.Header.Set("User-Agent", fmt.Sprintf(
		"GOV.UK Crawler Worker/%s on host '%s'", c.version, hostname))

	resp, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == 429:
			return nil, RetryRequest429Error
		case contains(Retry5XXStatusCodes(), resp.StatusCode):
			return nil, RetryRequest5XXError
		case resp.StatusCode == http.StatusNotFound:
			return nil, NotFoundError
		case contains(redirectStatusCodes, resp.StatusCode):
			return nil, RedirectError
		}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := &CrawlerResponse{
		Body:   body,
		Header: resp.Header,
	}

	return response, nil
}

func Retry5XXStatusCodes() []int {
	// This is go's equivalent of memoization/macro expansion. It's
	// being used here because we have a fixed array we're generating
	// with known values.
	once.Do(func() {
		statusCodes = []int{}

		for i := 500; i <= 599; i++ {
			statusCodes = append(statusCodes, i)
		}
	})

	return statusCodes
}

func contains(haystack []int, needle int) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}

	return false
}
