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
$GOPATH/bin/vgo test -race -v -short ./... || ret=$?
# vgo might be sad if vgo is bugged (such is the way of beta), so fallback
# to standard go and try again
echo "vgo exited with $ret"
if [[ "$ret" != 0 ]]; then
  # Don't use -u due to https://github.com/golang/go/issues/13084
  go get -t ./...
  go test -race -v -short ./...
fi

$GOPATH/bin/golint -set_exit_status ./...

# Capture the grep failure into a variable as || will stop bash immediately
# dying due to -e
# TODO(light): If error capturing has to happen elsewhere, it'll be more
# readable/safer to remove -e entirely and handle errors where they occur
ret=0
grep -R --exclude-dir ".git" --exclude-dir "internal" "DO NOT SUBMIT" || ret=$?
if [[ "$ret" == 0 ]]; then
  false
fi
