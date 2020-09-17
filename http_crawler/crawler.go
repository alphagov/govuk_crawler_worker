package http_crawler

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

var (
	ErrCannotCrawlURL  = errors.New("Cannot crawl URLs that don't live under the provided root URLs")
	ErrNotFound        = errors.New("404 Not Found")
	ErrRetryRequest5XX = errors.New("Retry request: 5XX HTTP Response returned")
	ErrRetryRequest429 = errors.New("Retry request: 429 HTTP Response returned (back off)")

	redirectStatusCodes = []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect}

	statusCodes []int
	once        sync.Once
)

type BasicAuth struct {
	Username string
	Password string
}

type Crawler struct {
	RootURLs []*url.URL

	basicAuth      *BasicAuth
	version        string
	rateLimitToken string
}

func NewCrawler(rootURLs []*url.URL, versionNumber string, rateLimitToken string, basicAuth *BasicAuth) *Crawler {
	return &Crawler{
		RootURLs: rootURLs,

		basicAuth:      basicAuth,
		version:        versionNumber,
		rateLimitToken: rateLimitToken,
	}
}

func (c *Crawler) Crawl(crawlURL *url.URL) (*CrawlerResponse, error) {
	var redirectDestinationURL = ""
	var redirectBody = ""
	var body []byte

	if !IsAllowedHost(crawlURL.Host, c.RootURLs) {
		return nil, ErrCannotCrawlURL
	}

	req, err := http.NewRequest("GET", crawlURL.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.basicAuth != nil {
		req.SetBasicAuth(c.basicAuth.Username, c.basicAuth.Password)
	}

	if c.rateLimitToken != "" {
		req.Header.Set("Rate-Limit-Token", c.rateLimitToken)
	}

	hostname, _ := os.Hostname()

	req.Header.Set("User-Agent", fmt.Sprintf(
		"GOV.UK Crawler Worker/%s on host '%s'", c.version, hostname))

	resp, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == 429:
		return nil, ErrRetryRequest429
	case containsInt(Retry5XXStatusCodes(), resp.StatusCode):
		return nil, ErrRetryRequest5XX
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case containsInt(redirectStatusCodes, resp.StatusCode):
		// If we encounter a redirect, create some HTML that does the redirect
		// and return this as a body. This enables two things:
		//  1. A file with this HTML will be created which will allow
		//     redirects to work in a static environment.
		//  2. The link to the new URL will be picked up and added to the queue
		//     so it will be visited and the eventual content saved.
		redirectDestinationURL = resp.Header.Get("Location")
		redirectBody = `<!DOCTYPE html>
		<html lang="en">
		<head>
		<meta http-equiv="refresh" content="1; url=` + redirectDestinationURL + `">
		<title>Redirecting</title>
		</head>
		<body>
		<p>Redirecting you to <a href="` + redirectDestinationURL + `">` + redirectDestinationURL + `</a>.</p>
		</body>
		</html>`
	}

	if redirectBody != "" {
		body = []byte(redirectBody)
	} else {
		body, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	response := &CrawlerResponse{
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
		URL:         resp.Request.URL,
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

func containsInt(haystack []int, needle int) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}

	return false
}

func IsAllowedHost(needle string, allowedHosts []*url.URL) bool {
	needleHost, err := HostOnly(needle)
	if err != nil {
		return false
	}

	for _, url := range allowedHosts {
		h, _ := HostOnly(url.Host)

		if h == needleHost {
			return true
		}
	}

	return false
}

// HostOnly parses out the host and removes the port (and separating colon) if
// present.
func HostOnly(hostport string) (string, error) {
	host, _, err := net.SplitHostPort(hostport)

	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			return hostport, nil
		}

		return "", err
	}

	return host, nil
}
