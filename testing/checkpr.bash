#!/bin/bash
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
# At the moment, this only gates running the Goose test suite.
# See https://github.com/google/go-cloud/issues/28 for solving the
# general case.

set -o pipefail

testflags=( "$@" )
module="github.com/google/go-cloud"

# Run the non-Goose tests.
vgo test "${testflags[@]}" $(vgo list "$module/..." | grep -F -v "$module/goose") || exit 1

# Run Goose tests if the branch made changes under goose/.
if [[ ! -z "$TRAVIS_PULL_REQUEST_BRANCH" && ! -z "$TRAVIS_PULL_REQUEST_SHA" ]]; then
  mergebase="$(git merge-base -- "$TRAVIS_PULL_REQUEST_BRANCH" "$TRAVIS_PULL_REQUEST_SHA")" || exit 1
  if git diff --name-only "$mergebase" "$TRAVIS_PULL_REQUEST_SHA" -- | grep -q '^goose/'; then
    vgo test "${testflags[@]}" "$module/goose/..." || exit 1
  fi
fi
