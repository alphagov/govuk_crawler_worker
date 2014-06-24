package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/kennygrant/sanitize"
	"github.com/streadway/amqp"
)

type CrawlerMessageItem struct {
	amqp.Delivery
	HTMLBody []byte

	host           string
	blacklistPaths []string
}

func NewCrawlerMessageItem(delivery amqp.Delivery, host string, blacklistPaths []string) *CrawlerMessageItem {
	return &CrawlerMessageItem{
		Delivery:       delivery,
		host:           host,
		blacklistPaths: blacklistPaths,
	}
}

func (c *CrawlerMessageItem) IsHTML() bool {
	return http.DetectContentType(c.HTMLBody) == "text/html; charset=utf-8"
}

func (c *CrawlerMessageItem) URL() string {
	return string(c.Body)
}

func (c *CrawlerMessageItem) FilePath() (string, error) {
	var filePath string

	if mirrorRoot == "" {
		return "", errors.New("mirrorRoot not defined")
	}

	if strings.HasSuffix(mirrorRoot, "/") == false {
		filePath = "/"
	}

	urlParts, err := url.Parse(c.URL())
	if err != nil {
		return "", err
	}

	filePath += urlParts.Path

	if c.IsHTML() {
		r, err := regexp.Compile(`.(html|htm)$`)

		if err != nil {
			return "", err
		}

		switch {
		case strings.HasSuffix(filePath, "/"):
			filePath += "index.html"
		case r.MatchString(filePath) == false: // extension not .html or .htm
			filePath += ".html"
		}
	}

	filePath = sanitize.Path(filePath)
	filePath = mirrorRoot + filePath

	return filePath, nil
}

func (c *CrawlerMessageItem) WriteToDisk() (string, error) {
	filePath, err := c.FilePath()
	if err != nil {
		return "", err
	}

	basePath := filepath.Dir(filePath)
	err = os.MkdirAll(basePath, 0755)

	if err != nil {
		return filePath, err
	}

	err = ioutil.WriteFile(filePath, c.HTMLBody, 0644)

	if err != nil {
		return filePath, err
	}

	return filePath, nil
}

func (c *CrawlerMessageItem) ExtractURLs() ([]string, error) {
	urls := []string{}

	document, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.HTMLBody))
	if err != nil {
		return urls, err
	}

	urlElementMatches := [][]string{
		[]string{"a", "href"},
		[]string{"img", "src"},
		[]string{"link", "href"},
		[]string{"script", "src"},
	}

	for _, attr := range urlElementMatches {
		element, attr := attr[0], attr[1]
		urls = append(urls, findByElementAttribute(document, c.host, c.blacklistPaths, element, attr)...)
	}

	return urls, err
}

func findByElementAttribute(
	document *goquery.Document,
	host string,
	blacklistPaths []string,
	element string,
	attr string) []string {

	urls := []string{}

	document.Find(element).Each(func(_ int, element *goquery.Selection) {
		href, exists := element.Attr(attr)
		unescapedHref, _ := url.QueryUnescape(href)
		trimmedHref := strings.TrimSpace(unescapedHref)

		u, err := url.Parse(trimmedHref)
		if err != nil {
			log.Fatal(err)
		}

		if exists && len(trimmedHref) > 0 && !isBlacklistedPath(trimmedHref, blacklistPaths) {
			if u.Host == host {
				urls = append(urls, trimmedHref)
			}

			if u.Host == "" && trimmedHref[0] == '/' {
				urls = append(urls, "http://"+host+trimmedHref)
			}
		}
	})

	return urls
}

func isBlacklistedPath(url string, blacklistedPaths []string) bool {
	for _, path := range blacklistedPaths {
		if strings.Contains(url, path) {
			return true
		}
	}

	return false
}
