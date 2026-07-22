#!/usr/bin/env bash

set -euo pipefail

SELFISHELL_RELEASE_ROOT="${SELFISHELL_RELEASE_ROOT:-https://github.com/jiminu/selfishell/releases}"
SELFISHELL_TEMP_DIR=""
SELFISHELL_STAGING_DIR=""

bootstrap_error() {
  printf 'selfishell installer: %s\n' "$*" >&2
}

bootstrap_cleanup() {
  if [[ -n "$SELFISHELL_TEMP_DIR" && -d "$SELFISHELL_TEMP_DIR" ]]; then
    rm -rf "$SELFISHELL_TEMP_DIR"
  fi
  if [[ -n "$SELFISHELL_STAGING_DIR" && -d "$SELFISHELL_STAGING_DIR" ]]; then
    rm -rf "$SELFISHELL_STAGING_DIR"
  fi
}

bootstrap_help() {
  cat <<'EOF'
Usage:
  install.sh [--version VERSION] [--prefix PATH] [--setup] [--yes]
             [--profile NAME] [--skip-packages] [--add-to-path]

Options:
  --version VERSION  Install an exact Selfishell release
  --prefix PATH      Installation prefix (default: $HOME/.local)
  --setup            Run 'selfishell install' after installing the CLI
  --yes              Skip setup confirmation when used with --setup
  --profile NAME     Profile passed to setup (default: developer)
  --skip-packages    Pass configuration-only mode to setup
  --add-to-path      Persist the CLI directory in Bash or Zsh PATH
  --help             Show this help
EOF
}

bootstrap_platform() {
  local system_name="${SELFISHELL_BOOTSTRAP_OS:-$(uname -s)}"

  case "$system_name" in
    Darwin) printf 'macos\n' ;;
    Linux) printf 'linux\n' ;;
    *)
      bootstrap_error "Unsupported operating system: $system_name"
      return 1
      ;;
  esac
}

bootstrap_architecture() {
  local machine_arch="${SELFISHELL_BOOTSTRAP_ARCH:-$(uname -m)}"

  case "$machine_arch" in
    arm64 | aarch64) printf 'arm64\n' ;;
    x86_64 | amd64) printf 'amd64\n' ;;
    *)
      bootstrap_error "Unsupported architecture: $machine_arch"
      return 1
      ;;
  esac
}

bootstrap_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    bootstrap_error "A SHA-256 tool is required (sha256sum or shasum)."
    return 1
  fi
}

bootstrap_version_is_valid() {
  local version="${1:-}"
  local prerelease identifier
  local identifiers=()

  [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]] ||
    return 1
  [[ "$version" == *-* ]] || return 0

  prerelease="${version#*-}"
  IFS=. read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && "$identifier" != 0 && "$identifier" == 0* ]]; then
      return 1
    fi
  done
}

bootstrap_validate_version() {
  bootstrap_version_is_valid "$1" || {
    bootstrap_error "Invalid semantic version: $1"
    return 1
  }
}

bootstrap_curl() {
  local mode="$1"
  local connect_timeout="${SELFISHELL_CURL_CONNECT_TIMEOUT:-10}"
  local low_speed_limit="${SELFISHELL_CURL_LOW_SPEED_LIMIT:-1024}"
  local low_speed_time="${SELFISHELL_CURL_LOW_SPEED_TIME:-30}"
  local metadata_max_time="${SELFISHELL_CURL_METADATA_MAX_TIME:-15}"
  local value
  local arguments=()
  shift

  for value in "$connect_timeout" "$low_speed_limit" "$low_speed_time" "$metadata_max_time"; do
    case "$value" in
      "" | *[!0-9]* | 0)
        bootstrap_error "Curl timeout and speed settings must be positive integers."
        return 2
        ;;
    esac
  done

  arguments=(
    --connect-timeout "$connect_timeout"
    --speed-limit "$low_speed_limit"
    --speed-time "$low_speed_time"
  )
  case "$mode" in
    metadata) arguments+=(--max-time "$metadata_max_time") ;;
    transfer) ;;
    *)
      bootstrap_error "Unknown curl mode: $mode"
      return 2
      ;;
  esac

  curl -fsSL "${arguments[@]}" "$@"
}

bootstrap_latest_version() {
  local official_root="https://github.com/jiminu/selfishell/releases"
  local api_url response version published_version

  if version="$(bootstrap_curl metadata "$SELFISHELL_RELEASE_ROOT/latest/download/VERSION" 2>/dev/null)"; then
    version="${version#v}"
    [[ -n "$version" ]] && {
      printf '%s\n' "$version"
      return
    }
  fi

  [[ "$SELFISHELL_RELEASE_ROOT" == "$official_root" || -n "${SELFISHELL_RELEASE_TAGS_API_URL:-${SELFISHELL_RELEASE_API_URL:-}}" ]] || return 1
  api_url="${SELFISHELL_RELEASE_TAGS_API_URL:-${SELFISHELL_RELEASE_API_URL:-https://api.github.com/repos/jiminu/selfishell/tags?per_page=1}}"
  response="$(bootstrap_curl metadata \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "$api_url" 2>/dev/null)" || return 1
  version="$(printf '%s\n' "$response" | sed -n \
    -e 's/.*"name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' \
    -e 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' | sed -n '1p')"
  [[ -n "$version" ]] || return 1
  published_version="$(bootstrap_curl metadata "$SELFISHELL_RELEASE_ROOT/download/v${version}/VERSION" 2>/dev/null)" || return 1
  published_version="${published_version#v}"
  [[ "$published_version" == "$version" ]] || return 1
  printf '%s\n' "$version"
}

bootstrap_atomic_link() {
  local link_target="$1"
  local link_path="$2"
  local temporary_link="${link_path}.tmp.$$"
  local suffix=0

  if [[ -e "$link_path" && ! -L "$link_path" ]]; then
    bootstrap_error "Refusing to replace non-link path: $link_path"
    return 1
  fi

  while [[ -e "$temporary_link" || -L "$temporary_link" ]]; do
    suffix=$((suffix + 1))
    temporary_link="${link_path}.tmp.$$.${suffix}"
  done

  ln -s "$link_target" "$temporary_link"
  if mv -fT "$temporary_link" "$link_path" 2>/dev/null; then
    return
  fi
  mv -fh "$temporary_link" "$link_path"
}

bootstrap_validate_link_path() {
  if [[ -e "$1" && ! -L "$1" ]]; then
    bootstrap_error "Refusing to replace non-link path: $1"
    return 1
  fi
}

bootstrap_prune_releases() {
  local releases_dir="$1"
  local current_target="$2"
  local previous_target="$3"
  local release_dir release_name

  current_target="${current_target##*/}"
  previous_target="${previous_target##*/}"
  for release_dir in "$releases_dir"/*; do
    [[ -d "$release_dir" && ! -L "$release_dir" ]] || continue
    release_name="${release_dir##*/}"
    [[ "$release_name" == "$current_target" || "$release_name" == "$previous_target" ]] && continue
    rm -rf "$release_dir"
    printf 'Removed inactive Selfishell release: %s\n' "$release_name"
  done
}

bootstrap_path_block_is_intact() {
  local startup_file="$1"
  local marker="$2"
  local entry="$3"

  awk -v marker="$marker" -v entry="$entry" '
    {
      if ($0 == marker) marker_count++
      if ($0 == entry) {
        entry_count++
        if (previous == marker) intact_count++
      }
      previous = $0
    }
    END { exit(marker_count == 1 && entry_count == 1 && intact_count == 1 ? 0 : 1) }
  ' "$startup_file"
}

bootstrap_validate_path_state_file() {
  local state_file="$1"
  local expected="$2"

  if [[ -L "$state_file" || ! -f "$state_file" || ! -r "$state_file" ]]; then
    bootstrap_error "Invalid Selfishell PATH state file: $state_file"
    return 1
  fi
  if [[ "$(<"$state_file")" != "$expected" ]]; then
    bootstrap_error "Selfishell PATH state does not match this installation: $state_file"
    return 1
  fi
}

bootstrap_record_path_state() {
  local startup_file="$1"
  local bin_dir="$2"
  local state_file="$3"
  local bin_state_file="$4"
  local temporary_state=""
  local temporary_bin_state=""
  local bin_state_existed=0

  [[ ! -e "$bin_state_file" && ! -L "$bin_state_file" ]] || bin_state_existed=1
  if [[ "$bin_state_existed" == 0 ]]; then
    temporary_bin_state="$(mktemp "${bin_state_file}.tmp.XXXXXX")" || return 1
    printf '%s\n' "$bin_dir" >"$temporary_bin_state" || {
      rm -f "$temporary_bin_state"
      return 1
    }
    mv "$temporary_bin_state" "$bin_state_file" || {
      rm -f "$temporary_bin_state"
      return 1
    }
  fi

  if [[ ! -e "$state_file" && ! -L "$state_file" ]]; then
    temporary_state="$(mktemp "${state_file}.tmp.XXXXXX")" || {
      [[ "$bin_state_existed" == 1 ]] || rm -f "$bin_state_file"
      return 1
    }
    printf '%s\n' "$startup_file" >"$temporary_state" || {
      rm -f "$temporary_state"
      [[ "$bin_state_existed" == 1 ]] || rm -f "$bin_state_file"
      return 1
    }
    mv "$temporary_state" "$state_file" || {
      rm -f "$temporary_state"
      [[ "$bin_state_existed" == 1 ]] || rm -f "$bin_state_file"
      return 1
    }
  fi
}

bootstrap_add_to_path() {
  local bin_dir="$1"
  local share_dir="$2"
  local shell_name="${SELFISHELL_BOOTSTRAP_SHELL:-${SHELL:-bash}}"
  local startup_file
  local escaped_bin_dir
  local marker='# Added by Selfishell installer'
  local path_entry
  local state_file="$share_dir/path-startup-file"
  local bin_state_file="$share_dir/path-bin-dir"
  local temporary_startup=""
  local state_file_existed=0
  local bin_state_file_existed=0

  case "${shell_name##*/}" in
    zsh) startup_file="$HOME/.zshrc" ;;
    *) startup_file="$HOME/.bashrc" ;;
  esac

  printf -v escaped_bin_dir '%q' "$bin_dir"
  path_entry="export PATH=${escaped_bin_dir}:\"\$PATH\""

  if [[ -L "$startup_file" || (-e "$startup_file" && ! -f "$startup_file") ]]; then
    bootstrap_error "Refusing to modify non-regular shell startup file: $startup_file"
    return 1
  fi
  if [[ -f "$startup_file" && (! -r "$startup_file" || ! -w "$startup_file") ]]; then
    bootstrap_error "Shell startup file must be readable and writable: $startup_file"
    return 1
  fi

  if [[ -e "$state_file" || -L "$state_file" ]]; then
    state_file_existed=1
    bootstrap_validate_path_state_file "$state_file" "$startup_file" || return
  fi
  if [[ -e "$bin_state_file" || -L "$bin_state_file" ]]; then
    bin_state_file_existed=1
    bootstrap_validate_path_state_file "$bin_state_file" "$bin_dir" || return
  fi

  if [[ -f "$startup_file" ]] && bootstrap_path_block_is_intact "$startup_file" "$marker" "$path_entry"; then
    bootstrap_record_path_state "$startup_file" "$bin_dir" "$state_file" "$bin_state_file" || {
      bootstrap_error "Failed to record Selfishell PATH state."
      return 1
    }
    printf 'PATH already configured in %s\n' "$startup_file"
    return
  fi

  if [[ "$state_file_existed" == 0 && "$bin_state_file_existed" == 0 && -f "$startup_file" ]] &&
    grep -Fqx "$path_entry" "$startup_file" && ! grep -Fqx "$marker" "$startup_file"; then
    printf 'PATH already configured in %s\n' "$startup_file"
    return
  fi

  if [[ -f "$startup_file" ]] &&
    { grep -Fqx "$marker" "$startup_file" || grep -Fqx "$path_entry" "$startup_file"; }; then
    bootstrap_error "Existing Selfishell PATH block is incomplete or duplicated; preserving: $startup_file"
    return 1
  fi

  temporary_startup="$(mktemp "${startup_file}.tmp.XXXXXX")" || return 1
  if [[ -f "$startup_file" ]]; then
    cp -p "$startup_file" "$temporary_startup" || {
      rm -f "$temporary_startup"
      return 1
    }
  fi
  {
    printf '\n# Added by Selfishell installer\n'
    printf '%s\n' "$path_entry"
  } >>"$temporary_startup" || {
    rm -f "$temporary_startup"
    return 1
  }

  bootstrap_record_path_state "$startup_file" "$bin_dir" "$state_file" "$bin_state_file" || {
    rm -f "$temporary_startup"
    bootstrap_error "Failed to record Selfishell PATH state."
    return 1
  }
  if ! mv "$temporary_startup" "$startup_file"; then
    rm -f "$temporary_startup"
    [[ "$state_file_existed" == 1 ]] || rm -f "$state_file"
    [[ "$bin_state_file_existed" == 1 ]] || rm -f "$bin_state_file"
    bootstrap_error "Failed to update shell startup file: $startup_file"
    return 1
  fi
  printf 'Added %s to PATH in %s\n' "$bin_dir" "$startup_file"
}

bootstrap_print_path_guidance() {
  local bin_dir="$1"

  printf '\nSelfishell is installed, but %s is not in PATH.\n' "$bin_dir"
  printf 'Run this in the current shell:\n'
  # Print a command for the user; do not expand the installer's PATH here.
  # shellcheck disable=SC2016
  printf '  export PATH="%s:$PATH"\n' "$bin_dir"
  printf 'To configure future Bash or Zsh sessions automatically, reinstall with --add-to-path.\n'
  printf 'Or continue without changing PATH:\n'
  printf '  %s/selfishell install\n' "$bin_dir"
}

main() {
  local version=""
  local prefix="${HOME}/.local"
  local setup=0
  local assume_yes=0
  local profile=developer
  local skip_packages=0
  local add_to_path=0
  local platform
  local architecture
  local release_url
  local archive_name
  local archive_file
  local checksum_file
  local expected_checksum
  local actual_checksum
  local share_dir
  local releases_dir
  local release_dir
  local staging_dir
  local bin_dir
  local setup_args=()
  local current_target=""
  local current_version=""
  local previous_target=""

  while (("$#" > 0)); do
    case "$1" in
      --version)
        shift
        (("$#" > 0)) || {
          bootstrap_error "--version requires a value"
          return 2
        }
        version="${1#v}"
        ;;
      --prefix)
        shift
        (("$#" > 0)) || {
          bootstrap_error "--prefix requires a path"
          return 2
        }
        prefix="$1"
        ;;
      --setup) setup=1 ;;
      --yes) assume_yes=1 ;;
      --profile)
        shift
        (("$#" > 0)) || {
          bootstrap_error "--profile requires a value"
          return 2
        }
        profile="$1"
        ;;
      --skip-packages) skip_packages=1 ;;
      --add-to-path) add_to_path=1 ;;
      help | --help | -h)
        bootstrap_help
        return
        ;;
      *)
        bootstrap_error "Unknown option: $1"
        return 2
        ;;
    esac
    shift
  done

  [[ "$prefix" == /* ]] || {
    bootstrap_error "--prefix must be an absolute path: $prefix"
    return 2
  }

  platform="$(bootstrap_platform)"
  architecture="$(bootstrap_architecture)"

  if [[ -z "$version" ]]; then
    version="$(bootstrap_latest_version)" || {
      bootstrap_error "Unable to determine the latest Selfishell release. Use --version VERSION to select one."
      return 1
    }
  fi
  bootstrap_validate_version "$version"

  release_url="$SELFISHELL_RELEASE_ROOT/download/v${version}"
  archive_name="selfishell-${version}-${platform}-${architecture}.tar.gz"
  SELFISHELL_TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-install.XXXXXX")"
  trap bootstrap_cleanup EXIT HUP INT TERM
  archive_file="$SELFISHELL_TEMP_DIR/$archive_name"
  checksum_file="$SELFISHELL_TEMP_DIR/SHA256SUMS"

  printf 'Downloading Selfishell %s for %s/%s\n' "$version" "$platform" "$architecture"
  bootstrap_curl transfer "$release_url/$archive_name" -o "$archive_file"
  bootstrap_curl transfer "$release_url/SHA256SUMS" -o "$checksum_file"

  expected_checksum="$(awk -v archive="$archive_name" '$2 == archive { print $1 }' "$checksum_file")"
  if [[ -z "$expected_checksum" || "$expected_checksum" == *[!0-9a-fA-F]* ]]; then
    bootstrap_error "No valid checksum found for $archive_name"
    return 1
  fi
  actual_checksum="$(bootstrap_sha256 "$archive_file")"
  if [[ "$actual_checksum" != "$expected_checksum" ]]; then
    bootstrap_error "Checksum mismatch for $archive_name"
    return 1
  fi

  share_dir="$prefix/share/selfishell"
  releases_dir="$share_dir/releases"
  release_dir="$releases_dir/$version"
  bin_dir="$prefix/bin"
  bootstrap_validate_link_path "$share_dir/current"
  bootstrap_validate_link_path "$share_dir/previous"
  bootstrap_validate_link_path "$bin_dir/selfishell"
  bootstrap_validate_link_path "$bin_dir/sfs"
  mkdir -p "$releases_dir" "$bin_dir"

  if [[ -e "$release_dir" && ! -d "$release_dir" ]]; then
    bootstrap_error "Release path is not a directory: $release_dir"
    return 1
  fi

  if [[ -d "$release_dir" ]]; then
    if [[ ! -r "$release_dir/VERSION" || "$(<"$release_dir/VERSION")" != "$version" || ! -x "$release_dir/bin/selfishell" ]]; then
      bootstrap_error "Existing release is incomplete: $release_dir"
      return 1
    fi
    printf 'Release already installed: %s\n' "$release_dir"
  else
    staging_dir="$(mktemp -d "$releases_dir/.${version}.tmp.XXXXXX")"
    SELFISHELL_STAGING_DIR="$staging_dir"
    tar -xzf "$archive_file" -C "$staging_dir"
    if [[ ! -r "$staging_dir/VERSION" || "$(<"$staging_dir/VERSION")" != "$version" || ! -x "$staging_dir/bin/selfishell" ]]; then
      bootstrap_error "Release archive is invalid or has the wrong version."
      return 1
    fi
    mv "$staging_dir" "$release_dir"
    SELFISHELL_STAGING_DIR=""
  fi

  [[ ! -L "$share_dir/current" ]] || current_target="$(readlink "$share_dir/current")"
  [[ ! -L "$share_dir/previous" ]] || previous_target="$(readlink "$share_dir/previous")"
  current_version="${current_target##*/}"
  if [[ -n "$current_target" && "$current_version" != "$version" ]]; then
    bootstrap_atomic_link "$current_target" "$share_dir/previous"
    previous_target="$current_target"
  fi
  bootstrap_atomic_link "releases/$version" "$share_dir/current"
  bootstrap_atomic_link "$share_dir/current/bin/selfishell" "$bin_dir/selfishell"
  bootstrap_atomic_link selfishell "$bin_dir/sfs"
  bootstrap_prune_releases "$releases_dir" "releases/$version" "$previous_target"

  printf 'Installed Selfishell %s\n' "$version"
  if [[ "$add_to_path" == 1 ]]; then
    bootstrap_add_to_path "$bin_dir" "$share_dir"
  elif [[ ":$PATH:" != *":$bin_dir:"* ]]; then
    bootstrap_print_path_guidance "$bin_dir"
  fi

  if [[ "$setup" == "1" ]]; then
    setup_args=(install --profile "$profile")
    [[ "$skip_packages" == "1" ]] && setup_args+=(--skip-packages)
    [[ "$assume_yes" == "1" ]] && setup_args+=(--yes)
    "$bin_dir/selfishell" "${setup_args[@]}"
  fi
}

main "$@"
