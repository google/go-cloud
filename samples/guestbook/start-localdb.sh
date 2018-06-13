#!/bin/bash
# Copyright 2018 Google LLC
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

set -o pipefail

if [[ $# -gt 1 ]]; then
  echo "usage: run-localdb.sh [CONTAINER_NAME]" 1>&2
  exit 64
fi
guestbook_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )" || exit 1
container_name="${1:-guestbook-sql}"
image="mysql:5.6"

log() {
  echo "$@" 1>&2
}

# Start container
docker run \
  --rm \
  --name="$container_name" \
  --env MYSQL_DATABASE=guestbook \
  --env MYSQL_ROOT_PASSWORD=password \
  --detach \
  --publish 3306:3306 \
  "$image" || exit 1
log "Started container $container_name"

# Initialize database schema and users
cat schema.sql roles.sql | docker run \
  --rm \
  --interactive \
  --link "${container_name}:mysql" \
  "$image" \
  sh -c 'exec mysql -h"$MYSQL_PORT_3306_TCP_ADDR" -P"$MYSQL_PORT_3306_TCP_PORT" -uroot -ppassword guestbook' || { \
    log "Failed to seed database; stopping $container_name"; \
    docker stop "$container_name"; \
    exit 1 }

log "Database running at localhost:3306. Run 'docker stop $container_name' to stop."
