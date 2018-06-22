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

# deploy.sh builds the Guestbook server locally and deploys it to GKE.

set -o pipefail

if [[ $# -gt 1 ]]; then
  echo "usage: gcp/deploy.sh [TFSTATE]" 1>&2
  exit 64
fi
guestbook_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )" || exit 1
tf_state="${1:-$guestbook_dir/gcp/terraform.tfstate}"
project_id="$( terraform output -state="$tf_state" project )" || exit 1
cluster_name="$( terraform output -state="$tf_state" cluster_name )" || exit 1
cluster_zone="$( terraform output -state="$tf_state" cluster_zone )" || exit 1
bucket="$( terraform output -state="$tf_state" bucket )" || exit 1
database_instance="$( terraform output -state="$tf_state" database_instance )" || exit 1
database_region="$( terraform output -state="$tf_state" database_region )" || exit 1
motd_var_config="$( terraform output -state="$tf_state" motd_var_config )" || exit 1
motd_var_name="$( terraform output -state="$tf_state" motd_var_name )" || exit 1

GCLOUD() {
  gcloud --quiet --project="$project_id" "$@"
}
log() {
  echo "gcp/deploy.sh:" "$@" 1>&2
}

# Fill in Kubernetes template parameters.
tempdir="$( mktemp -d 2>/dev/null || mktemp -d -t 'guestbook-k8s' )" || exit 1
cleanup() {
  rm -rf "$tempdir"
}
trap cleanup EXIT
image_name="gcr.io/${project_id//:/\/}/guestbook" || exit 1
# TODO(light): Some values might need escaping.
sed \
  -e "s|{{IMAGE}}|${image_name}|" \
  -e "s|{{bucket}}|${bucket}|" \
  -e "s|{{database_instance}}|${database_instance}|" \
  -e "s|{{database_region}}|${database_region}|" \
  -e "s|{{motd_var_config}}|${motd_var_config}|" \
  -e "s|{{motd_var_name}}|${motd_var_name}|" \
  < "$guestbook_dir/gcp/guestbook.yaml.in" \
  > "$tempdir/guestbook.yaml" || exit 1

# Build Guestbook Docker image.
log "Building $image_name..."
( cd "$guestbook_dir" && GOOS=linux GOARCH=amd64 vgo build -o gcp/guestbook ) || exit 1
GCLOUD container builds submit \
  -t "$image_name" \
  "$guestbook_dir/gcp" || exit 1

# Run on Kubernetes.
log "Deploying to $cluster_name..."
GCLOUD container clusters get-credentials \
  --zone="$cluster_zone" "$cluster_name" || exit 1
kubectl apply -f "$tempdir/guestbook.yaml" || exit 1
# Force pull the latest image.
kubectl scale --replicas=0 deployment/guestbook || exit 1
kubectl scale --replicas=1 deployment/guestbook || exit 1
# Wait for endpoint then print it.
log "Waiting for load balancer..."
while true; do
  if endpoint="$( kubectl get service guestbook -o json | jq -r '.status.loadBalancer.ingress[0].ip' )" && [[ "$endpoint" != null ]]; then
    log "Deployed at http://${endpoint}:8080/"
    break
  fi
  sleep 5
done
