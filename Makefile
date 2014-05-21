.PHONY: deps build test

GOPATH := `pwd`/vendor:$(GOPATH)
BINARY := govuk_crawler_worker

all: deps build test

deps:
	GOPATH=$(GOPATH) go get -t -v ./...

build:
	GOPATH=$(GOPATH) go build -o $(BINARY)

test:
	GOPATH=$(GOPATH) go test -v ./...
