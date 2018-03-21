#!/usr/bin/env bash

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

. prepare_common.bash

# exists $1 $2 checks if a resource ($2) exists in the listing of resource
# types ($1). E.g.:
#
#   exists "sql databases" "my-database" "--instance=my-instance"
#
exists() {
  if [ -z ${3+x} ]; then
    set -- "$1" "$2" ''
  fi
  count=$(gcloud $1 list $3 --filter="NAME=$2" | (grep -c "$2" || true))
  if [ $? -ne 0 ]; then
    fatal "Failed to check service '$1' for resource '$2' with flags '$3'"
  fi
  if [ $count -gt 0 ]; then
    return 0 # true
  fi
  return 1 # false
}

# ensureAPI enables a GCP API if it isn't already enabled. E.g.:
#
#   enxureAPI "sql-component.googleapis.com"
#
ensureAPI() {
  if ! exists "services" $1; then
    log "Enabling API: $1"
    gcloud beta services enable $1
  fi
}

# increment EPOCH to cause all resources to be re-created on next execution
EPOCH=1

PROJECT=$(gcloud config get-value core/project)
log "Project: $PROJECT (existing)"

###################
# Service account #
###################
ACCOUNT=go-cloud-demo-account$EPOCH
FULLACCOUNT=$ACCOUNT@$PROJECT.iam.gserviceaccount.com

if ! exists "iam service-accounts" "$ACCOUNT"; then
  log "Service account: $ACCOUNT (new), granting 'roles/owner' to $PROJECT"
  gcloud beta iam service-accounts create $ACCOUNT --display-name $ACCOUNT
  gcloud beta projects add-iam-policy-binding $PROJECT --member serviceAccount:$FULLACCOUNT --role 'roles/owner'
else
  log "Service account: $ACCOUNT (existing)"
fi

CREDENTIALSFILE=credentials_gcp.json
log "Credentials file: $CREDENTIALSFILE"
gcloud iam service-accounts keys create "$CREDENTIALSFILE" --iam-account=$FULLACCOUNT

########################
# Google Cloud Storage #
########################
BUCKET="go-cloud-demo"
BLOBNAME="go-cloud-demo-gcp-hello.txt"  # already created, world-readable
BLOBURI="gs://$BUCKET/$BLOBNAME"

####################
# Google Cloud SQL #
####################
DBINSTANCE="go-cloud-demo-db-instance$EPOCH"
# DBINSTANCE="go-cloud-demo-db-instance-$RANDOM"
REGION="us-central1"
PASSWORD="password"
DBNAME="go-cloud-demo-db"

ensureAPI sql-component.googleapis.com
ensureAPI sqladmin.googleapis.com # needed by Go Cloud SQL proxy library

if ! exists "sql instances" "$DBINSTANCE" "--filter=region:$REGION"; then
  log "Cloud MySQL instance: $DBINSTANCE (new). This may take a long time. You can monitor progress at:"
  log "https://console.cloud.google.com/sql/instances?project=$PROJECT"
  out=$(gcloud sql instances create $DBINSTANCE --tier=db-f1-micro --region=$REGION --async)
  op=$(echo "$out" | grep name | cut -f 2 -d ' ')
  while true; do
    if out=$(gcloud sql operations wait $op 2>&1); then
      break
    fi
    if echo $out | grep "taking longer than expected"; then
      continue
    else
      break
    fi
  done
  gcloud sql users set-password root % --password=$PASSWORD --instance=$DBINSTANCE
else
  log "Cloud MySQL instance: $DBINSTANCE (existing)"
fi

# populate database
PORT=3306
GOOGLE_APPLICATION_CREDENTIALS=$CREDENTIALSFILE cloud_sql_proxy -instances=$PROJECT:$REGION:$DBINSTANCE=tcp:$PORT &
PROXYPID=$!
function stop_proxy {
  kill -INT $PROXYPID
}
_on_exit stop_proxy
sleep 3
mysql --host=127.0.0.1 --port=$PORT --user=root --password=password <<EOF
DROP DATABASE IF EXISTS \`$DBNAME\`;
CREATE DATABASE \`$DBNAME\`;
USE \`$DBNAME\`
CREATE TABLE blobs (
name VARCHAR(255),
blob_name VARCHAR(255)
);
INSERT INTO blobs (name, blob_name) VALUES ('hellomessage', '$BLOBURI');
quit
EOF

DBURI="mysql://root:$PASSWORD@cloudsql($PROJECT:$REGION:$DBINSTANCE)/$DBNAME?charset=utf8&parseTime=True&loc=UTC"

###############################
# Google Runtime Configurator #
###############################
CONFIGNAME="go-cloud-demo$EPOCH"

ensureAPI runtimeconfig.googleapis.com

if ! exists "beta runtime-config configs" $CONFIGNAME; then
  log "Runtime configuration: $CONFIGNAME (new)"
  gcloud beta runtime-config configs create $CONFIGNAME --description "go-cloud demo configuration"
else
  log "Runtime configuration: $CONFIGNAME (existing)"
fi

log "Setting configuration variable: $CONFIGNAME=$DBURI"
gcloud beta runtime-config configs variables set --config-name="$CONFIGNAME" go-cloud-demo-db "$DBURI"

###################################
# Start application (for testing) #
###################################
log "Starting server"
GOOGLE_APPLICATION_CREDENTIALS=$CREDENTIALSFILE go run -tags 'gcp' example.go -config=grc://$CONFIGNAME
