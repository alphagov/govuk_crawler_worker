#!/bin/sh

set -xeu

go get -t -v ./...
go test -v ./...
go build
