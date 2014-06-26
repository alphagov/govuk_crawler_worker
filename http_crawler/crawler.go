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
	CannotCrawlURL    error = errors.New("Cannot crawl URLs that don't live under the provided root URL")
	RetryRequestError error = errors.New("Retry request: 429 or 5XX HTTP Response returned")

	statusCodes []int
	once        sync.Once
)

type Crawler struct {
	RootURL *url.URL
}

func RetryStatusCodes() []int {
	// This is go's equivalent of memoization/macro expansion. It's
	// being used here because we have a fixed array we're generating
	// with known values.
	once.Do(func() {
		statusCodes = []int{429}

		for i := 500; i <= 599; i++ {
			statusCodes = append(statusCodes, i)
		}
	})

	return statusCodes
}

func NewCrawler(rootURL string) (*Crawler, error) {
	if rootURL == "" {
		return nil, errors.New("Cannot provide an empty root URL")
	}

	u, err := url.Parse(rootURL)
	if err != nil {
		return nil, err
	}

	return &Crawler{
		RootURL: u,
	}, nil
}

func (c *Crawler) Crawl(crawlURL string) ([]byte, error) {
	u, err := url.Parse(crawlURL)
	if err != nil {
		return []byte{}, err
	}

	if !strings.HasPrefix(u.Host, c.RootURL.Host) {
		return []byte{}, CannotCrawlURL
	}

	req, err := http.NewRequest("GET", crawlURL, nil)
	if err != nil {
		return []byte{}, err
	}

	hostname, _ := os.Hostname()

	req.Header.Set("User-Agent", fmt.Sprintf("GOV.UK Crawler Worker on host '%s'", hostname))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []byte{}, err
	}

	if contains(RetryStatusCodes(), resp.StatusCode) {
		return []byte{}, RetryRequestError
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return body, nil
}

func contains(haystack []int, needle int) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}

	return false
}
