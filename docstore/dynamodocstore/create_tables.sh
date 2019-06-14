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

# Creates the DynamoDB tables needed for tests.
#
# If a table already exists, this script will fail. To re-create the table, run
#   aws dynamodb delete-table --table-name ...
# and wait until the deletion completes.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# except we want to keep going if there is a failure.
set -uxo pipefail

# The docstore-test-1 table has a single partition key called "name".


aws dynamodb create-table \
  --table-name docstore-test-1 \
  --attribute-definitions AttributeName=name,AttributeType=S \
  --key-schema AttributeName=name,KeyType=HASH \
  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5


# The docstore-test-2 table has both a partition and a sort key, and two indexes.

aws dynamodb create-table \
  --table-name docstore-test-2 \
  --attribute-definitions \
        AttributeName=Game,AttributeType=S \
        AttributeName=Player,AttributeType=S \
        AttributeName=Score,AttributeType=N \
        AttributeName=Time,AttributeType=S \
  --key-schema AttributeName=Game,KeyType=HASH AttributeName=Player,KeyType=RANGE \
  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
  --local-secondary-indexes \
  'IndexName=local,KeySchema=[{AttributeName=Game,KeyType=HASH},{AttributeName=Score,KeyType=RANGE}],Projection={ProjectionType=ALL}' \
  --global-secondary-indexes \
  'IndexName=global,KeySchema=[{AttributeName=Player,KeyType=HASH},{AttributeName=Time,KeyType=RANGE}],Projection={ProjectionType=ALL},ProvisionedThroughput={ReadCapacityUnits=5,WriteCapacityUnits=5}'


