#!/usr/bin/env bash

# Upgrade a dependency across all dependent submodules.
#
# Usage:
# 	./contrib/upgrade-dependency.sh github.com/foo/bar
#
# Requires ripgrep.

if [[ -z "$1" ]]; then
	echo "Usage: ./contrib/upgrade-dependency.sh github.com/foo/bar"
	exit 1
fi

if [[ -z "$(which rg)" ]]; then
	echo "This script requires ripgrep. Please visit https://github.com/BurntSushi/ripgrep to learn how to install this utility."
	exit 1
fi

set -euo pipefail

DEPENDENCY="$1"
printf "%s\n" "Upgrading \"$DEPENDENCY\"."

ROOT="$(git rev-parse --show-toplevel)"

GO_MOD_FILES=$(rg -l "$DEPENDENCY" | grep "go.mod")
printf "%s\n" "Module files to adjust: ${GO_MOD_FILES//$'\n'/, }."

for f in ${GO_MOD_FILES}; do
	MODULE_PATH="$(dirname "$f")"
	printf "%s\n" "Upgrading \"$DEPENDENCY\" in \"$MODULE_PATH\"."

	set -x
	cd "$MODULE_PATH"
	go get -u "$DEPENDENCY" 
	go mod tidy 
	cd "$ROOT"
	set +x
done
