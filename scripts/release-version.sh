#!/usr/bin/env bash

# Keep pre-toolchain usage validation aligned with ValidReleaseVersion in Go.
selfishell_version_is_valid() {
  local version="${1:-}" prerelease identifier
  local identifiers=()

  [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]] || return 1
  [[ "$version" == *-* ]] || return 0
  prerelease="${version#*-}"
  IFS=. read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && "$identifier" != 0 && "$identifier" == 0* ]]; then
      return 1
    fi
  done
}
