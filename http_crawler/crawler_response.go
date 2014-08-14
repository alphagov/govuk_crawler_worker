package http_crawler

import (
	"mime"
)

const (
	ATOM = "application/atom+xml"
	CSV  = "text/csv"
	DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	HTML = "text/html"
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
	Body        []byte
	ContentType string
}

func (c *CrawlerResponse) AcceptedContentType() bool {
	mimeType, err := c.ParseContentType()
	if err != nil {
		return false
	}

	switch mimeType {
	case ATOM, CSV, DOCX, HTML, ICS, JSON, ODP, ODS, ODT, PDF, XLS, XLSX:
		return true
	}

	return false
}

func (c *CrawlerResponse) ParseContentType() (string, error) {
	mimeType, _, err := mime.ParseMediaType(c.ContentType)
	if err != nil {
		return "", err
	}

	return mimeType, nil
}
