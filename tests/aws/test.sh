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

usage() { echo "Usage: $0 gcp_project_id ssh_key_path [region]"; exit 1; }
log() {
    echo "tests/aws/test.sh:" "$@" 1>&2 
}

set -o pipefail

export gcp_project_id="$1"
export ssh_key_path="$2"
if [[ -z "${gcp_project_id}" || -z "${ssh_key_path}" ]]; then
    usage
fi

export region="$3"
if [[ -z "${region}" ]]; then
    region="us-west-1"
    echo "region not provided, default to ${region}"
fi

cd "$(dirname $0)"
test_dir="$(pwd)"
export TF_IN_AUTOMATION=1

tempdir="$( mktemp -d 2>/dev/null || mktemp -d -t 'go-cloud-aws-test' )" || exit 1
cleanup1() {
    rm -rf "${tempdir}"
}
trap cleanup1 EXIT

log "Building test app..."
( cd "${test_dir}/app" && GOOS=linux GOARCH=amd64 go build -o "${tempdir}/app" ) || exit 1

log "Provisioning AWS resources..."
terraform init -input=false "${test_dir}" || exit 1
cleanup2() {
    log "Tearing down AWS resources..."
    terraform destroy -auto-approve -input=false -var app_binary="${tempdir}/app" -var gcp_project="${gcp_project_id}" "${test_dir}"
    cleanup1
}
trap cleanup2 EXIT
terraform apply -auto-approve -input=false -var app_binary="${tempdir}/app" -var gcp_project="${gcp_project_id}" "${test_dir}" || exit 1

log "Running test..."
host_ip="$( terraform output -state=${test_dir}/terraform.tfstate host_ip )" || exit 1
( cd "${test_dir}/app" && go test -v -args --gcp-project "${gcp_project_id}" --aws-region "${region}" --host-ip "${host_ip}" --key-path "${ssh_key_path}" ) || exit 1
