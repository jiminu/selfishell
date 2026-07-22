#!/usr/bin/env bash

set -euo pipefail

base_sha="${1:-}"
head_sha="${2:-HEAD}"

print_classification() {
  printf 'runtime=%s\n' "$1"
  printf 'ubuntu_container_e2e=%s\n' "$2"
}

dependencies_change_is_nvim_only() {
  local changed_line
  local record
  local record_type
  local saw_record=0

  while IFS= read -r changed_line; do
    case "$changed_line" in
      '+++'* | '---'*) continue ;;
      +* | -*) record="${changed_line#?}" ;;
      *) continue ;;
    esac

    record="${record#"${record%%[![:space:]]*}"}"
    case "$record" in
      '' | \#*) continue ;;
    esac

    saw_record=1
    record_type="${record%%[[:space:]]*}"
    [[ "$record_type" == "nvim-plugin" ]] || return 1
  done < <(git diff --no-renames --unified=0 "$base_sha" "$head_sha" -- dependencies.conf)

  ((saw_record == 1))
}

if [[ -z "$base_sha" || "$base_sha" == 0000000000000000000000000000000000000000 ]] ||
  ! git cat-file -e "$base_sha^{commit}" 2>/dev/null ||
  ! git cat-file -e "$head_sha^{commit}" 2>/dev/null; then
  print_classification true true
  exit 0
fi

runtime=false
ubuntu_container_e2e=false
dependencies_changed=false

while IFS= read -r changed_file; do
  case "$changed_file" in
    AGENTS.md | README.md | docs/* | .agents/*) ;;
    common/nvim/*)
      runtime=true
      ;;
    dependencies.conf)
      runtime=true
      dependencies_changed=true
      ;;
    *)
      runtime=true
      ubuntu_container_e2e=true
      ;;
  esac
done < <(git diff --no-renames --name-only "$base_sha" "$head_sha")

if [[ "$dependencies_changed" == "true" ]] && ! dependencies_change_is_nvim_only; then
  ubuntu_container_e2e=true
fi

print_classification "$runtime" "$ubuntu_container_e2e"
