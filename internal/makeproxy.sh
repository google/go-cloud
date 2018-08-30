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

set -o pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: makeproxy.sh OUTDIR" 1>&2
  exit 64
fi

export GO111MODULE=on
GO="${GO:-go}"  # allows user customization of which Go command to use.

# Create destination directory if it doesn't exist.
dest="$1"
mkdir -p "$dest" || exit 1

# Download modules.
downloadpath="$( $GO env GOPATH | sed -e 's/:.*$//' )/pkg/mod/cache/download" || exit 1
$GO mod download || exit 1

# Iterate through all modules used.
mapfile -t all_mods < <( $GO list -m -f '{{.Path}}' all | sed -e '1d' ) || exit 1
for name in "${all_mods[@]}"; do
  # Compute on-disk path to cache. Capital letter 'A' is translated to '!a'.
  version="$( $GO list -m -f '{{.Version}}' "$name" )" || exit 1
  # shellcheck disable=SC2018,SC2019
  dname="$( echo "$name" | sed -e 's:[A-Z]:!\0:g' | tr 'A-Z' 'a-z' )" || exit 1

  # Create destination directory.
  mkdir -p "$dest/$dname/@v" || exit 1

  # Copy individual cache files.
  cp "$downloadpath/$dname/@v/${version}."{info,mod,zip,ziphash} "$dest/$dname/@v" || exit 1

  # Add version to listing.
  echo "$version" >> "$dest/$dname/@v/list" || exit 1
  sort "$dest/$dname/@v/list" | uniq > "$dest/$dname/@v/list.sorted" || exit 1
  mv "$dest/$dname/@v/list.sorted" "$dest/$dname/@v/list" || exit 1
done
