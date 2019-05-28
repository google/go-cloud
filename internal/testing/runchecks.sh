#!/usr/bin/env bash
# Copyright 2018 The Go Cloud Development Kit Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script runs all checks for Go CDK on Travis, including go test suites,
# compatibility checks, consistency checks, Wire, etc.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# Change to -euxo if debugging.
set -euo pipefail

if [[ $# -gt 0 ]]; then
  echo "usage: runchecks.sh" 1>&2
  exit 64
fi

# The following logic lets us skip the (lengthy) installation process and tests
# in some cases where the PR carries trivial changes that don't affect the code
# (such as documentation-only).
if [[ ! -z "${TRAVIS_BRANCH:-}" ]] && [[ ! -z "${TRAVIS_PULL_REQUEST_SHA:-}" ]]; then
  tmpfile=$(mktemp)
  function cleanup() {
    rm -rf "$tmpfile"
  }
  trap cleanup EXIT

  mergebase="$(git merge-base -- "$TRAVIS_BRANCH" "$TRAVIS_PULL_REQUEST_SHA")"
  if [[ -z $mergebase ]]; then
    echo "merge-base empty. Please ensure that the PR is mergeable."
    exit 1
  fi
  git diff --name-only "$mergebase" "$TRAVIS_PULL_REQUEST_SHA" -- > "$tmpfile"

  # Find out if the diff has any files that are neither:
  #
  # * in internal/website, nor
  # * end with .md
  #
  # If there are no such files, grep returns 1 and we don't have to run the
  # tests.
  echo "The following files changed:"
  cat "$tmpfile"
  if grep -vP "(^internal/website|.md$)" "$tmpfile"; then
    echo "--> Found some non-trivial changes, running tests"
  else
    echo "--> Diff doesn't affect tests; not running them"
    exit 0
  fi
fi


# start_local_deps.sh requires that Docker is installed, via Travis services,
# which are only supported on Linux.
# Tests that depend on them should check the Travis environment before running.
# Don't do this when running locally, as it's slow; user should do it.
if [[ "${TRAVIS_OS_NAME:-}" == "linux" ]]; then
  echo
  echo "Starting local dependencies..."
  ./internal/testing/start_local_deps.sh
  go install -mod=readonly github.com/google/wire/cmd/wire
fi

result=0

rootdir="$(pwd)"
while read -r path || [[ -n "$path" ]]; do
  echo
  echo "******************************"
  echo "* Checking module: $path"
  echo "******************************"
  echo
  pushd "$path" &> /dev/null

  # Run Go tests.
  # Only do coverage for the Linux build on Travis because it is slow, and
  # codecov will only save the last one anyway.
  if [[ "${TRAVIS_OS_NAME:-}" == "linux" ]]; then
    echo "Running Go tests with coverage..."
    go test -mod=readonly -json -race -coverpkg=./... -coverprofile=coverage.out ./... | go run "$rootdir"/internal/testing/test-summary/test-summary.go -progress || result=1
    if [ -f coverage.out ] && [ $result -eq 0 ]; then
      # Filter out test packages.
      grep -v test coverage.out > coverage2.out
      mv coverage2.out coverage.out
      bash <(curl -s https://codecov.io/bash)
      rm coverage.out
    fi
  else
    echo "Running Go tests..."
    # TODO(rvangent): Special case modules to skip for Windows. Perhaps
    # this should be data-driven by allmodules?
    # (https://github.com/google/go-cloud/issues/2111).
    if [[ "${TRAVIS_OS_NAME:-}" == "windows" ]] && ([[ "$path" == "internal/contributebot" ]] || [[ "$path" == "internal/website" ]]); then
      echo "  Skipping tests on Window"
    else
      go test -mod=readonly -json -race ./... | go run "$rootdir"/internal/testing/test-summary/test-summary.go -progress || result=1
    fi
  fi
  # Do these additional checks for the Linux build on Travis, or when running
  # locally.
  if [[ "${TRAVIS_OS_NAME:-linux}" == "linux" ]]; then
    echo "Running go mod tidy:"
    ( "$rootdir"/internal/testing/check_mod_tidy.sh && echo "  OK" ) || { echo "FAIL: please run ./internal/testing/gomodcleanup.sh" && result=1; }
    echo "Running wire diff:"
    ( wire diff ./... && echo "  OK " ) || { echo "FAIL: wire diff found diffs!" && result=1; }
  fi
  popd &> /dev/null
done < <( sed -e '/^#/d' -e '/^$/d' allmodules )
# The above filters out comments and empty lines from allmodules.

# The rest of these checks are not OS-specific, so we only run them for the
# Linux build on Travis, or when running locally.
if [[ "${TRAVIS_OS_NAME:-linux}" != "linux" ]]; then
  exit $result
fi

echo
echo "******************************"
echo "* Doing non-module checks"
echo "******************************"
echo
echo "Ensuring .go files are formatted with gofmt -s..."
DIFF="$(gofmt -s -d .)"
if [ -n "$DIFF" ]; then
  echo "FAIL: please run 'gofmt -s -w .' and commit the result"
  echo "$DIFF";
  exit 1;
else
  echo "  OK"
fi;


echo
echo "Ensuring that go mod tidy has been run for the root module..."
( ./internal/testing/check_mod_tidy.sh && echo "  OK" ) || {
  echo "FAIL: please run ./internal/testing/gomodcleanup.sh" && result=1
}


echo
echo "Ensuring that gocdk static content is up to date..."
tmpstaticgo=$(mktemp)
function cleanupstaticgo() {
  rm -rf "$tmpstaticgo"
}
trap cleanupstaticgo EXIT
pushd internal/cmd/gocdk/ &> /dev/null
go run -mod=readonly generate_static.go -- "$tmpstaticgo" &> /dev/null
( diff -u ./static.go - < "$tmpstaticgo" && echo "  OK" ) || {
  echo "FAIL: gocdk static files are out of date; please run go generate in internal/cmd/gocdk and commit the updated static.go" && result=1
}
popd &> /dev/null


if [[ $(go version) == *go1\.12* ]]; then
  echo
  echo "Ensuring that there are no dependencies not listed in ./internal/testing/alldeps..."
  ( ./internal/testing/listdeps.sh | diff -u ./internal/testing/alldeps - && echo "  OK" ) || {
    echo "FAIL: dependencies changed; run: internal/testing/listdeps.sh > internal/testing/alldeps" && result=1
    # Module behavior may differ across versions.
    echo "using go version 1.12."
  }
fi


echo
echo "Ensuring that any new packages have the corresponding entries in Hugo..."
missing_packages="$(internal/website/listnewpkgs.sh)"
if ! [[ -z "$missing_packages" ]]; then
  echo "FAIL: missing package meta tags for:" 1>&2
  echo "$missing_packages" 1>&2
  result=1
else
  echo "  OK"
fi


echo
echo "Ensuring that all examples used in Hugo match what's in source..."
(internal/website/gatherexamples/run.sh | diff -u internal/website/data/examples.json - > /dev/null && echo "  OK") || {
  echo "FAIL: examples changed; run: internal/website/gatherexamples/run.sh > internal/website/data/examples.json"
  result=1
}

# For pull requests, check if there are undeclared incompatible API changes.
# Skip this if we're already going to fail since it is expensive.
if [[ ${result} -eq 0 ]] && [[ ! -z "${TRAVIS_BRANCH:-}" ]] && [[ ! -z "${TRAVIS_PULL_REQUEST_SHA:-}" ]]; then
  echo
  ./internal/testing/check_api_change.sh || result=1;
fi


echo
if [[ ${result} -eq 0 ]]; then
  echo "SUCCESS!"
else
  echo "FAILED; see above for more info."
fi

exit $result
