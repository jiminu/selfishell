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
  local temporary_dir archive extracted previous_target
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-dependency.XXXXXX")" || return 1
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
  # A pre-existing target (e.g. replaced by a directory, or a stale/tampered
  # install) would otherwise make `mv` nest $extracted inside it instead of
  # replacing it -- silently leaving the approved binary unreachable while
  # still reporting success. Move it aside first, mirroring
  # dependency_install_git's existing-target handling, so mv only ever
  # renames onto an absent path and a failed activation can be restored.
  if [[ -e "$DEPENDENCY_TARGET" || -L "$DEPENDENCY_TARGET" ]]; then
    previous_target="$(selfishell_unique_path "${DEPENDENCY_TARGET}.previous.$$")"
    mv "$DEPENDENCY_TARGET" "$previous_target" || {
      rm -rf "$temporary_dir"
      return 1
    }
  fi
  # Guarded explicitly: dependency_install_download runs with errexit
  # disabled (it's called as `dependency_install_download || return`), so an
  # unguarded mv failure here would fall through to `rm -rf`, silently
  # deleting the freshly extracted binary and reporting success.
  if ! mv "$extracted" "$DEPENDENCY_TARGET"; then
    # previous_target may itself be a dangling symlink (moved aside above),
    # which -e alone would miss and skip restoring.
    [[ -z "${previous_target:-}" || (! -e "$previous_target" && ! -L "$previous_target") ]] ||
      mv "$previous_target" "$DEPENDENCY_TARGET"
    rm -rf "$temporary_dir"
    return 1
  fi
  [[ -z "${previous_target:-}" ]] || rm -rf "$previous_target"
  rm -rf "$temporary_dir"
}

dependency_install_git() {
  local temporary_target previous_target

  temporary_target="$(selfishell_unique_path "${DEPENDENCY_TARGET}.tmp.$$")"
  previous_target="$(selfishell_unique_path "${DEPENDENCY_TARGET}.previous.$$")"
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
  if [[ -e "$DEPENDENCY_TARGET" || -L "$DEPENDENCY_TARGET" ]]; then
    mv "$DEPENDENCY_TARGET" "$previous_target" || {
      rm -rf "$temporary_target"
      return 1
    }
  fi
  # previous_target may itself be a dangling symlink (moved aside above),
  # which -e alone would miss and skip restoring.
  if ! mkdir -p "$(dirname "$DEPENDENCY_TARGET")"; then
    [[ ! -e "$previous_target" && ! -L "$previous_target" ]] || mv "$previous_target" "$DEPENDENCY_TARGET"
    rm -rf "$temporary_target"
    return 1
  fi
  if ! mv "$temporary_target" "$DEPENDENCY_TARGET"; then
    [[ ! -e "$previous_target" && ! -L "$previous_target" ]] || mv "$previous_target" "$DEPENDENCY_TARGET"
    rm -rf "$temporary_target"
    return 1
  fi
  rm -rf "$previous_target"
}

# Whether the currently loaded dependency's target is a valid Selfishell-
# managed install (used only when Selfishell state for it exists). Strict on
# purpose: Selfishell owns this path, so a directory, non-executable file, or
# symlink here means a corrupted install that must be repaired, not a shape
# to tolerate.
dependency_managed_target_is_valid() {
  case "$DEPENDENCY_TYPE" in
    download)
      [[ -f "$DEPENDENCY_TARGET" && ! -L "$DEPENDENCY_TARGET" && -x "$DEPENDENCY_TARGET" ]]
      ;;
    git)
      [[ -d "$DEPENDENCY_TARGET" && ! -L "$DEPENDENCY_TARGET" &&
        -e "$DEPENDENCY_TARGET/.git" && -e "$DEPENDENCY_TARGET/$DEPENDENCY_MARKER" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

# Whether the currently loaded dependency's target, which Selfishell does not
# own (no recorded state), is usable as that dependency. Deliberately looser
# than dependency_managed_target_is_valid: an external install may
# legitimately be a symlink, and a git-type external checkout need not carry
# `.git` (it may be a release archive or package-managed copy).
dependency_external_target_is_usable() {
  case "$DEPENDENCY_TYPE" in
    download)
      [[ -f "$DEPENDENCY_TARGET" && -x "$DEPENDENCY_TARGET" ]]
      ;;
    git)
      [[ -d "$DEPENDENCY_TARGET" && -e "$DEPENDENCY_TARGET/$DEPENDENCY_MARKER" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

dependency_install() {
  local name="$1"
  local platform="$2"
  local architecture="$3"
  local force="${4:-0}"
  local installed
  # Only a genuine version bump counts as an update; reinstalling a
  # corrupted or force-refreshed same-version target is still a fresh
  # Installed, since nothing was actually approved-and-working before.
  local approved_version_changed=0

  dependency_load "$name" "$platform" "$architecture" || return
  selfishell_initialize_paths
  installed="$(dependency_installed_version "$name")"

  if [[ -n "$installed" ]]; then
    if [[ "$force" == 0 && "$installed" == "$DEPENDENCY_VERSION" ]] && dependency_managed_target_is_valid; then
      SELFISHELL_UNCHANGED_COUNT=$((SELFISHELL_UNCHANGED_COUNT + 1))
      return
    fi
    [[ "$installed" == "$DEPENDENCY_VERSION" ]] || approved_version_changed=1
  elif [[ -e "$DEPENDENCY_TARGET" || -L "$DEPENDENCY_TARGET" ]]; then
    if dependency_external_target_is_usable; then
      printf '%sExternally installed; preserving:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$DEPENDENCY_TARGET"
      return
    fi
    cli_error "An existing $DEPENDENCY_TARGET is not a usable $name installation; leaving it in place."
    return 1
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
  if [[ "$approved_version_changed" == 1 ]]; then
    printf '%sUpdated approved dependency:%s %s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$name" "$DEPENDENCY_VERSION"
  else
    printf '%sInstalled approved dependency:%s %s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$name" "$DEPENDENCY_VERSION"
  fi
}
