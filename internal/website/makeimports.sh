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

# This script generates Markdown files that will include <meta> suitable for
# "go get"'s import path redirection feature (see
# https://golang.org/cmd/go/#hdr-Remote_import_paths) in the final Hugo output.


# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# except x is too verbose
set -euo pipefail

# Change into repository root.
cd "$(dirname "$0")/../.."
OUTDIR=internal/website/content

for pkg in $(internal/website/listnewpkgs.sh); do
  # Only consider directories that contain Go source files.
  outfile="$OUTDIR/$pkg/_index.md"
  mkdir -p "$OUTDIR/$pkg"
  echo "Generating gocloud.dev/$pkg"
  echo "---" >> "$outfile"
  echo "title: gocloud.dev/$pkg" >> "$outfile"
  echo "type: pkg" >> "$outfile"
  echo "---" >> "$outfile"
done

