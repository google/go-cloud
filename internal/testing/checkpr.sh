#!/usr/bin/env bash
# Copyright 2018 Google LLC
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

set -o pipefail

if [[ -z "$TRAVIS_BRANCH" || -z "$TRAVIS_PULL_REQUEST_SHA" ]]; then
    echo "TRAVIS_BRANCH and TRAVIS_PULL_REQUEST_SHA environment variables must be set." 1>&2
    exit 1
fi
cd "$( dirname "${BASH_SOURCE[0]}" )/../.." || exit 1

testflags=( "$@" )
module="github.com/google/go-cloud"

# Place changed files into changed array.
mergebase="$(git merge-base -- "$TRAVIS_BRANCH" "$TRAVIS_PULL_REQUEST_SHA")" || exit 1
mapfile -t changed < <( git diff --name-only "$mergebase" "$TRAVIS_PULL_REQUEST_SHA" -- ) || exit 1

has_files() {
  printf "%s\n" "${changed[@]}" | grep -q "$1"
}

# Only run Go checks if Go files were modified.
if ! has_files '\.go$\|^go\.mod$\|/testdata/'; then
  echo "No Go files modified. Skipping tests." 1>&2
  exit 0
fi

# Run the non-Wire tests.
mapfile -t non_wire_pkgs < <( vgo list "$module/..." | grep -F -v "$module/wire" ) || exit 1
vgo test "${testflags[@]}" "${non_wire_pkgs[@]}" || exit 1

# Run Wire tests if the branch made changes under wire/.
if has_files '^wire/'; then
  vgo test "${testflags[@]}" "$module/wire/..." || exit 1
fi

# Run linter and Wire checks.
vgo mod vendor || exit 1
mapfile -t lintdirs < <( find . -type d \
  ! -path "./.git*" \
  ! -path "./tests*" \
  ! -path "./vendor*" \
  ! -path "./wire/internal/wire/testdata*" \
  ! -path "*_demo*" ) || exit 1
golangci-lint run \
  --new-from-rev="$mergebase" \
  --print-welcome=false \
  --disable=deadcode \
  --disable=typecheck \
  --disable=unused \
  --build-tags=wireinject \
  "${lintdirs[@]}" || exit 1
internal/testing/wirecheck.sh || exit 1
