#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/lib/common.sh"

manifest="$ROOT_DIR/dependencies.conf"
zsh_root="$ROOT_DIR"
metadata=""
temporary_dir=""

cleanup() {
  [[ -z "$temporary_dir" ]] || rm -rf "$temporary_dir"
}

usage() {
  printf 'Usage: scripts/update-dependencies.sh [--manifest FILE] [--metadata FILE] [--zsh-root DIR]\n'
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

github_latest_tag() {
  local repository="$1"
  local arguments=(-H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28')
  [[ -z "${GH_TOKEN:-}" ]] || arguments+=(-H "Authorization: Bearer $GH_TOKEN")
  selfishell_curl metadata "${arguments[@]}" "https://api.github.com/repos/$repository/releases/latest" |
    jq -er '.tag_name'
}

record_download() {
  local name="$1"
  local version="$2"
  local platform="$3"
  local architecture="$4"
  local source="$5"
  local archive="$temporary_dir/$name-$platform-$architecture"
  local checksum

  selfishell_curl transfer "$source" -o "$archive"
  checksum="$(sha256_file "$archive")"
  printf 'download %s %s %s %s %s %s\n' \
    "$name" "$version" "$platform" "$architecture" "$source" "$checksum" >>"$metadata"
}

discover_metadata() {
  local starship_tag starship_version mise_tag mise_version asset_architecture
  local type name source
  local repository tag commit asset

  command -v curl >/dev/null 2>&1 || {
    printf 'curl is required.\n' >&2
    return 1
  }
  command -v jq >/dev/null 2>&1 || {
    printf 'jq is required.\n' >&2
    return 1
  }
  command -v git >/dev/null 2>&1 || {
    printf 'git is required.\n' >&2
    return 1
  }
  name=zinit
  repository=zdharma-continuum/zinit
  tag="$(github_latest_tag "$repository")"
  printf 'git %s %s\n' "$name" "$tag" >>"$metadata"

  while read -r type name _ _ _ source _; do
    case "$type" in nvim-plugin | zsh-plugin) ;; *) continue ;; esac
    commit="$(git ls-remote "$source" HEAD | awk 'NR == 1 { print $1 }')"
    [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
      printf 'Invalid Git plugin commit for %s: %s\n' "$name" "$commit" >&2
      return 1
    }
    printf '%s %s %s\n' "$type" "$name" "$commit" >>"$metadata"
  done <"$manifest"

  starship_tag="$(github_latest_tag starship/starship)"
  starship_version="${starship_tag#v}"
  for platform in linux macos; do
    for architecture in amd64 arm64; do
      case "$platform:$architecture" in
        linux:amd64) asset=starship-x86_64-unknown-linux-gnu.tar.gz ;;
        linux:arm64) asset=starship-aarch64-unknown-linux-musl.tar.gz ;;
        macos:amd64) asset=starship-x86_64-apple-darwin.tar.gz ;;
        macos:arm64) asset=starship-aarch64-apple-darwin.tar.gz ;;
      esac
      source="https://github.com/starship/starship/releases/download/$starship_tag/$asset"
      record_download starship "$starship_version" "$platform" "$architecture" "$source"
    done
  done

  mise_tag="$(github_latest_tag jdx/mise)"
  mise_version="${mise_tag#v}"
  for platform in linux macos; do
    for architecture in amd64 arm64; do
      case "$architecture" in amd64) asset_architecture=x64 ;; arm64) asset_architecture=arm64 ;; esac
      source="https://github.com/jdx/mise/releases/download/$mise_tag/mise-$mise_tag-$platform-$asset_architecture"
      record_download mise "$mise_version" "$platform" "$architecture" "$source"
    done
  done
}

# Builds the updated manifest into $output without touching the real
# manifest file; the caller only commits it once every Zsh plugin pin
# rewrite below has also validated, so a rejected pin bump can never leave
# dependencies.conf and the Zinit pins out of sync.
build_manifest() {
  local output="$1"

  awk '
    NR == FNR {
      if ($1 == "git") {
        git_version[$2] = $3
        expected_git[$2] = 1
      } else if ($1 == "nvim-plugin") {
        nvim_plugin_version[$2] = $3
        expected_nvim_plugin[$2] = 1
      } else if ($1 == "zsh-plugin") {
        zsh_plugin_version[$2] = $3
        expected_zsh_plugin[$2] = 1
      } else if ($1 == "download") {
        key = $2 SUBSEP $4 SUBSEP $5
        download_version[key] = $3
        download_source[key] = $6
        download_checksum[key] = $7
        expected_download[key] = 1
      } else {
        print "Unknown dependency metadata record: " $0 > "/dev/stderr"
        invalid = 1
      }
      next
    }
    /^#/ || NF == 0 { print; next }
    $1 == "git" && ($2 in git_version) {
      $3 = git_version[$2]
      matched_git[$2] = 1
    }
    $1 == "nvim-plugin" && ($2 in nvim_plugin_version) {
      $3 = nvim_plugin_version[$2]
      matched_nvim_plugin[$2] = 1
    }
    $1 == "zsh-plugin" && ($2 in zsh_plugin_version) {
      $3 = zsh_plugin_version[$2]
      matched_zsh_plugin[$2] = 1
    }
    $1 == "download" {
      key = $2 SUBSEP $4 SUBSEP $5
      if (key in download_version) {
        $3 = download_version[key]
        $6 = download_source[key]
        $7 = download_checksum[key]
        matched_download[key] = 1
      }
    }
    { print }
    END {
      for (name in expected_git) {
        if (!(name in matched_git)) {
          print "Dependency metadata did not match manifest git entry: " name > "/dev/stderr"
          invalid = 1
        }
      }
      for (key in expected_download) {
        if (!(key in matched_download)) {
          split(key, fields, SUBSEP)
          print "Dependency metadata did not match manifest download entry: " fields[1] "/" fields[2] "/" fields[3] > "/dev/stderr"
          invalid = 1
        }
      }
      for (name in expected_nvim_plugin) {
        if (!(name in matched_nvim_plugin)) {
          print "Dependency metadata did not match manifest Neovim plugin entry: " name > "/dev/stderr"
          invalid = 1
        }
      }
      for (name in expected_zsh_plugin) {
        if (!(name in matched_zsh_plugin)) {
          print "Dependency metadata did not match manifest Zsh plugin entry: " name > "/dev/stderr"
          invalid = 1
        }
      }
      exit invalid
    }
  ' "$metadata" "$manifest" >"$output"
}

# Maps a pinned Zsh plugin's repository to the file that hardcodes its
# Zinit `ver'<sha>'` pin, so a bump can target that exact string instead of
# a blanket repo-wide substitution. An unrecognized repository is a hard
# failure rather than a silent no-op.
zsh_plugin_pin_file() {
  case "$1" in
    zsh-users/zsh-completions) printf 'common/completion.zsh\n' ;;
    Aloxaf/fzf-tab) printf 'common/interactive.zsh\n' ;;
    zsh-users/zsh-autosuggestions) printf 'common/interactive.zsh\n' ;;
    zdharma-continuum/fast-syntax-highlighting) printf 'common/interactive.zsh\n' ;;
    *) return 1 ;;
  esac
}

# Rewrites one plugin's `ver'<old>'` pin to `ver'<new>'` on a staged copy
# under $staged_dir, chaining onto an earlier plugin's edit to the same
# file instead of re-copying it from disk. Nothing here touches a real
# file: stage_zsh_plugin_pins' caller only commits the staged copies once
# every plugin (and the manifest) has validated.
stage_zsh_plugin_pin() {
  local repository="$1" old_commit="$2" new_commit="$3" staged_dir="$4"
  local target_file staged_file match_count

  target_file="$(zsh_plugin_pin_file "$repository")" || {
    printf 'No pin-file mapping for Zsh plugin: %s\n' "$repository" >&2
    return 1
  }
  [[ "$new_commit" =~ ^[0-9a-f]{40}$ ]] || {
    printf 'New commit for %s is not a 40-character lowercase SHA: %s\n' "$repository" "$new_commit" >&2
    return 1
  }
  [[ "$old_commit" =~ ^[0-9a-f]{40}$ ]] || {
    printf 'Recorded dependencies.conf commit for %s is not a 40-character lowercase SHA: %s\n' "$repository" "$old_commit" >&2
    return 1
  }

  staged_file="$staged_dir/$target_file"
  if [[ ! -f "$staged_file" ]]; then
    [[ -r "$zsh_root/$target_file" ]] || {
      printf 'Zsh plugin pin file not found: %s\n' "$target_file" >&2
      return 1
    }
    mkdir -p "$(dirname "$staged_file")"
    cp "$zsh_root/$target_file" "$staged_file"
  fi

  match_count="$(grep -Fc "ver'$old_commit'" "$staged_file" || true)"
  [[ "$match_count" -eq 1 ]] || {
    printf "Expected exactly one ver'%s' pin for %s in %s, found %s\n" \
      "$old_commit" "$repository" "$target_file" "$match_count" >&2
    return 1
  }

  awk -v old="ver'$old_commit'" -v new="ver'$new_commit'" '{ gsub(old, new); print }' \
    "$staged_file" >"$staged_file.next"
  mv "$staged_file.next" "$staged_file"
}

# Stages every `zsh-plugin` bump in $metadata against the pre-update
# commit recorded in $manifest (build_manifest hasn't overwritten it yet,
# since it only writes to a temp file). Returns non-zero without staging
# anything further as soon as one plugin fails to validate.
stage_zsh_plugin_pins() {
  local staged_dir="$1"
  local type name commit old_commit

  while read -r type name commit _; do
    [[ "$type" == zsh-plugin ]] || continue
    old_commit="$(awk -v name="$name" '
      $1 == "zsh-plugin" && $2 == name { print $3; found = 1 }
      END { exit !found }
    ' "$manifest")" || {
      printf 'No dependencies.conf zsh-plugin entry for %s\n' "$name" >&2
      return 1
    }
    stage_zsh_plugin_pin "$name" "$old_commit" "$commit" "$staged_dir" || return 1
  done <"$metadata"
}

while (($# > 0)); do
  case "$1" in
    --manifest)
      shift
      manifest="${1:-}"
      ;;
    --metadata)
      shift
      metadata="${1:-}"
      ;;
    --zsh-root)
      shift
      zsh_root="${1:-}"
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

[[ -r "$manifest" ]] || {
  printf 'Dependency manifest not found: %s\n' "$manifest" >&2
  exit 1
}
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-dependency-update.XXXXXX")"
trap cleanup EXIT HUP INT TERM
if [[ -z "$metadata" ]]; then
  metadata="$temporary_dir/metadata"
  : >"$metadata"
  discover_metadata
else
  [[ -r "$metadata" ]] || {
    printf 'Dependency metadata not found: %s\n' "$metadata" >&2
    exit 1
  }
fi

manifest_output="$temporary_dir/dependencies.conf"
staged_dir="$temporary_dir/staged"
mkdir -p "$staged_dir"

build_manifest "$manifest_output"
stage_zsh_plugin_pins "$staged_dir"

# Nothing above touched a real file; only now, with the manifest and every
# Zsh plugin pin rewrite validated, are they committed together.
mv "$manifest_output" "$manifest"
for target_file in common/completion.zsh common/interactive.zsh; do
  staged_file="$staged_dir/$target_file"
  [[ -f "$staged_file" ]] || continue
  mv "$staged_file" "$zsh_root/$target_file"
done
