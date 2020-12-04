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

# Starts a local Kafka instance (plus supporting Zookeeper) via Docker.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
set -euo pipefail

# Clean up and run Zookeeper.
echo "Starting Zookeeper (for Kafka)..."
docker rm -f zookeeper &> /dev/null || :
docker run -d --net=host --name=zookeeper -e ZOOKEEPER_CLIENT_PORT=2181 confluentinc/cp-zookeeper:6.0.1 &> /dev/null
echo "...done. Run \"docker rm -f zookeeper\" to clean up the container."
echo

# Clean up and run Kafka.
echo "Starting Kafka..."
docker rm -f kafka &> /dev/null || :
docker run -d --net=host -p 9092:9092 --name=kafka -e KAFKA_ZOOKEEPER_CONNECT=localhost:2181 -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=false -e KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=100 confluentinc/cp-kafka:6.0.1 &> /dev/null
echo "...done. Run \"docker rm -f kafka\" to clean up the container."
echo
