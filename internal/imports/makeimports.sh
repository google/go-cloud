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

# This script generates html files that contain go-import meta tags,
# suitable for "go get"'s import path redirection feature (see
# https://golang.org/cmd/go/#hdr-Remote_import_paths).
# It also adds go-source tags so godoc.org can link to the proper source files.
#
# This script must be run at the repo root.


# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# except x is too verbose
set -euo pipefail

OUTDIR=makeimports-output

TEMPLATE=internal/imports/makeimports.template.html

if [ ! -f "$TEMPLATE" ]; then
  echo "Template not found: $TEMPLATE"
  echo "This script must be run from the repo root."
  exit 1
fi


if [ -e $OUTDIR ]; then
  echo "The $OUTDIR directory exists. Move or remove it, then re-run."
  exit 1
fi

shopt -s nullglob  # glob patterns that don't match turn into the empty string, instead of themselves

function files_exist() {  # assumes nullglob
  [[ ${1:-""} != "" ]]
}


# Generate top-level index.html.
echo "Generating gocloud.dev"
mkdir -p "$OUTDIR"
# Replace the redirect URL with the github repo home page.
cat "$TEMPLATE" | sed \
        -e "s|url=https://godoc.org/gocloud.dev{{path}}|url=https://github.com/google/go-cloud|" \
         -e "s|{{path}}||" > "$OUTDIR/index.html"


# Find all directories that do not begin with '.' or contain 'testdata'. Use the %P printf
# directive to remove the initial './'.
for pkg in $(find . -type d \( -name '.?*' -prune -o -name testdata -prune -o -printf '%P ' \)); do
  # Only consider directories that contain Go source files.
  if files_exist $pkg/*.go; then
    mkdir -p "$OUTDIR/$pkg"
    echo "Generating gocloud.dev/$pkg"
    cat "$TEMPLATE" | sed -e "s|{{path}}|/$pkg|" > "$OUTDIR/$pkg/index.html"
  fi
done

