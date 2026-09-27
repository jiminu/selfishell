#!/usr/bin/env bash
set -euo pipefail
SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
CALLER_DIR="$(pwd -P)"
for name in TMPDIR SELFISHELL_BENCHMARK_RESULTS_FILE SELFISHELL_BENCHMARK_ZPROF_FILE \
  SELFISHELL_BENCHMARK_ROOT SELFISHELL_BENCHMARK_CLI; do
  value="${!name-}"
  if [[ -n "$value" && "$value" != /* ]]; then
    printf -v "$name" '%s/%s' "$CALLER_DIR" "$value"
    export "${name?}"
  fi
done
cd "$SOURCE_ROOT"
export SELFISHELL_BENCHMARK_SOURCE_ROOT="$SOURCE_ROOT"
mkdir -p "$SOURCE_ROOT/.build"
go build -o "$SOURCE_ROOT/.build/selfishell-benchmark" ./cmd/selfishell-benchmark
exec "$SOURCE_ROOT/.build/selfishell-benchmark" "$@"
