#!/usr/bin/env bash

set -euo pipefail

repo_dir="."
base=""
head=""
manifest_path=dependencies.conf
pinned_zsh_files=(common/completion.zsh common/interactive.zsh)

usage() {
  printf 'Usage: scripts/verify-dependency-release-diff.sh --base REF --head REF [--repo-dir DIR]\n'
}

git_c() {
  git -C "$repo_dir" "$@"
}

# True only if every difference between $base and $head's copy of $1 sits
# inside a `ver'<40-hex-sha>'` token: masks that token to a fixed
# placeholder on both sides first, so a pin bump normalizes away and any
# other line-content change (a masked comparison can't hide) does not.
file_diff_is_pin_only() {
  local file="$1"
  local old_normalized new_normalized

  old_normalized="$(git_c show "$base:$file" | sed -E "s/ver'[0-9a-f]{40}'/ver'PIN'/g")" || {
    printf '%s could not be read at %s\n' "$file" "$base" >&2
    return 1
  }
  new_normalized="$(git_c show "$head:$file" | sed -E "s/ver'[0-9a-f]{40}'/ver'PIN'/g")" || {
    printf '%s could not be read at %s\n' "$file" "$head" >&2
    return 1
  }

  [[ "$old_normalized" == "$new_normalized" ]]
}

while (($# > 0)); do
  case "$1" in
    --base)
      shift
      base="${1:-}"
      ;;
    --head)
      shift
      head="${1:-}"
      ;;
    --repo-dir)
      shift
      repo_dir="${1:-}"
      ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

[[ -n "$base" && -n "$head" ]] || {
  printf -- '--base and --head are required.\n' >&2
  usage >&2
  exit 2
}

changed_files=()
while IFS= read -r file; do
  [[ -n "$file" ]] && changed_files+=("$file")
done < <(git_c diff --name-only "$base" "$head")

manifest_changed=0
zsh_changed=0

for file in "${changed_files[@]}"; do
  case "$file" in
    "$manifest_path")
      manifest_changed=1
      ;;
    common/completion.zsh | common/interactive.zsh)
      zsh_changed=1
      ;;
    *)
      printf 'Automatic dependency releases only allow changes to %s, %s, and %s.\n' \
        "$manifest_path" "${pinned_zsh_files[0]}" "${pinned_zsh_files[1]}" >&2
      printf 'Unexpected changed file: %s\n' "$file" >&2
      exit 1
      ;;
  esac
done

if ((manifest_changed == 0)); then
  printf '%s must change for an automatic dependency release. Changed files: %s\n' \
    "$manifest_path" "${changed_files[*]:-(none)}" >&2
  exit 1
fi

if ((zsh_changed == 1)); then
  old_zsh_plugin_lines="$(git_c show "$base:$manifest_path" | grep '^zsh-plugin ' || true)"
  new_zsh_plugin_lines="$(git_c show "$head:$manifest_path" | grep '^zsh-plugin ' || true)"
  [[ "$old_zsh_plugin_lines" != "$new_zsh_plugin_lines" ]] || {
    printf 'Zsh plugin pin files changed without a corresponding zsh-plugin record change in %s.\n' \
      "$manifest_path" >&2
    exit 1
  }

  for file in "${pinned_zsh_files[@]}"; do
    printf '%s\n' "${changed_files[@]}" | grep -Fqx "$file" || continue
    file_diff_is_pin_only "$file" || {
      printf 'Unexpected non-pin change detected in %s.\n' "$file" >&2
      exit 1
    }
  done
fi

printf 'Dependency release diff is limited to approved pin changes.\n'
