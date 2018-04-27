#!/bin/bash
# Fail on any error
set -eo pipefail
# Display commands being run
set -x
# cd to project dir on Kokoro instance
cd git/go-cloud/

go version

export GOPATH="$HOME/go"
go get -u golang.org/x/vgo
"$GOPATH/bin/vgo" version

GO_CLOUD_HOME="$GOPATH/src/github.com/google/go-cloud"
mkdir -p "$(dirname "$GO_CLOUD_HOME")"
cp -R . "$GO_CLOUD_HOME"
cd "$GO_CLOUD_HOME"

CC=gcc "$GOPATH/bin/vgo" test -race -v -short ./...
"$GOPATH/bin/vgo" vet ./...
golint -set_exit_status ./...
