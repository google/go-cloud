#!/bin/bash
# add.sh adds new dependencies to the Go Cloud Development Kit module proxy.

set -euo pipefail

function log() {
  echo "add.sh: $@" 1>&2
}

function die() {
  log "$@"
  exit 1
}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." >/dev/null 2>&1 && pwd)
cd "$root"

rsyncing=true
while getopts ":R" opt; do
  case ${opt} in
    R)
      log 'disabling rsync step'
      rsyncing=false
      ;;
    \?)
      echo "usage: add.sh [-R]

-R disables the gsutil rsync step to prune unused module dependencies
"
      exit 1
      ;;
  esac
done

if ! [[ -d .git ]]; then die 'this script must be run from the root of the repository'; fi

# This approach was suggested by:
# https://github.com/go-modules-by-example/index/tree/master/012_modvendor

log "creating temp dir where we'll create a module download cache"
tgp="$(mktemp -d)"
if [[ $? != 0 ]]; then die 'failed'; fi
function cleanup1() {
	rm -rf "$tgp"
}
trap cleanup1 EXIT

temp=$(mktemp)
if [[ $? != 0 ]]; then die 'failed to create temp file for verbose command output'; fi
# This temp file will be cleaned up at the end, but not on errors since it may
# help in debugging those.

# Copy current module cache into temporary directory as basis.
log 'making temp download dir'
if ! mkdir -p "$tgp/pkg/mod/cache/download"; then die 'failed'; fi
if "$rsyncing"; then
  log "downloading current module cache, logging to $temp"
  if ! gsutil -m rsync -r gs://go-cloud-modules "$tgp/pkg/mod/cache/download" > "$temp" 2>&1; then die 'failed'; fi
else
  log 'skipping download of current module cache because -R was specified'
fi

log "filling cache with all module dependencies from current branch, logging to $temp"
if ! ./internal/proxy/makeproxy.sh "$tgp" > "$temp" 2>&1; then die 'failed while running makeproxy.sh'; fi

# TODO(#1043): see if we can do without this modvendor directory.
log 'moving the temporary cache to modvendor'
if ! rm -rf modvendor; then die 'failed to remove modvendor dir'; fi
if ! cp -rp "$tgp/pkg/mod/cache/download/" modvendor; then die 'copy failed'; fi
function cleanup2() {
  rm -rf modvendor
  GOPATH="$tgp" go clean -modcache
  cleanup1
}
trap cleanup2 EXIT

log 'previewing synchronization of modvendor to the proxy'
# -n: preview only
# -r: recurse directories
# -c: compare checksums not write times
# -d: delete remote files that aren't present locally
if ! gsutil rsync -n -r -c -d modvendor gs://go-cloud-modules; then die 'gsutil rsync failed'; fi
# If the set of packages being added and removed looks good,
# repeat without the -n.
log 'If this looks good, enter "yes" without the quotes to continue, or anything else to cancel:'
read input
if [[ $input != "yes" ]]; then die 'canceled'; fi

log "running gsutil rsync in non-preview mode, sending output to $temp"
if ! gsutil rsync -r -c -d modvendor gs://go-cloud-modules > "$temp" 2>&1; then die 'gsutil rsync failed'; fi

log 'cleaning up'
if ! rm "$temp"; then die "failed to remove $temp"; fi
