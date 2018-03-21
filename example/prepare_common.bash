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

set -euo pipefail
# set -x

# logging from http://stackoverflow.com/a/7287873/608382
log() { printf '%s: %s\n' "$(date +"%T")" "$*"; }
error() { log "ERROR - $*" >&2; }
fatal() { log "FATAL - $*" >&2; exit 1; }

# multiple traps from https://www.reddit.com/r/programming/comments/15mnxe/how_exit_traps_can_make_your_bash_scripts_way/c7o6yqf
_ON_EXIT=( )
_exit () {
    [[ ${#_ON_EXIT[@]} -gt 0 ]] || return
    for i in $(seq ${#_ON_EXIT[@]} -1 1); do # in reverse order
        ( ${_ON_EXIT[$(( i - 1 ))]} ) || true
    done
}
trap "_exit" EXIT
_on_exit () {
    _ON_EXIT+=( "$1" )
}
