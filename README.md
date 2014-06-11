# GOV.UK Crawler Worker

[![continuous integration status](https://secure.travis-ci.org/alphagov/govuk_crawler_worker.png)](http://travis-ci.org/alphagov/govuk_crawler_worker)

This is a worker that will consume [GOV.UK](https://www.gov.uk/) URLs
from a message queue and crawl them, saving the output to disk.

## Requirements

To run this worker you will need:

 - Go 1.2
 - [RabbitMQ](https://www.rabbitmq.com/)
 - [Redis](http://redis.io/)

## Development

You can run the tests locally by running the following:

```
go get -v -t ./...
go test -v ./...
```

Alternatively to localise the dependencies you can use `make`. This
will use the `third_party.go` tool to vendorise dependencies into a
folder within the project.

## Running

To run the worker you'll first need to build it using `go build` to
generate a binary. You can then run the built binary directly using
`./govuk_crawler_worker`. All configuration is injected using
environment varibles. For details on this look at the `main.go` file.
