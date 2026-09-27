#!/usr/bin/env bash
set -euo pipefail
SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$SOURCE_ROOT"
export SELFISHELL_BENCHMARK_SOURCE_ROOT="$SOURCE_ROOT"
mkdir -p "$SOURCE_ROOT/.build"
go build -o "$SOURCE_ROOT/.build/selfishell-benchmark" ./cmd/selfishell-benchmark
exec "$SOURCE_ROOT/.build/selfishell-benchmark" "$@"
