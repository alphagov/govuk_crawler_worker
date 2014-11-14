.PHONY: deps vendor test build

BINARY := govuk_crawler_worker
ORG_PATH := github.com/alphagov
REPO_PATH := $(ORG_PATH)/govuk_crawler_worker

all: test build

deps:
	gom install

vendor: deps
	rm -rf _vendor/src/$(ORG_PATH)
	mkdir -p _vendor/src/$(ORG_PATH)
	ln -s $(CURDIR) _vendor/src/$(REPO_PATH)

test: vendor
	gom test -v \
		$(REPO_PATH) \
		$(REPO_PATH)/http_crawler \
		$(REPO_PATH)/queue \
		$(REPO_PATH)/ttl_hash_set \
		$(REPO_PATH)/util \

build: vendor
	gom build -o $(BINARY)

clean:
	rm -rf bin _vendor $(BINARY)
