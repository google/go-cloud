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

# This script runs expensive checks that we don't normally run on Travis, but
# that should run periodically, before each release.
# For example, tests that can't use record/replay, so must be performed live
# against the service provider.
#
# It should be run from the root directory.

# https://coderwall.com/p/fkfaqq/safer-bash-scripts-with-set-euxo-pipefail
# Change to -euxo if debugging.
set -euo pipefail

function usage() {
  echo
  echo "Usage: prereleasechecks.sh <init | run | cleanup>" 1>&2
  echo "  init: creates any needed resources; rerun until it succeeds"
  echo "  runs: runs all needed checks"
  echo "  cleanup: cleans up resources created in init"
  exit 64
}

if [[ $# -ne 1 ]] ; then
  echo "Need at least one argument."
  usage
fi

op="$1"
case "$op" in
  init|run|cleanup);;
  *) echo "Unknown operation '$op'" && usage;;
esac

# TODO: It would be nice to ensure that none of the tests are skipped. For now,
#       we assume that if the "init" steps succeeded, the necessary tests will
#       run.

echo "***** mysql/azuremysql *****"
pushd mysql/azuremysql &> /dev/null
case "$op" in
  init)
    terraform init && terraform apply -var location="centralus" -var resourcegroup="GoCloud" -auto-approve
    ;;
  run)
    # TODO: These tests fail with "Error 9999".
    go test -mod=readonly ./...
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** mysql/cloudmysql *****"
pushd mysql/cloudmysql &> /dev/null
case "$op" in
  init)
    # TODO: This fails with "Error 403: The caller does not have permission, forbidden".
    terraform init && terraform apply -var project="go-cloud-test" -auto-approve
    ;;
  run)
    go test -mod=readonly
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** mysql/rdsmysql *****"
pushd mysql/rdsmysql &> /dev/null
case "$op" in
  init)
    terraform init && terraform apply -var region="us-west-1" -auto-approve
    ;;
  run)
    go test -mod=readonly
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** postgres/cloudpostgres *****"
pushd postgres/cloudpostgres &> /dev/null
case "$op" in
  init)
    # TODO: This fails with "Error 403: The caller does not have permission, forbidden".
    terraform init && terraform apply -var project="go-cloud-test" -auto-approve
    ;;
  run)
    go test -mod=readonly
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** postgres/rdspostgres *****"
pushd postgres/rdspostgres &> /dev/null
case "$op" in
  init)
    terraform init && terraform apply -var region="us-west-1" -auto-approve
    ;;
  run)
    go test -mod=readonly
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** tests/aws *****"
pushd tests/aws &> /dev/null
case "$op" in
  init)
    # TODO: Need some more vars, like app_binary.
    terraform init && terraform apply -var region="us-west-1" -auto-approve
    ;;
  run)
    # TODO: Is this the right way to run this test? There's also a `test.sh`.
    go test -mod=readonly ./...
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** tests/gcp *****"
pushd tests/gcp &> /dev/null
case "$op" in
  init)
    # TODO: This fails with "Error 403: The caller does not have permission, forbidden".
    terraform init && terraform apply -var project="go-cloud-test" -auto-approve
    ;;
  run)
    # TODO: Is this the right way to run this test? There's also a `test.sh`.
    go test -mod=readonly ./...
    ;;
  cleanup)
    terraform destroy
    ;;
esac
popd &> /dev/null


echo
echo "***** pubsub/azure *****"
pushd pubsub/azuresb &> /dev/null
case "$op" in
  init)
    ;;
  run)
    go test -mod=readonly -v -record
    ;;
  cleanup)
    ;;
esac
popd &> /dev/null

echo "SUCCESS!"
