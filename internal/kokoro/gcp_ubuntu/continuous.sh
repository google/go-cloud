#!/bin/bash
# Uses -euxo pipefail as described at
# https://vaneyckt.io/posts/safer_bash_scripts_with_set_euxo_pipefail/
# Fails on any error, prints all commands run
set -euxo pipefail

echo $PATH
export GOPATH="$HOME/go"
GOPARENT="/usr/local"
export GOROOT="$GOPARENT/go"

# Download Go 1.10+, required for vgo
GOVERSION="1.10.2"
curl "https://dl.google.com/go/go$GOVERSION.linux-amd64.tar.gz" -o /tmp/go.tar.gz
sudo rm -rf $GOROOT
sudo tar -C $GOPARENT -xf /tmp/go.tar.gz

# cd to project dir on Kokoro instance
cd git/go-cloud/

which go
$GOROOT/bin/go version

# Replace golint as the removal of the old go installation deleted it
$GOROOT/bin/go get -u golang.org/x/lint/golint
$GOROOT/bin/go get -u golang.org/x/vgo
$GOPATH/bin/vgo version

GO_CLOUD_HOME="$GOPATH/src/github.com/google/go-cloud"
mkdir -p "$(dirname "$GO_CLOUD_HOME")"
cp -R . "$GO_CLOUD_HOME"
cd "$GO_CLOUD_HOME"

export CC=gcc
ret=0
$GOPATH/bin/vgo test -race -short ./...
$GOPATH/bin/golint -set_exit_status ./...

