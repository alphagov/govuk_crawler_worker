package http_crawler_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestHTTPCrawler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Crawler Suite")
}
