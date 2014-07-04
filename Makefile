.PHONY: deps test build rm_compiled_self

BINARY := govuk_crawler_worker
ORG_PATH := github.com/alphagov
REPO_PATH := $(ORG_PATH)/govuk_crawler_worker

all: deps test build

deps: third_party/src/$(REPO_PATH) rm_compiled_self
	go run third_party.go get -t -v .

rm_compiled_self:
	rm -rf third_party/pkg/*/$(REPO_PATH)

third_party/src/$(REPO_PATH):
	mkdir -p third_party/src/$(ORG_PATH)
	ln -s ../../../.. third_party/src/$(REPO_PATH)

test:
	go run third_party.go test -v \
		$(REPO_PATH) \
		$(REPO_PATH)/http_crawler \
		$(REPO_PATH)/queue \
		$(REPO_PATH)/ttl_hash_set \
		$(REPO_PATH)/util \

build:
	go run third_party.go build -o $(BINARY)
