#!/usr/bin/env bash

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "$ROOT_DIR/scripts/go-env.sh"
selfishell_prepare_go "$ROOT_DIR"

temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-dependency-update.XXXXXX")"
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
(cd "$ROOT_DIR" && go build -buildvcs=false -o "$temporary_dir/selfishell-dev" ./cmd/selfishell-dev)
"$temporary_dir/selfishell-dev" "$ROOT_DIR" update-dependencies "$@"
