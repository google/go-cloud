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

# Starts a local VerneMQ instance via Docker.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euo pipefail

# Clean up and run VerneMQ.
echo "Starting VerneMQ..."
docker rm -f vernemq &> /dev/null || :
docker run -d -p 1883:1883 -e DOCKER_VERNEMQ_ALLOW_ANONYMOUS=on -e DOCKER_VERNEMQ_ACCEPT_EULA=yes --name vernemq erlio/docker-vernemq &> /dev/null
echo "...done. Run \"docker rm -f vernemq\" to clean up the container."
echo
