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

# Starts all local instances needed for Go CDK tests.
# You must have Docker installed.
# Run this script from the top level of the tree, e.g.:
#   ./internal/testing/start_local_deps.sh

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euo pipefail

./pubsub/kafkapubsub/localkafka.sh
./pubsub/rabbitpubsub/localrabbit.sh
./runtimevar/etcdvar/localetcd.sh
./docstore/mongodocstore/localmongo.sh
./secrets/hashivault/localvault.sh
