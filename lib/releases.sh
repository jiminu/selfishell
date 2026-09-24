#!/usr/bin/env bash

release_root_url() {
  printf '%s\n' "${SELFISHELL_RELEASE_ROOT:-https://github.com/jiminu/selfishell/releases}"
}

release_latest_version() {
  local official_root="https://github.com/jiminu/selfishell/releases"
  local root api_url response version published_version
  # This is called from the interactive shell's background update-notice
  # check, which holds a lock for as long as it runs. Metadata mode therefore
  # has a short total deadline in addition to the shared connection limits.

  root="$(release_root_url)"
  if version="$(selfishell_curl metadata "$root/latest/download/VERSION" 2>/dev/null)"; then
    version="${version#v}"
    [[ -n "$version" ]] && {
      printf '%s\n' "$version"
      return
    }
  fi

  [[ "$root" == "$official_root" || -n "${SELFISHELL_RELEASE_TAGS_API_URL:-}" ]] || return 1
  api_url="${SELFISHELL_RELEASE_TAGS_API_URL:-https://api.github.com/repos/jiminu/selfishell/tags?per_page=1}"
  response="$(selfishell_curl metadata \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "$api_url" 2>/dev/null)" || return 1
  version="$(printf '%s\n' "$response" | sed -n \
    -e 's/.*"name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' | sed -n '1p')"
  [[ -n "$version" ]] || return 1
  published_version="$(selfishell_curl metadata "$root/download/v${version}/VERSION" 2>/dev/null)" || return 1
  published_version="${published_version#v}"
  [[ "$published_version" == "$version" ]] || return 1
  printf '%s\n' "$version"
}

release_installation_paths() {
  local releases_dir share_dir

  releases_dir="${SELFISHELL_ROOT%/*}"
  share_dir="${releases_dir%/*}"

  if [[ "${releases_dir##*/}" != releases ||
    ! -L "$share_dir/current" ]]; then
    cli_error "This command requires a versioned Selfishell installation."
    return 1
  fi
  SELFISHELL_RELEASES_DIR="$releases_dir"
  SELFISHELL_SHARE_DIR="$share_dir"
}

# Confirms the release directory is real (not a symlink, which -x/-r would
# follow, accepting another path's contents), complete, and actually holds the
# version it claims, so a corrupt or mislabeled one is never activated.
release_directory_is_valid() {
  local version="$1"
  local dir="$SELFISHELL_RELEASES_DIR/$version"

  [[ -d "$dir" && ! -L "$dir" ]] || return 1
  [[ -x "$dir/bin/selfishell" && -r "$dir/VERSION" && "$(<"$dir/VERSION")" == "$version" ]]
}

# Rejects what a release archive should never hold (FIFOs, device nodes,
# sockets) and any symlink that isn't a plain existing sibling, as the build
# packages "bin/sfs -> selfishell": absolute, traversal-shaped, or dangling
# targets could smuggle a link outside the release directory.
release_validate_extracted_members() {
  local staging="$1"
  local unexpected link target

  unexpected="$(find "$staging" ! -type f ! -type d ! -type l)"
  [[ -z "$unexpected" ]] || return 1

  while IFS= read -r link; do
    target="$(readlink "$link")"
    case "$target" in
      /* | .. | ../* | */.. | */../*) return 1 ;;
    esac
    [[ -e "$link" ]] || return 1
  done < <(find "$staging" -type l)
}

release_atomic_link() {
  local target="$1"
  local path="$2"
  local temporary

  [[ ! -e "$path" || -L "$path" ]] || return 1

  temporary="$(selfishell_unique_path "${path}.tmp.$$")"
  ln -s "$target" "$temporary" || return 1
  if mv -fT "$temporary" "$path" 2>/dev/null; then
    return
  fi
  if ! mv -fh "$temporary" "$path"; then
    rm -f "$temporary"
    return 1
  fi
}

release_platform() {
  case "$(uname -s)" in
    Darwin) printf 'macos\n' ;;
    Linux) printf 'linux\n' ;;
    *)
      cli_error "Unsupported operating system: $(uname -s)"
      return 1
      ;;
  esac
}

# The retention contract in docs/UPDATES.md: only the active and rollback
# releases are kept. These are managed product state, not user data, so a
# superseded one goes silently; `status` reports what can still be rolled to.
release_prune_inactive() {
  local current_version previous_version release_dir release_version

  current_version="$(readlink "$SELFISHELL_SHARE_DIR/current")"
  current_version="${current_version##*/}"
  previous_version=""
  if [[ -L "$SELFISHELL_SHARE_DIR/previous" ]]; then
    previous_version="$(readlink "$SELFISHELL_SHARE_DIR/previous")"
    previous_version="${previous_version##*/}"
  fi

  # Staging left by an interrupted update; a day-old one belongs to no running update.
  find "$SELFISHELL_RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d -name '.*.tmp.*' -mmin +1440 \
    -exec rm -rf {} + 2>/dev/null || true
  for release_dir in "$SELFISHELL_RELEASES_DIR"/*; do
    [[ -d "$release_dir" && ! -L "$release_dir" ]] || continue
    release_version="${release_dir##*/}"
    [[ "$release_version" == "$current_version" || "$release_version" == "$previous_version" ]] && continue
    rm -rf "$release_dir"
  done
}

release_install() {
  local version="$1"
  local platform architecture archive_name release_url temporary_dir archive checksum_file expected actual staging
  local current_target nested_staging

  release_installation_paths || return
  platform="$(release_platform)"
  architecture="$(detect_architecture)"
  archive_name="selfishell-${version}-${platform}-${architecture}.tar.gz"
  release_url="$(release_root_url)/download/v${version}"
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-update.XXXXXX")" || return 1
  archive="$temporary_dir/$archive_name"
  checksum_file="$temporary_dir/SHA256SUMS"

  selfishell_curl transfer "$release_url/$archive_name" -o "$archive" || {
    rm -rf "$temporary_dir"
    return 1
  }
  selfishell_curl transfer "$release_url/SHA256SUMS" -o "$checksum_file" || {
    rm -rf "$temporary_dir"
    return 1
  }
  # Collapse agreeing checksum entries; conflicting duplicates still fail verification.
  expected="$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksum_file" | sort -u)"
  actual="$(dependency_sha256 "$archive")"
  if [[ -z "$expected" || "$actual" != "$expected" ]]; then
    cli_error "Checksum mismatch for $archive_name."
    rm -rf "$temporary_dir"
    return 1
  fi

  if [[ -e "$SELFISHELL_RELEASES_DIR/$version" || -L "$SELFISHELL_RELEASES_DIR/$version" ]]; then
    release_directory_is_valid "$version" || {
      cli_error "Existing release is incomplete: $SELFISHELL_RELEASES_DIR/$version"
      rm -rf "$temporary_dir"
      return 1
    }
  else
    staging="$(mktemp -d "$SELFISHELL_RELEASES_DIR/.${version}.tmp.XXXXXX")" || {
      rm -rf "$temporary_dir"
      return 1
    }
    tar -xzf "$archive" -C "$staging" || {
      rm -rf "$temporary_dir" "$staging"
      return 1
    }
    release_validate_extracted_members "$staging" || {
      cli_error "Release archive contains unsupported file types."
      rm -rf "$temporary_dir" "$staging"
      return 1
    }
    if [[ ! -x "$staging/bin/selfishell" || ! -r "$staging/VERSION" || "$(<"$staging/VERSION")" != "$version" ]]; then
      cli_error "Release archive is invalid or has the wrong version."
      rm -rf "$temporary_dir" "$staging"
      return 1
    fi
    mv "$staging" "$SELFISHELL_RELEASES_DIR/$version" || {
      rm -rf "$temporary_dir" "$staging"
      return 1
    }
    # A concurrent update that created the release first receives this staging inside it.
    nested_staging="$SELFISHELL_RELEASES_DIR/$version/${staging##*/}"
    [[ ! -d "$nested_staging" ]] || rm -rf "$nested_staging"
  fi
  rm -rf "$temporary_dir"

  current_target="$(readlink "$SELFISHELL_SHARE_DIR/current")"
  # A failure here only loses the rollback link, not the update itself, so
  # warn and continue. A concurrent update may already have activated this
  # version; its current release is not a rollback target.
  if [[ "$current_target" != "releases/$version" ]] &&
    ! release_atomic_link "$current_target" "$SELFISHELL_SHARE_DIR/previous"; then
    cli_warn "Failed to update the previous release link; continuing."
  fi
  release_atomic_link "releases/$version" "$SELFISHELL_SHARE_DIR/current" || {
    cli_error "Failed to activate Selfishell $version."
    return 1
  }
  release_prune_inactive
}
