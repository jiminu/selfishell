#!/usr/bin/env bash

set -euo pipefail

current=""

if [[ "${1:-}" == --current ]]; then
  current="${2:-}"
  shift 2
fi
if (($# > 0)); then
  printf 'Usage: scripts/next-patch-version.sh [--current VERSION]\n' >&2
  exit 2
fi

if [[ -z "$current" ]]; then
  while IFS= read -r tag; do
    if [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      current="${tag#v}"
      break
    fi
  done < <(git tag --list 'v*.*.*' --sort=-v:refname)
fi

[[ "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  printf 'No stable semantic version is available for a patch release.\n' >&2
  exit 1
}

IFS=. read -r major minor patch <<<"$current"
printf '%d.%d.%d\n' "$major" "$minor" "$((patch + 1))"
