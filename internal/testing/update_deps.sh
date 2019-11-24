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

# This script should be run from the root directory.
# It runs "go get -u && go mod tidy" on all modules in
# the repo, to update dependencies. Run runchecks.sh afterwards.
set -euo pipefail

sed -e '/^#/d' -e '/^$/d' allmodules | awk '{print $1}' | while read -r path || [[ -n "$path" ]]; do
  echo "updating $path"
  ( cd "$path" && go get -u &> /dev/null && go mod tidy &> /dev/null || echo "  FAILED! (some modules without code, like samples, are expected to fail)")
done
