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

# This script downloads and arranges packages in a structure for a
# Go Modules proxy.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euxo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: makeproxy.sh <dir>" 1>&2
  exit 64
fi

GOPATH="$1"

# Download modules for each of the modules in our repo.
for path in "." "./internal/contributebot" "./samples/appengine"; do
  pushd ${path}
  go mod download
  popd
done
