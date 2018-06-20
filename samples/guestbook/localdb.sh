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
container_name="$1"
image="mysql:5.6"
netcat_image="alpine:3.7"

log() {
  echo "$@" 1>&2
}

# Start container
docker_flags=()
docker_flags+=( --rm )
if [[ ! -z "$container_name" ]]; then
  docker_flags+=( "--name=$container_name" )
fi
docker_flags+=( --env 'MYSQL_DATABASE=guestbook' )
docker_flags+=( --env 'MYSQL_ROOT_PASSWORD=password' )
docker_flags+=( --detach )
docker_flags+=( --publish 3306:3306 )
container_id="$( docker run "${docker_flags[@]}" "$image" )" || exit 1
log "Started container $container_id, waiting for healthy"

# shellcheck disable=SC2016
docker run --rm --link "${container_id}:mysql" "$netcat_image" \
  sh -c 'while ! nc -z "$MYSQL_PORT_3306_TCP_ADDR:$MYSQL_PORT_3306_TCP_PORT"; do sleep 1; done' || {
    log "Database port not open; stopping $container_id"
    docker stop "$container_id" > /dev/null
    exit 1
}

# Initialize database schema and users

# shellcheck disable=SC2016
cat "$guestbook_dir/schema.sql" "$guestbook_dir/roles.sql" | docker run \
  --rm \
  --interactive \
  --link "${container_id}:mysql" \
  "$image" \
  sh -c 'exec mysql -h"$MYSQL_PORT_3306_TCP_ADDR" -P"$MYSQL_PORT_3306_TCP_PORT" -uroot -ppassword guestbook' || {
    log "Failed to seed database; stopping $container_id"
    docker stop "$container_id" > /dev/null
    exit 1
}

log "Database running at localhost:3306"
exec docker attach "$container_id"
