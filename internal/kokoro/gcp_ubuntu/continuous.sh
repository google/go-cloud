#!/bin/bash
# Fail on any error
# TODO(cflewis): Commented out pending Kokoro upgrading their go version to 1.10.
# TODO(cflewis): Consider -euxo pipefail as described at
# https://vaneyckt.io/posts/safer_bash_scripts_with_set_euxo_pipefail/
# set -eo pipefail

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

grep -R --exclude-dir ".git" --exclude-dir "internal" "DO NOT SUBMIT" .
if [[ $? == 0 ]]; then
  false
fi

# TODO(cflewis): Remove this once Kokoro moves to go 1.10.
exit 0
