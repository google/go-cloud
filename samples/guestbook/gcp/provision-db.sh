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

# provision-db.sh connects to a Cloud SQL database and initializes it
# with SQL from stdin. It's intended to be invoked from Terraform.

set -o pipefail

if [[ $# -ne 5 ]]; then
  echo "usage: provision-db.sh PROJECT SERVICE_ACCOUNT INSTANCE DATABASE ROOT_PASSWORD" 1>&2
  exit 64
fi
mysql_image="mysql:5.6"
cloudsql_proxy_image="gcr.io/cloudsql-docker/gce-proxy:1.11"
project_id="$1"
service_account="$2"
db_instance="$3"
db_name="$4"
db_password="$5"

GCLOUD() {
  gcloud --quiet --project="$project_id" "$@"
}
log() {
  echo "gcp/provision-db.sh:" "$@" 1>&2
}

# Pull the necessary Docker images.
log "Downloading Docker images..."
docker pull "$mysql_image" || exit 1
docker pull "$cloudsql_proxy_image" || exit 1

# Obtain the connection string.
log "Getting database metadata..."
db_conn_str="$( GCLOUD sql instances describe --format='value(connectionName)' "$db_instance" )" || exit 1

# Create a temporary directory to hold the service account key.
# We resolve all symlinks to avoid Docker on Mac issues, see
# https://github.com/google/go-cloud/issues/110.
service_account_voldir="$( cd "$( mktemp -d 2>/dev/null || mktemp -d -t 'guestbook-service-acct' )" && pwd -P )" || exit 1
cleanup1() {
  rm -rf "$service_account_voldir"
}
trap cleanup1 EXIT
log "Created $service_account_voldir"

# Furnish a new service account key.
GCLOUD iam service-accounts keys create \
  --iam-account="$service_account" \
  "$service_account_voldir/key.json" || exit 1
service_account_key_id="$( jq -r .private_key_id "$service_account_voldir/key.json" )" || exit 1
cleanup2() {
  GCLOUD iam service-accounts keys delete \
    --iam-account="$service_account" \
    "$service_account_key_id"
  cleanup1
}
trap cleanup2 EXIT
log "Created service account key $service_account_key_id"

# Start the Cloud SQL Proxy.
log "Starting Cloud SQL proxy..."
proxy_container_id="$( docker run \
  --detach \
  --rm \
  --volume "${service_account_voldir}:/creds" \
  --publish 3306 \
  "$cloudsql_proxy_image" /cloud_sql_proxy \
  -instances="${db_conn_str}=tcp:0.0.0.0:3306" \
  -credential_file=/creds/key.json )" || exit 1
cleanup3() {
  docker stop "$proxy_container_id"
  cleanup2
}
trap cleanup3 EXIT

# Send schema (input comes from stdin).
log "Connecting to database..."
docker run \
  --rm \
  --interactive \
  --link "${proxy_container_id}:proxy" \
  "$mysql_image" \
  sh -c "exec mysql --wait -h\"\$PROXY_PORT_3306_TCP_ADDR\" -P\"\$PROXY_PORT_3306_TCP_PORT\" -uroot -p'$db_password' '$db_name'" || exit 1
