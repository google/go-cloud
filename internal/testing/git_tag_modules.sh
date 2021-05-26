#!/usr/bin/env bash
# Copyright 2021 The Go Cloud Development Kit Authors
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
# It creates git tags for all marked modules listed in the allmodules file.
set -euo pipefail

function usage() {
  echo
  echo "Usage: git_tag_modules.sh vX.X.X" 1>&2
  echo "  vX.X.X: the git tag version"
  exit 64
}

if [[ $# -ne 1 ]] ; then
  echo "Need at least one argument"
  usage
fi
version="$1"

sed -e '/^#/d' -e '/^$/d' allmodules | awk '{ print $1, $2}' |  while read -r path update || [[ -n "$path" ]]  ; do
   if [[ "$update" != "yes" ]]; then
        echo "$path is not marked to be released"
        continue
   fi

   tag="$version"
   if [[ "$path" != "." ]]; then
     tag="$path/$version"
   fi
   echo "Creating tag: ${tag}"
   git tag "$tag"
done
