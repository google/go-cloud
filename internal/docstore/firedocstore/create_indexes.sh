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

# Creates the Firestore indexes needed for tests.
# Takes one argument: the GCP project ID.
#
# If an index already exists, this script will fail. To re-create the index, delete
# it from the UI at https://firebase.corp.google.com/project/$project_id/database/firestore/indexes.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# Except we want to keep going if there is a failure, and x is too verbose.
set -uo pipefail

project_id="${1:-}"
if [[ -z "$project_id" ]]; then
  echo "usage: create_indexes.sh PROJECT" 1>&2
  exit 64
fi

echo "Creating indexes for $project_id"
echo "UI at https://firebase.corp.google.com/project/$project_id/database/firestore/indexes"

collection=docstore-test-2

function create_index() {
   gcloud --project "$project_id" beta firestore indexes composite create --collection-group "$collection" \
    --field-config field-path=$1,order=$2 --field-config field-path=$3,order=$4
}

set -x

create_index Player ascending Score ascending
create_index Game   ascending Score ascending
create_index Player ascending Time  ascending
create_index Game   ascending Player ascending
create_index Game   ascending Player descending
