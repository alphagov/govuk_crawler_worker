.PHONY: deps build test

GOPATH := `pwd`/vendor:$(GOPATH)
BINARY := govuk_crawler_worker

deps:
	git submodule update --init

build:
	GOPATH=$(GOPATH) go build -o $(BINARY)

test:
	GOPATH=$(GOPATH) go test -v ./...
