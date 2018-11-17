#!/usr/bin/env bash
# Copyright 2018 The Go Cloud Authors
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

# Runs only tests relevant to the current pull request.
# At the moment, this only gates running the Wire test suite.
# See https://github.com/google/go-cloud/issues/28 for solving the
# general case.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euxo pipefail

if [[ $# -gt 0 ]]; then
  echo "usage: runchecks.sh" 1>&2
  exit 64
fi

result=0

# Run Go tests for the root, including coverage.
go test -race -coverpkg=./... -coverprofile=coverage.out ./... || result=1
if [ -f coverage.out ]; then
  # Filter out test and sample packages.
  grep -v test coverage.out | grep -v samples > coverage2.out
  goveralls -coverprofile=coverage2.out -service=travis-ci
fi
wire check ./... || result=1

# Run Go tests for each additional module, without coverage.
for path in "./internal/contributebot" "./samples/appengine"; do
  ( cd "$path" && exec go test ./... ) || result=1
  ( cd "$path" && exec wire check ./... ) || result=1
done
exit $result
