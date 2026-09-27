#!/usr/bin/env bash
set -euo pipefail
SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
CALLER_DIR="$(pwd -P)"
for name in TMPDIR SELFISHELL_BENCHMARK_RESULTS_FILE SELFISHELL_BENCHMARK_ZPROF_FILE; do
  value="${!name-}"
  if [[ -n "$value" && "$value" != /* ]]; then
    printf -v "$name" '%s/%s' "$CALLER_DIR" "$value"
    export "${name?}"
  fi
done
cd "$SOURCE_ROOT"
source "$SOURCE_ROOT/scripts/go-env.sh"
selfishell_prepare_go "$SOURCE_ROOT"
mkdir -p "$SOURCE_ROOT/.build"
go build -buildvcs=false -o "$SOURCE_ROOT/.build/selfishell-benchmark" ./cmd/selfishell-benchmark
exec "$SOURCE_ROOT/.build/selfishell-benchmark" "$@"
