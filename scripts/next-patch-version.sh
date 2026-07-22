#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/lib/common.sh"

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
    candidate="${tag#v}"
    if [[ "$candidate" != *-* ]] && selfishell_version_is_valid "$candidate"; then
      current="$candidate"
      break
    fi
  done < <(git tag --list 'v*.*.*' --sort=-v:refname)
fi

if [[ "$current" == *-* ]] || ! selfishell_version_is_valid "$current"; then
  printf 'No stable semantic version is available for a patch release.\n' >&2
  exit 1
fi

IFS=. read -r major minor patch <<<"$current"
printf '%d.%d.%d\n' "$major" "$minor" "$((patch + 1))"
