.PHONY: deps test build

BINARY := govuk_crawler_worker
REPO_PATH := github.com/alphagov/govuk_crawler_worker

all: deps test build

deps:
	go run third_party.go get -t -v .

test:
	go run third_party.go test -v \
		$(REPO_PATH) \
		$(REPO_PATH)/http_crawler \
		$(REPO_PATH)/queue \
		$(REPO_PATH)/ttl_hash_set \

build:
	go run third_party.go build -o $(BINARY)
