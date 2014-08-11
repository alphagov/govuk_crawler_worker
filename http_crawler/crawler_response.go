package http_crawler

import (
	"mime"
	"net/http"
)

const (
	JSON = "application/json"
)

type CrawlerResponse struct {
	Body   []byte
	Header http.Header
}

func (c *CrawlerResponse) ContentType() (string, error) {
	mimeType, _, err := mime.ParseMediaType(c.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}

	return mimeType, nil
}

func (c *CrawlerResponse) IsBodyHTML() bool {
	return http.DetectContentType(c.Body) == "text/html; charset=utf-8"
}
