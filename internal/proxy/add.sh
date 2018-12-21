#!/bin/bash
# add.sh adds new dependencies to the Go Cloud module proxy. This script
# should be run from the root of the repository.

set -o pipefail

function log() {
	echo "add.sh: $@" 1>&2
}

function die() {
	log "$@"
	exit 1
}

if ! [[ -d .git ]]; then die 'this script must be run from the root of the repository'; fi

# This approach was suggested by:
# https://github.com/go-modules-by-example/index/tree/master/012_modvendor

log "creating temp dir where we'll create a module download cache"
tgp="$(mktemp -d)"
if [[ $? != 0 ]]; then die 'failed'; fi

temp=$(mktemp)
if [[ $? != 0 ]]; then die 'failed to create temp file for verbose command output'; fi

# Copy current module cache into temporary directory as basis.
# Periodically, someone on the project should go through this process without
# running this step to prune unused dependencies.
log 'making temp download dir'
if ! mkdir -p "$tgp/pkg/mod/cache/download"; then die 'failed'; fi
log "downloading current module cache, logging to $temp"
if ! gsutil -m rsync -r gs://go-cloud-modules "$tgp/pkg/mod/cache/download" > $temp 2>&1; then die 'failed'; fi

log "filling cache with all module dependencies from current branch, logging to $temp"
if ! ./internal/proxy/makeproxy.sh "$tgp" > $temp 2>&1; then die 'failed while running makeproxy.sh'; fi

log 'moving the temporary cache to modvendor'
if ! rm -rf modvendor; then die 'failed to remove modvendor dir'; fi
if ! cp -rp "$tgp/pkg/mod/cache/download/" modvendor; then die 'copy failed'; fi

log 'cleaning up temporary cache'
if ! GOPATH="$tgp" go clean -modcache; then die 'failed while running go clean'; fi
if ! rm -rf "$tgp"; then die "failed to remove temp dir"; fi
unset tgp

log 'previewing synchronization of modvendor to the proxy'
# -n: preview only
# -r: recurse directories
# -c: compare checksums not write times
# -d: delete remote files that aren't present locally
if ! gsutil rsync -n -r -c -d modvendor gs://go-cloud-modules; then die 'gsutil rsync failed'; fi
# If the set of packages being added and removed looks good,
# repeat without the -n.
echo 'if this looks good, hit ENTER to continue, otherwise hit ctrl-C to quit'
read
if [[ $? != 0 ]]; then die 'cancelled'; fi
log "running gsutil rsync in non-preview mode, sending output to $temp"
if ! gsutil rsync -r -c -d modvendor gs://go-cloud-modules > $temp 2>&1; then die 'gsutil rsync failed'; fi

log 'cleaning up'
if ! rm -rf modvendor; then die 'failed to remove modvendor dir'; fi
