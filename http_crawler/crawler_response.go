package http_crawler

import (
	"mime"
	"net/http"
)

const (
	ATOM = "application/atom+xml"
	CSV  = "text/csv"
	DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	ICS  = "text/calendar"
	JSON = "application/json"
	ODP  = "application/vnd.oasis.opendocument.presentation"
	ODS  = "application/vnd.oasis.opendocument.spreadsheet"
	ODT  = "application/vnd.oasis.opendocument.text"
	PDF  = "application/pdf"
	XLS  = "application/vnd.ms-excel"
	XLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
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
