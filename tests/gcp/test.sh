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

usage() { echo "Usage: $0 [project_id]"; exit 1; }

set -e

export project="$1"
if [[ -z "${project}" ]]; then
    usage
fi

cd "$(dirname $0)"
cluster="gcp-test"
test_dir="$(pwd)"

# Create a cluster
gcloud container clusters create "${cluster}" \
    --async \
    --disk-size=10 \
    --scopes="cloud-platform" \
    --num-nodes=1

# Build the app binary and image
cd "${test_dir}"/app
# TODO(shantuo): reuse vgo when it supports gopkg.in better.
# https://github.com/golang/go/issues/25243
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app
gcloud container builds submit -t gcr.io/${project}/gcp-test:latest .
rm -f app

# Wait for the cluster to be created
while true; do
    status="$(gcloud container clusters describe --format='get(status)' ${cluster})"
    if [[ "${status}" == "RUNNING" ]]; then
        break
    fi
    echo "Cluster ${cluster} is ${status}, retrying in 30s."
    sleep 30s
done

# Create a deployment and service for the test app
gcloud container clusters get-credentials "${cluster}"
sed "s/PROJECT_ID/${project}/g" gcp-test.yaml.in > gcp-test.yaml
kubectl create -f gcp-test.yaml

# Build the test driver binary
cd "${test_dir}"/test-driver
# TODO(shantuo): reuse vgo when it supports gopkg.in better.
# https://github.com/golang/go/issues/25243
CGO_ENABLED=0 go build -o test-driver

# Wait for the load balancer to be ready
while true; do
    ingress="$(kubectl describe service/gcp-test | grep '^LoadBalancer Ingress' \
        | cut -d':' -f2 | sed 's/^[ \t]*//')"
    if [[ -n "${ingress}" ]]; then
        ./test-driver --address "http://${ingress}" --project "${project}"
        break
    fi
    echo "Waiting for LoadBalancer to be created, retrying in 30s."
    sleep 30s
done

# TODO(shantuo): add tear-down after finishing the test.
