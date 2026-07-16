#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$ROOT_DIR/dependencies.conf"
metadata=""
temporary_dir=""

cleanup() {
  [[ -z "$temporary_dir" ]] || rm -rf "$temporary_dir"
}

usage() {
  printf 'Usage: scripts/update-dependencies.sh [--manifest FILE] [--metadata FILE]\n'
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
  local arguments=(-fsSL -H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28')
  [[ -z "${GH_TOKEN:-}" ]] || arguments+=(-H "Authorization: Bearer $GH_TOKEN")
  curl "${arguments[@]}" "https://api.github.com/repos/$repository/releases/latest" | jq -er '.tag_name'
}

record_download() {
  local name="$1"
  local version="$2"
  local platform="$3"
  local architecture="$4"
  local source="$5"
  local archive="$temporary_dir/$name-$platform-$architecture"
  local checksum

  curl -fsSL "$source" -o "$archive"
  checksum="$(sha256_file "$archive")"
  printf 'download %s %s %s %s %s %s\n' \
    "$name" "$version" "$platform" "$architecture" "$source" "$checksum" >>"$metadata"
}

discover_metadata() {
  local starship_tag starship_version kubectl_tag kubectl_version
  local name repository tag commit platform architecture asset source checksum

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

  for name in nvm pyenv pyenv-virtualenv zinit; do
    case "$name" in
      nvm) repository=nvm-sh/nvm ;;
      pyenv) repository=pyenv/pyenv ;;
      pyenv-virtualenv) repository=pyenv/pyenv-virtualenv ;;
      zinit) repository=zdharma-continuum/zinit ;;
    esac
    tag="$(github_latest_tag "$repository")"
    printf 'git %s %s\n' "$name" "$tag" >>"$metadata"
  done

  commit="$(git ls-remote https://github.com/VundleVim/Vundle.vim.git HEAD | awk 'NR == 1 { print $1 }')"
  [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
    printf 'Invalid Vundle commit: %s\n' "$commit" >&2
    return 1
  }
  printf 'git vundle %s\n' "$commit" >>"$metadata"

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

  kubectl_tag="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
  kubectl_version="${kubectl_tag#v}"
  for architecture in amd64 arm64; do
    source="https://dl.k8s.io/release/$kubectl_tag/bin/linux/$architecture/kubectl"
    checksum="$(curl -fsSL "$source.sha256")"
    [[ "$checksum" =~ ^[0-9a-fA-F]{64}$ ]] || {
      printf 'Invalid kubectl checksum for %s.\n' "$architecture" >&2
      return 1
    }
    printf 'download kubectl %s linux %s %s %s\n' \
      "$kubectl_version" "$architecture" "$source" "$checksum" >>"$metadata"
  done
}

apply_metadata() {
  local output="$temporary_dir/dependencies.conf"

  awk '
    NR == FNR {
      if ($1 == "git") {
        git_version[$2] = $3
        expected_git[$2] = 1
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
      exit invalid
    }
  ' "$metadata" "$manifest" >"$output"
  mv "$output" "$manifest"
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
apply_metadata
