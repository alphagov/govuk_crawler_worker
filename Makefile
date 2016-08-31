.PHONY: test build

BINARY := govuk_crawler_worker
ORG_PATH := github.com/alphagov
REPO_PATH := $(ORG_PATH)/govuk_crawler_worker

all: test build

test:
	go test -v \
		$(REPO_PATH) \
		$(REPO_PATH)/http_crawler \
		$(REPO_PATH)/queue \
		$(REPO_PATH)/ttl_hash_set \
		$(REPO_PATH)/util \

build:
	go build -o $(BINARY)

clean:
	rm -rf bin $(BINARY)
