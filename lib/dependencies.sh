#!/usr/bin/env bash

dependencies_manifest_path() {
  printf '%s\n' "${SELFISHELL_DEPENDENCIES_FILE:-$SELFISHELL_ROOT/dependencies.conf}"
}

dependency_load() {
  local requested="$1"
  local platform="$2"
  local architecture="$3"
  local type name version entry_platform entry_architecture source checksum target marker
  local manifest

  manifest="$(dependencies_manifest_path)"
  [[ -r "$manifest" ]] || {
    cli_error "Dependency manifest not found: $manifest"
    return 1
  }

  while read -r type name version entry_platform entry_architecture source checksum target marker; do
    [[ -n "$type" && "${type#\#}" == "$type" ]] || continue
    [[ "$name" == "$requested" ]] || continue
    [[ "$entry_platform" == all || "$entry_platform" == "$platform" ]] || continue
    [[ "$entry_architecture" == all || "$entry_architecture" == "$architecture" ]] || continue
    DEPENDENCY_TYPE="$type"
    DEPENDENCY_NAME="$name"
    DEPENDENCY_VERSION="$version"
    DEPENDENCY_SOURCE="$source"
    DEPENDENCY_CHECKSUM="$checksum"
    DEPENDENCY_TARGET="$HOME/$target"
    DEPENDENCY_MARKER="$marker"
    return 0
  done <"$manifest"

  cli_error "No approved dependency entry for $requested ($platform/$architecture)."
  return 1
}

dependency_sha256() {
  if have_command sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  elif have_command shasum; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    cli_error "A SHA-256 tool is required (sha256sum or shasum)."
    return 1
  fi
}

dependency_installed_version() {
  local name="$1"
  local state_file="$SELFISHELL_STATE_DIR/dependencies/$name"
  if [[ -r "$state_file" ]]; then
    printf '%s\n' "$(<"$state_file")"
  fi
  return 0
}

dependency_write_version() {
  local name="$1"
  local version="$2"
  local state_dir="$SELFISHELL_STATE_DIR/dependencies"
  local temporary_file

  mkdir -p "$state_dir" || return 1
  temporary_file="$(mktemp "$state_dir/${name}.tmp.XXXXXX")" || return 1
  printf '%s\n' "$version" >"$temporary_file" || {
    rm -f "$temporary_file"
    return 1
  }
  mv "$temporary_file" "$state_dir/$name" || {
    rm -f "$temporary_file"
    return 1
  }
}

dependency_install_download() {
  local temporary_dir archive extracted
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-dependency.XXXXXX")"
  archive="$temporary_dir/archive"
  selfishell_curl transfer "$DEPENDENCY_SOURCE" -o "$archive" || {
    rm -rf "$temporary_dir"
    return 1
  }
  if [[ "$(dependency_sha256 "$archive")" != "$DEPENDENCY_CHECKSUM" ]]; then
    cli_error "Checksum mismatch for $DEPENDENCY_NAME $DEPENDENCY_VERSION."
    rm -rf "$temporary_dir"
    return 1
  fi

  mkdir -p "$(dirname "$DEPENDENCY_TARGET")" || {
    rm -rf "$temporary_dir"
    return 1
  }
  if [[ "$DEPENDENCY_MARKER" == raw ]]; then
    extracted="$archive"
  else
    tar -xzf "$archive" -C "$temporary_dir" || {
      rm -rf "$temporary_dir"
      return 1
    }
    extracted="$temporary_dir/$DEPENDENCY_MARKER"
  fi
  [[ -f "$extracted" ]] || {
    cli_error "Expected executable missing from $DEPENDENCY_NAME archive."
    rm -rf "$temporary_dir"
    return 1
  }
  chmod 0755 "$extracted" || {
    rm -rf "$temporary_dir"
    return 1
  }
  # Guarded explicitly: dependency_install_download runs with errexit
  # disabled (it's called as `dependency_install_download || return`), so an
  # unguarded mv failure here would fall through to `rm -rf`, silently
  # deleting the freshly extracted binary and reporting success.
  if ! mv "$extracted" "$DEPENDENCY_TARGET"; then
    rm -rf "$temporary_dir"
    return 1
  fi
  rm -rf "$temporary_dir"
}

dependency_install_git() {
  local temporary_target="${DEPENDENCY_TARGET}.tmp.$$"
  local previous_target="${DEPENDENCY_TARGET}.previous.$$"
  [[ ! -e "$temporary_target" ]] || {
    cli_error "Temporary dependency path already exists: $temporary_target"
    return 1
  }
  git clone --quiet "$DEPENDENCY_SOURCE" "$temporary_target" || {
    rm -rf "$temporary_target"
    return 1
  }
  git -C "$temporary_target" checkout --quiet --detach "$DEPENDENCY_VERSION" || {
    rm -rf "$temporary_target"
    return 1
  }
  [[ -e "$temporary_target/$DEPENDENCY_MARKER" ]] || {
    cli_error "Expected marker missing from $DEPENDENCY_NAME checkout."
    rm -rf "$temporary_target"
    return 1
  }

  # Guarded explicitly: dependency_install_git runs with errexit disabled
  # (it's called as `dependency_install_git || return`), so an unguarded mv
  # failure here would previously fall through and delete the working
  # previous install via the final `rm -rf`. Restore it instead of losing it.
  if [[ -e "$DEPENDENCY_TARGET" ]]; then
    mv "$DEPENDENCY_TARGET" "$previous_target" || {
      rm -rf "$temporary_target"
      return 1
    }
  fi
  if ! mkdir -p "$(dirname "$DEPENDENCY_TARGET")"; then
    [[ ! -e "$previous_target" ]] || mv "$previous_target" "$DEPENDENCY_TARGET"
    rm -rf "$temporary_target"
    return 1
  fi
  if ! mv "$temporary_target" "$DEPENDENCY_TARGET"; then
    [[ ! -e "$previous_target" ]] || mv "$previous_target" "$DEPENDENCY_TARGET"
    rm -rf "$temporary_target"
    return 1
  fi
  rm -rf "$previous_target"
}

dependency_install() {
  local name="$1"
  local platform="$2"
  local architecture="$3"
  local force="${4:-0}"
  local installed

  dependency_load "$name" "$platform" "$architecture" || return
  selfishell_initialize_paths
  installed="$(dependency_installed_version "$name")"
  if [[ "$force" == 0 && "$installed" == "$DEPENDENCY_VERSION" && -e "$DEPENDENCY_TARGET" ]]; then
    printf '%sAlready approved:%s %s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$name" "$DEPENDENCY_VERSION"
    return
  fi
  if [[ -z "$installed" && -e "$DEPENDENCY_TARGET" ]]; then
    printf '%sExternally installed; preserving:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$DEPENDENCY_TARGET"
    return
  fi

  case "$DEPENDENCY_TYPE" in
    download) dependency_install_download || return ;;
    git) dependency_install_git || return ;;
    *)
      cli_error "Unknown dependency type: $DEPENDENCY_TYPE"
      return 1
      ;;
  esac
  dependency_write_version "$name" "$DEPENDENCY_VERSION" || return 1
  printf '%sInstalled approved dependency:%s %s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$name" "$DEPENDENCY_VERSION"
}
