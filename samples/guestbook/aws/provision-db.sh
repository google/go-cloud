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

# provision-db.sh connects to an RDS database and initializes it
# with SQL from stdin. It's intended to be invoked from Terraform.

set -o pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: aws/provision-db.sh HOST SECURITY_GROUP DATABASE ROOT_PASSWORD" 1>&2
  exit 64
fi
mysql_image="mysql:5.6"
db_host="$1"
security_group_id="$2"
db_name="$3"
db_password="$4"

AWS() {
  aws "$@"
}
log() {
  echo "aws/provision-db.sh:" "$@" 1>&2
}

# Pull the necessary Docker images.
log "Downloading Docker images..."
docker pull "$mysql_image" || exit 1

# Create a temporary directory to hold the certificates.
# We resolve all symlinks to avoid Docker on Mac issues, see
# https://github.com/google/go-cloud/issues/110.
tempdir="$( cd "$( mktemp -d 2>/dev/null || mktemp -d -t 'guestbook-ca' )" && pwd -P )" || exit 1
cleanup1() {
  rm -rf "$tempdir"
}
trap cleanup1 EXIT
curl -fsSL 'https://s3.amazonaws.com/rds-downloads/rds-ca-2015-root.pem' > "$tempdir/rds-ca.pem" || exit 1

# Add a temporary ingress rule.
AWS ec2 authorize-security-group-ingress \
  --group-id="$security_group_id" \
  --protocol=tcp \
  --port=3306 \
  --cidr=0.0.0.0/0 || exit 1
cleanup2() {
  log "Removing ingress rule..."
  AWS ec2 revoke-security-group-ingress \
    --group-id="$security_group_id" \
    --protocol=tcp \
    --port=3306 \
    --cidr=0.0.0.0/0
  cleanup1
}
trap cleanup2 EXIT
log "Added ingress rule to $security_group_id for port 3306"

# Send schema (input comes from stdin).
log "Connecting to database..."
docker run \
  --rm \
  --interactive \
  --volume "${tempdir}:/ca" \
  "$mysql_image" \
  sh -c "exec mysql -h'$db_host' -uroot -p'$db_password' --ssl-ca=/ca/rds-ca.pem '$db_name'" || exit 1
