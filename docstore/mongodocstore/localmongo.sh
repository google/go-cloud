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

# Starts two local MongoDB instances (v3 and v4) via Docker listening on two
# different ports.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euo pipefail

echo "Starting MongoDB v4 listening on 27017, 27018, 27019..."
docker rm -f mongo41 mongo42 mongo43 mongosetup &> /dev/null || :
docker compose up --wait &> /dev/null
echo "...done. Run \"docker rm -f mongo41 mongo42 mongo43 mongosetup\" to clean up the container."
echo

echo "Starting MongoDB v3 listening on 27020..."
docker rm -f mongo3 &> /dev/null || :
docker run -d --name mongo3  -p 27020:27017 mongo:3 &> /dev/null
echo "...done. Run \"docker rm -f mongo3\" to clean up the container."
echo

