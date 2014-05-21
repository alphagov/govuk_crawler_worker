.PHONY: build test

GOPATH := `pwd`/vendor:$(GOPATH)
BINARY := govuk_crawler_worker

all: build test

build:
	GOPATH=$(GOPATH) go build -o $(BINARY)

test:
	GOPATH=$(GOPATH) go test -v ./ttl_hash_set ./http_crawler ./queue .
