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

# This script checks to see if there are any incompatible API changes on the
# current branch relative to the upstream branch.
# It fails if it finds any, unless there is a commit with BREAKING_CHANGE_OK
# in the first line of the commit message.
#
# It checks all modules listed in allmodules, and skips packages with
# "internal" or "test" in their name.
#
# It expects to be run at the root of the repository, and that HEAD is pointing
# to a commit that merges between the pull request and the upstream branch
# (TRAVIS_BRANCH). This is what Travis does (see
# https://docs.travis-ci.com/user/pull-requests/ for details), but if you
# are testing this script manually, you may need to manually create a merge
# commit.

set -euo pipefail

UPSTREAM_BRANCH="${TRAVIS_BRANCH:-master}"
echo "Checking for incompatible API changes relative to ${UPSTREAM_BRANCH}..."

INSTALL_DIR="$(mktemp -d)"
MASTER_CLONE_DIR="$(mktemp -d)"
PKGINFO_BRANCH=$(mktemp)
PKGINFO_MASTER=$(mktemp)

function cleanup() {
  rm -rf "$INSTALL_DIR" "$MASTER_CLONE_DIR"
  rm -f "$PKGINFO_BRANCH" "$PKGINFO_MASTER"
}
trap cleanup EXIT

# Move to a temporary directory while installing apidiff to avoid changing
# the local .mod file.
( cd "$INSTALL_DIR" && exec go mod init unused )
( cd "$INSTALL_DIR" && exec go install golang.org/x/exp/cmd/apidiff )

git clone -b "$UPSTREAM_BRANCH" . "$MASTER_CLONE_DIR" &> /dev/null

# Run the following checks in the master directory
ORIG_DIR="$(pwd)"
cd "$MASTER_CLONE_DIR"

incompatible_change_pkgs=()
while read -r path || [[ -n "$path" ]]; do
  echo "  checking packages in module $path"
  pushd "$path" &> /dev/null

  PKGS=$(go list ./...)
  for pkg in $PKGS; do
    if [[ "$pkg" =~ "test" ]] || [[ "$pkg" =~ "internal" ]] || [[ "$pkg" =~ "samples" ]]; then
      continue
    fi
    echo "    checking ${pkg}..."

    # Compute export data for the current branch.
    package_deleted=0
    (cd "$ORIG_DIR/$path" && apidiff -w "$PKGINFO_BRANCH" "$pkg") || package_deleted=1
    if [[ $package_deleted -eq 1 ]]; then
      echo "    package ${pkg} was deleted! Recording as an incompatible change.";
      incompatible_change_pkgs+=("${pkg}");
      continue;
    fi

    # Compute export data for master@HEAD.
    apidiff -w "$PKGINFO_MASTER" "$pkg"

    # Print all changes for posterity.
    apidiff "$PKGINFO_MASTER" "$PKGINFO_BRANCH"

    # Note if there's an incompatible change.
    ic=$(apidiff -incompatible "$PKGINFO_MASTER" "$PKGINFO_BRANCH")
    if [ -n "$ic" ]; then
      incompatible_change_pkgs+=("$pkg");
    fi
  done
  popd &> /dev/null
done < <( sed -e '/^#/d' -e '/^$/d' allmodules | awk '{print $1}' )

if [ ${#incompatible_change_pkgs[@]} -eq 0 ]; then
  # No incompatible changes, we are good.
  echo "OK: No incompatible changes found."
  exit 0;
fi
echo "Found breaking API change(s) in: ${incompatible_change_pkgs[*]}."

# Found incompatible changes; see if they were declared as OK via a commit.
cd "$ORIG_DIR"
if git cherry -v master | grep -q "BREAKING_CHANGE_OK"; then
  echo "Allowing them due to a commit message with BREAKING_CHANGE_OK.";
  exit 0;
fi

echo "FAIL. If this is expected and OK, you can pass this check by adding a commit with BREAKING_CHANGE_OK in the first line of the message."
exit 1
