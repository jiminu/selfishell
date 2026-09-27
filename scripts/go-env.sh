#!/usr/bin/env bash

# Source-build policy only; installed Selfishell never needs a Go toolchain.
selfishell_prepare_go() {
  local source_root="$1" required
  required="$(awk '$1 == "go" { print $2 }' "$source_root/go.mod")" || return 1
  # Set these before the first Go invocation, including the version check.
  # Preserve caller-owned caches and HTTP proxy variables used by development tools.
  export GOTOOLCHAIN=local GO111MODULE=on GOENV=off GOWORK=off GOFLAGS='' GOEXPERIMENT=none
  export GOOS='' GOARCH='' CGO_ENABLED=0 GOAMD64=v1 GOARM64=v8.0 GOPROXY=off GOSUMDB=off
  if ! command -v go >/dev/null 2>&1 || [[ "$(cd "$source_root" && go env GOVERSION 2>/dev/null || :)" != "go$required" ]]; then
    printf 'Building Selfishell developer tools requires Go %s on PATH.\n' "$required" >&2
    return 1
  fi
}
