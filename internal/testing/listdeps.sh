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

set -euo pipefail

# To run this script manually to update alldeps:
#
# $ internal/testing/listdeps.sh > internal/testing/alldeps
#
# Make sure to use the same version of Go as used by tests
# (see .github/actions/tests.yml) when updating the alldeps file.
tmpfile=$(mktemp)
function cleanup() {
  rm -rf "$tmpfile"
}
trap cleanup EXIT


sed -e '/^#/d' -e '/^$/d' allmodules | awk '{print $1}' | while read -r path || [[ -n "$path" ]]; do
  ( cd "$path" && go list -mod=readonly -deps -f '{{with .Module}}{{.Path}}{{end}}' ./... >> "$tmpfile")
done

# Sort using the native byte values to keep results from different environment consistent.
LC_ALL=C sort "$tmpfile" | uniq
