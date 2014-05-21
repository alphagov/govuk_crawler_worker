.PHONY: deps build test

BINARY := govuk_crawler_worker

all: deps test build

deps:
	go run third_party.go get -t -v .

test:
	go run third_party.go test -v \
		github.com/alphagov/govuk_crawler_worker \
		github.com/alphagov/govuk_crawler_worker/http_crawler \
		github.com/alphagov/govuk_crawler_worker/queue \
		github.com/alphagov/govuk_crawler_worker/ttl_hash_set \

build:
	go run third_party.go build -o $(BINARY)
