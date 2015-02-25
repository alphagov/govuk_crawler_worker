package main

import (
	"bytes"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	log "github.com/Sirupsen/logrus"
	"github.com/alphagov/govuk_crawler_worker/http_crawler"
	"github.com/streadway/amqp"
)

type CrawlerMessageItem struct {
	amqp.Delivery
	Response *http_crawler.CrawlerResponse

	rootURLs       []*url.URL
	blacklistPaths []string
}

func NewCrawlerMessageItem(delivery amqp.Delivery, rootURLs []*url.URL, blacklistPaths []string) *CrawlerMessageItem {
	return &CrawlerMessageItem{
		Delivery:       delivery,
		rootURLs:       rootURLs,
		blacklistPaths: blacklistPaths,
	}
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
	host := strings.SplitN(urlParts.Host, ":", 2)[0]

	contentType, err := c.Response.ParseContentType()
	if err != nil {
		return "", err
	}

	if contentType == http_crawler.HTML {
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

	filePath = path.Clean(filePath)
	filePath = filepath.Join(host, filePath)
	filePath = strings.TrimPrefix(filePath, "/")

	return filePath, nil
}

func (c *CrawlerMessageItem) ExtractURLs() ([]*url.URL, error) {
	extractedURLs := []*url.URL{}

	document, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.Response.Body))
	if err != nil {
		return extractedURLs, err
	}

	urlElementMatches := [][]string{
		{"a", "href"},
		{"img", "src"},
		{"link", "href"},
		{"script", "src"},
	}

	var hrefs []string
	var urls []*url.URL

	for _, attr := range urlElementMatches {
		element, attr := attr[0], attr[1]

		hrefs = findHrefsByElementAttribute(document, element, attr)
		urls, err = parseURLs(hrefs)

		if err != nil {
			return extractedURLs, err
		}

		baseURL := &url.URL{
			Scheme: c.Response.URL.Scheme,
			Host:   c.Response.URL.Host,
		}
		urls = convertURLsToAbsolute(baseURL, urls)
		urls = filterURLsByHost(c.rootURLs, urls)
		urls = filterBlacklistedURLs(c.blacklistPaths, urls)
		urls = removeFragmentFromURLs(urls)

		extractedURLs = append(extractedURLs, urls...)
	}

	extractedURLs = filterDuplicateURLs(extractedURLs)

	return extractedURLs, err
}

func (c *CrawlerMessageItem) IsBlacklisted() bool {
	urlParts, err := url.Parse(c.URL())
	if err != nil {
		log.Warningln("Malformed URL", c.URL())
		return false
	}
	return isBlacklistedPath(urlParts.Path, c.blacklistPaths)
}

func parseURLs(urls []string) ([]*url.URL, error) {
	var parsedURLs []*url.URL
	var err error

	for _, u := range urls {
		u, err := url.Parse(u)
		if err != nil {
			return parsedURLs, err
		}
		parsedURLs = append(parsedURLs, u)
	}

	return parsedURLs, err
}

func convertURLsToAbsolute(baseURL *url.URL, urls []*url.URL) []*url.URL {
	return mapURLs(urls, func(url *url.URL) *url.URL {
		return baseURL.ResolveReference(url)
	})
}

func removeFragmentFromURLs(urls []*url.URL) []*url.URL {
	return mapURLs(urls, func(url *url.URL) *url.URL {
		url.Fragment = ""
		return url
	})
}

func filterURLsByHost(allowedHosts []*url.URL, urls []*url.URL) (ret []*url.URL) {
	for _, allowed := range allowedHosts {
		ret = append(ret, filterURLs(urls, func(url *url.URL) bool {
			return url.Host == allowed.Host
		})...)
	}

	return ret
}

func filterBlacklistedURLs(blacklistedPaths []string, urls []*url.URL) []*url.URL {
	return filterURLs(urls, func(url *url.URL) bool {
		return !isBlacklistedPath(url.Path, blacklistedPaths)
	})
}

func filterDuplicateURLs(urls []*url.URL) []*url.URL {
	urlMap := make(map[string]*url.URL)
	for _, url := range urls {
		urlMap[url.String()] = url
	}

	uniqueUrls := make([]*url.URL, 0, len(urlMap))
	for _, url := range urlMap {
		uniqueUrls = append(uniqueUrls, url)
	}

	return uniqueUrls
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
