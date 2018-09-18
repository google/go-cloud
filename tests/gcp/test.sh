#!/bin/bash

# Copyright 2018 The Go Cloud Authors
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

project_id="$1"
if [[ -z "$project_id" ]]; then
  echo "usage: test.sh PROJECT" 1>&2
  exit 64
fi
test_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )" || exit 1
export TF_IN_AUTOMATION=1

GCLOUD() {
  gcloud --quiet --project="$project_id" "$@"
}
log() {
  echo "tests/gcp/test.sh:" "$@" 1>&2
}

tempdir="$( mktemp -d 2>/dev/null || mktemp -d -t 'go-cloud-gcp-test' )" || exit 1
cleanup1() {
  rm -rf "$tempdir"
}
trap cleanup1 EXIT

log "Provisioning GCP resources..."
terraform init -input=false "$test_dir" || exit 1
cleanup2() {
  log "Tearing down GCP resources..."
  terraform destroy -auto-approve -var project="$project_id" "$test_dir"
  cleanup1
}
trap cleanup2 EXIT
terraform apply -auto-approve -input=false -var project="$project_id" "$test_dir" || exit 1

# Build the app binary.
log "Building application..."
app_image="gcr.io/${project_id//:/\/}/gcp-test"
build_id="$( GCLOUD container builds submit \
  --async \
  --format='value(id)' \
  --config="$test_dir/app/cloudbuild.yaml" \
  --substitutions="_IMAGE_NAME=$app_image" \
  "$test_dir/../.." )" || exit 1
GCLOUD container builds log --stream "$build_id" || exit 1

# Deploy the app on the cluster.
log "Deploying..."
# TODO(light): Some values might need escaping.
sed -e "s|{{IMAGE}}|${app_image}:${build_id}|" \
  < "$test_dir/app/gcp-test.yaml.in" \
  > "$tempdir/gcp-test.yaml" || exit 1
cluster_name="$( terraform output -state="$test_dir/terraform.tfstate" cluster_name )" || exit 1
cluster_zone="$( terraform output -state="$test_dir/terraform.tfstate" cluster_zone )" || exit 1
GCLOUD container clusters get-credentials \
  --zone="$cluster_zone" "$cluster_name" || exit 1
kubectl apply -f "$tempdir/gcp-test.yaml" || exit 1

# Wait for load balancer to come up.
log "Waiting for load balancer..."
while true; do
  if endpoint="$( kubectl get service gcp-test -o json | jq -r '.status.loadBalancer.ingress[0].ip' )" && [[ "$endpoint" != null ]]; then
    break
  fi
  sleep 5
done

# Run test driver.
log "Running test:"
( cd "$test_dir/app" && go test -v -args --address "http://${endpoint}" --project "$project_id" ) || exit 1
