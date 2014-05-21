.PHONY: deps build test

GOPATH := `pwd`/vendor:$(GOPATH)
BINARY := govuk_crawler_worker

all: deps build test

deps:
	git submodule update --init

build:
	GOPATH=$(GOPATH) go build -o $(BINARY)

test:
	GOPATH=$(GOPATH) go test -v ./ttl_hash_set ./http_crawler ./queue .
