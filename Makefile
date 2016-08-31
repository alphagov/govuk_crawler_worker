.PHONY: test build

BINDIR ?= .
BINARY := govuk_crawler_worker

all: test build

test:
	go test -v $$(go list ./... | grep -v '/vendor')

build:
	go build -o $(BINDIR)/$(BINARY)

clean:
	rm -rf $(BINDIR)/$(BINARY)
