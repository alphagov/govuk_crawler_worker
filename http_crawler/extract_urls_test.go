package http_crawler_test

import (
	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"bytes"
)

var _ = Describe("ExtractURLs", func() {
	It("should return an empty array if it can't find any matching URLs", func() {
		buffer := bytes.NewBufferString("")
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(Equal([]string{}))
	})

	It("should extract all a[@href] URLs from a given HTML document", func() {
		buffer := bytes.NewBufferString(`<div><a href="https://www.foo.com/"></a></div>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("https://www.foo.com/"))
	})

	It("should extract all img[@src] URLs from a given HTML document", func() {
		buffer := bytes.NewBufferString(`<div><img src="https://www.foo.com/image.png" /></div>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("https://www.foo.com/image.png"))
	})

	It("should extract all link[@href] URLs from a given HTML document", func() {
		buffer := bytes.NewBufferString(`<head><link rel="icon" href="https://www.foo.com/favicon.ico"></head>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("https://www.foo.com/favicon.ico"))
	})

	It("should extract all script[@src] URLs from a given HTML document", func() {
		buffer := bytes.NewBufferString(
			`<head><script type="text/javascript" src="https://www.foo.com/jq.js"></script></head>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("https://www.foo.com/jq.js"))
	})

	It("successfully extracts multiple matching URLs from the provided DOM", func() {
		buffer := bytes.NewBufferString(
			`<head>
<script type="text/javascript" src="https://www.foo.com/jq.js"></script>
<link rel="icon" href="https://www.foo.com/favicon.ico">
</head>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("https://www.foo.com/jq.js"))
		Expect(urls).To(ContainElement("https://www.foo.com/favicon.ico"))
	})

	It("will not provide URLs that don't match the provided prefix host", func() {
		buffer := bytes.NewBufferString(
			`<head><script type="text/javascript" src="https://www.foo.com/jq.js"></script></head>`)
		urls, err := ExtractURLs(buffer, "www.foobar.com")

		Expect(err).To(BeNil())
		Expect(urls).To(BeEmpty())
	})

	It("will unescape URLs", func() {
		buffer := bytes.NewBufferString(`<div><a href="http://www.foo.com/bar%20"></a></div>`)
		urls, err := ExtractURLs(buffer, "www.foo.com")

		Expect(err).To(BeNil())
		Expect(urls).To(ContainElement("http://www.foo.com/bar"))
	})
})
