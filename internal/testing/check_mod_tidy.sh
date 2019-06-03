#!/usr/bin/env bash
# Copyright 2019 The Go Cloud Development Kit Authors
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

# This script checks to see if `go mod tidy` has been run on the module
# in the current directory.
#
# It exits with status 1 if "go mod tidy && go list -deps ./..." would
# make changes.
#
# TODO(rvangent): Replace this with `go mod tidy --check` when it exists:
# https://github.com/golang/go/issues/27005.
#
# TODO(rvangent): Drop the "go list" part here and in gomodcleanup.sh once
# https://github.com/golang/go/issues/31248 is fixed.

set -euo pipefail

TMP_GOMOD=$(mktemp)
TMP_GOSUM=$(mktemp)

function cleanup() {
  # Restore the original files in case "go mod tidy" made changes.
  if [[ -f "$TMP_GOMOD" ]]; then
    mv "$TMP_GOMOD" ./go.mod
  fi
  if [[ -f "$TMP_GOSUM" ]]; then
    mv "$TMP_GOSUM" ./go.sum
  fi
}
trap cleanup EXIT

# Make copies of the current files.
cp ./go.mod "$TMP_GOMOD"
cp ./go.sum "$TMP_GOSUM"

# Modifies the files in-place.
go mod tidy
go list -deps ./... &> /dev/null

# Check for diffs.
diff -u "$TMP_GOMOD" ./go.mod
diff -u "$TMP_GOSUM" ./go.sum
