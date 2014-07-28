package main

import (
	"bytes"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/kennygrant/sanitize"
	"github.com/streadway/amqp"
)

type CrawlerMessageItem struct {
	amqp.Delivery
	HTMLBody []byte

	rootURL        *url.URL
	blacklistPaths []string
}

func NewCrawlerMessageItem(delivery amqp.Delivery, rootURL *url.URL, blacklistPaths []string) *CrawlerMessageItem {
	return &CrawlerMessageItem{
		Delivery:       delivery,
		rootURL:        rootURL,
		blacklistPaths: blacklistPaths,
	}
}

func (c *CrawlerMessageItem) IsHTML() bool {
	return http.DetectContentType(c.HTMLBody) == "text/html; charset=utf-8"
}

func (c *CrawlerMessageItem) URL() string {
	return string(c.Body)
}

func (c *CrawlerMessageItem) RelativeFilePath() (string, error) {
	var filePath string

	urlParts, err := url.Parse(c.URL())
	if err != nil {
		return "", err
	}

	filePath = urlParts.Path

	if c.IsHTML() {
		r, err := regexp.Compile(`.(html|htm)$`)

		if err != nil {
			return "", err
		}

		switch {
		case strings.HasSuffix(filePath, "/"):
			filePath += "index.html"
		case !r.MatchString(filePath): // extension not .html or .htm
			filePath += ".html"
		}
	}

	filePath = sanitize.Path(filePath)
	filePath = strings.TrimPrefix(filePath, "/")

	return filePath, nil
}

func (c *CrawlerMessageItem) ExtractURLs() ([]*url.URL, error) {
	extractedUrls := []*url.URL{}

	document, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.HTMLBody))
	if err != nil {
		return extractedUrls, err
	}

	urlElementMatches := [][]string{
		[]string{"a", "href"},
		[]string{"img", "src"},
		[]string{"link", "href"},
		[]string{"script", "src"},
	}

	var hrefs []string
	var urls []*url.URL

	for _, attr := range urlElementMatches {
		element, attr := attr[0], attr[1]

		hrefs = findHrefsByElementAttribute(document, element, attr)
		urls, err = parseUrls(hrefs)

		if err != nil {
			return extractedUrls, err
		}

		urls = convertUrlsToAbsolute(c.rootURL, urls)
		urls = filterUrlsByHost(c.rootURL.Host, urls)
		urls = filterBlacklistedUrls(c.blacklistPaths, urls)
		urls = removeFragmentFromUrls(urls)

		extractedUrls = append(extractedUrls, urls...)
	}

	return extractedUrls, err
}

func parseUrls(urls []string) ([]*url.URL, error) {
	var parsedUrls []*url.URL
	var err error

	for _, u := range urls {
		u, err := url.Parse(u)
		if err != nil {
			return parsedUrls, err
		}
		parsedUrls = append(parsedUrls, u)
	}

	return parsedUrls, err
}

func convertUrlsToAbsolute(rootURL *url.URL, urls []*url.URL) []*url.URL {
	return mapURLs(urls, func(url *url.URL) *url.URL {
		return rootURL.ResolveReference(url)
	})
}

func removeFragmentFromUrls(urls []*url.URL) []*url.URL {
	return mapURLs(urls, func(url *url.URL) *url.URL {
		url.Fragment = ""
		return url
	})
}

func filterUrlsByHost(host string, urls []*url.URL) []*url.URL {
	return filterURLs(urls, func(url *url.URL) bool {
		return url.Host == host
	})
}

func filterBlacklistedUrls(blacklistedPaths []string, urls []*url.URL) []*url.URL {
	return filterURLs(urls, func(url *url.URL) bool {
		return !isBlacklistedPath(url.Path, blacklistedPaths)
	})
}

// Filter an array of *url.URL objects based on a filter function that
// returns a boolean. Only elements that return true for this filter
// function will be kept. Returns a new array.
func filterURLs(urls []*url.URL, filterFunc func(u *url.URL) bool) []*url.URL {
	var filteredURLs []*url.URL

	for _, url := range urls {
		if filterFunc(url) {
			filteredURLs = append(filteredURLs, url)
		}
	}

	return filteredURLs
}

// Map a function to each element of a *url.URL array. Returns a new
// array but will edit any url.URL objects in place should the mapFunc
// mutate state.
func mapURLs(urls []*url.URL, mapFunc func(u *url.URL) *url.URL) []*url.URL {
	for index, url := range urls {
		urls[index] = mapFunc(url)
	}

	return urls
}

func findHrefsByElementAttribute(
	document *goquery.Document,
	element string,
	attr string) []string {

	hrefs := []string{}

	document.Find(element).Each(func(_ int, element *goquery.Selection) {
		href, _ := element.Attr(attr)
		unescapedHref, _ := url.QueryUnescape(href)
		trimmedHref := strings.TrimSpace(unescapedHref)
		hrefs = append(hrefs, trimmedHref)
	})

	return hrefs
}

func isBlacklistedPath(path string, blacklistedPaths []string) bool {
	for _, blacklistedPath := range blacklistedPaths {
		if strings.HasPrefix(path, blacklistedPath) {
			return true
		}
	}

	return false
}
