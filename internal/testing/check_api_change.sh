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
# current branch relative to master@HEAD.
# It fails if it finds any, unless there is a commit with BREAKING_CHANGE_OK
# in the first line of the commit message.

set -euo pipefail

echo "Checking for incompatible API changes relative to master@HEAD..."
echo

go install -mod=readonly golang.org/x/exp/cmd/apidiff

MASTER_CLONE_DIR="$(mktemp -d)"
PKGINFO_BRANCH=$(mktemp)
PKGINFO_MASTER=$(mktemp)

function cleanup() {
  rm -rf ${MASTER_CLONE_DIR}
  rm -f ${PKGINFO_BRANCH}
  rm -f ${PKGINFO_MASTER}
}
trap cleanup EXIT

# We compare against master@HEAD. This is unfortunate in some cases: if you're
# working on an out-of-date branch, and master gets some new feature (that has
# nothing to do with your work on your branch), you'll get an error message.
# Thankfully the fix is quite simple: rebase your branch.
git clone https://github.com/google/go-cloud ${MASTER_CLONE_DIR}
echo

incompatible_change_pkgs=()
PKGS=$(cd ${MASTER_CLONE_DIR}; go list ./... | grep -v internal | grep -v test | grep -v samples | grep pubsub)
for pkg in ${PKGS}; do
  echo "Testing ${pkg}..."

  # Compute export data for the current branch.
  package_deleted=0
  apidiff -w ${PKGINFO_BRANCH} ${pkg} || package_deleted=1
  if [ ${package_deleted} -eq 1 ]; then
    echo "  Package ${pkg} was deleted! Recording as an incompatible change.";
    incompatible_change_pkgs+=(${pkg});
    continue;
  fi

  # Compute export data for master@HEAD.
  (cd ${MASTER_CLONE_DIR}; apidiff -w ${PKGINFO_MASTER} ${pkg})

  # Print all changes for posterity.
  apidiff ${PKGINFO_MASTER} ${PKGINFO_BRANCH}

  # Note if there's an incompatible change.
  ic=$(apidiff -incompatible ${PKGINFO_MASTER} ${PKGINFO_BRANCH})
  if [ ! -z "${ic}" ]; then
    incompatible_change_pkgs+=(${pkg});
  fi
done
echo

if [ ${#incompatible_change_pkgs[@]} -eq 0 ]; then
  # No incompatible changes, we are good.
  echo "OK: No incompatible changes found."
  exit 0;
fi
echo "Found breaking API change(s) in: ${incompatible_change_pkgs[@]}."

# Found incompatible changes; see if they were declared as OK via a commit.
if git cherry -v master | grep -q "BREAKING_CHANGE_OK"; then
  echo "Allowing them due to a commit message with BREAKING_CHANGE_OK.";
  exit 0;
fi

echo "FAIL. If this is expected and OK, you can pass this check by adding a commit with BREAKING_CHANGE_OK in the first line of the message."
exit 1

