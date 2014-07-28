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

		// Remove the fragment from the URLs.
		for _, u := range urls {
			u.Fragment = ""
		}

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
	var absoluteUrls []*url.URL

	for _, u := range urls {
		absUrl := rootURL.ResolveReference(u)
		absoluteUrls = append(absoluteUrls, absUrl)
	}

	return absoluteUrls
}

func filterUrlsByHost(host string, urls []*url.URL) []*url.URL {
	var filteredUrls []*url.URL

	for _, u := range urls {
		if u.Host == host {
			filteredUrls = append(filteredUrls, u)
		}
	}

	return filteredUrls
}

func filterBlacklistedUrls(blacklistedPaths []string, urls []*url.URL) []*url.URL {
	var filteredUrls []*url.URL

	for _, u := range urls {
		if !isBlacklistedPath(u.Path, blacklistedPaths) {
			filteredUrls = append(filteredUrls, u)
		}
	}

	return filteredUrls
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
