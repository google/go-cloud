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
# It runs "go mod tidy && go list -deps ./..." on all modules in
# the repo, to ensure that go.mod and go.sum are in the canonical
# form that Travis will verify (see check_mod_tidy.sh).
set -euo pipefail

sed -e '/^#/d' -e '/^$/d' allmodules | awk '{print $1}' | while read -r path || [[ -n "$path" ]]; do
  echo "cleaning up $path"
  ( cd "$path" && go mod tidy && go list -deps ./... &> /dev/null || echo "  FAILED!")
done
