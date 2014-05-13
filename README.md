# GOV.UK Crawler Worker

[![continuous integration status](https://secure.travis-ci.org/alphagov/govuk_crawler_worker.png)](http://travis-ci.org/alphagov/govuk_crawler_worker)

This is a worker that will consume [GOV.UK](https://www.gov.uk/) URLs
from a message queue and crawl them, saving the output to disk.

## Requirements

To run this worker you will need:

 - [RabbitMQ](https://www.rabbitmq.com/)
 - [Redis](http://redis.io/)

## Development

You can run the tests by running the following:

```
go get -v -t ./...
go test -v ./...
```
