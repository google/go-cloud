#!/bin/bash
# Fail on any error
set -eo pipefail
# Display commands being run
set -x
# cd to project dir on Kokoro instance
cd git/go-gcp/

go version

export GOPATH="$HOME/go"
go get -u golang.org/x/vgo
"$GOPATH/bin/vgo" version

CODENAME_HOME="$GOPATH/src/codename"
mkdir -p "$(dirname "$CODENAME_HOME")"
cp -R . "$CODENAME_HOME"
cd "$CODENAME_HOME"

CC=gcc "$GOPATH/bin/vgo" test -race -v -short ./...
"$GOPATH/bin/vgo" vet ./...
golint -set_exit_status ./...
