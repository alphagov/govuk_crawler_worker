# GOV.UK Crawler Worker

[![continuous integration status](https://travis-ci.org/alphagov/govuk_crawler_worker.svg?branch=master)](http://travis-ci.org/alphagov/govuk_crawler_worker)

This is a worker that will consume [GOV.UK](https://www.gov.uk/) URLs
from a message queue and crawl them, saving the output to disk.

## Requirements

To run this worker you will need:

 - Go 1.4
 - [RabbitMQ](https://www.rabbitmq.com/)
 - [Redis](http://redis.io/)

## Development

You can run the tests locally by running the following: `make`. This
will use the `gom` tool to vendorise dependencies into a folder within
the project.

You can download the `gom` tool by running:
`go get github.com/mattn/gom`.

## Running

To run the worker you'll first need to build it using `go build` to
generate a binary. You can then run the built binary directly using
`./govuk_crawler_worker`. All configuration is injected using
environment varibles. For details on this look at the `main.go` file.

## How it works

This is a message queue worker that will consume URLs from a queue and
crawl them, saving the output to disk. Whilst this is the main reason
for this worker to exist it has a few activities that it covers before
the page gets written to disk.

### Workflow

The workflow for the worker can be defined as the following set of
steps:

1. Read a URL from the queue, e.g. https://www.gov.uk/bank-holidays
2. Crawl the recieved URL
3. Write the body of the crawled URL to disk
4. Extract any matching URLs from the HTML body of the crawled URL
5. Publish the extracted URLs to the worker's own exchange
6. Acknowledge that the URL has been crawled

### The Interface

The public interface for the worker is the exchange labelled
**govuk_crawler_exchange**. When the worker starts it creates this
exchange and binds it to it's own queue for consumption.

If you provide user credentials for RabbitMQ that aren't on the root
vhost `/`, you may wish to bind a global exchange yourself for easier
publishing by other applications.
