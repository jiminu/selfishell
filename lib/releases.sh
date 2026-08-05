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

  [[ "$root" == "$official_root" || -n "${SELFISHELL_RELEASE_TAGS_API_URL:-${SELFISHELL_RELEASE_API_URL:-}}" ]] || return 1
  api_url="${SELFISHELL_RELEASE_TAGS_API_URL:-${SELFISHELL_RELEASE_API_URL:-https://api.github.com/repos/jiminu/selfishell/tags?per_page=1}}"
  response="$(selfishell_curl metadata \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "$api_url" 2>/dev/null)" || return 1
  version="$(printf '%s\n' "$response" | sed -n \
    -e 's/.*"name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' \
    -e 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' | sed -n '1p')"
  [[ -n "$version" ]] || return 1
  published_version="$(selfishell_curl metadata "$root/download/v${version}/VERSION" 2>/dev/null)" || return 1
  published_version="${published_version#v}"
  [[ "$published_version" == "$version" ]] || return 1
  printf '%s\n' "$version"
}

release_installation_paths() {
  local releases_dir

  releases_dir="$(dirname "$SELFISHELL_ROOT")"
  if [[ "$(basename "$releases_dir")" != releases || ! -L "$(dirname "$releases_dir")/current" ]]; then
    cli_error "This command requires a versioned Selfishell installation."
    return 1
  fi
  SELFISHELL_RELEASES_DIR="$releases_dir"
  SELFISHELL_SHARE_DIR="$(dirname "$releases_dir")"
}

# Confirms "$SELFISHELL_RELEASES_DIR/$version" both looks complete (an
# executable CLI is present) and actually contains the version it claims to,
# so a corrupted, incomplete, or mislabeled release directory is never
# activated or rolled back to.
release_directory_is_valid() {
  local version="$1"
  local dir="$SELFISHELL_RELEASES_DIR/$version"

  [[ -x "$dir/bin/selfishell" && -r "$dir/VERSION" && "$(<"$dir/VERSION")" == "$version" ]]
}

# Rejects anything a release archive should never contain (FIFOs, device
# nodes, sockets, ...) and any symlink that isn't a plain, existing sibling
# path inside the archive (the release build packages "bin/sfs -> selfishell"
# this way) -- absolute, traversal-shaped, or dangling targets are rejected
# so extraction can't smuggle a link pointing outside the release directory.
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

release_prune_inactive() {
  local current_version previous_version release_dir release_version

  current_version="$(readlink "$SELFISHELL_SHARE_DIR/current")"
  current_version="${current_version##*/}"
  previous_version=""
  if [[ -L "$SELFISHELL_SHARE_DIR/previous" ]]; then
    previous_version="$(readlink "$SELFISHELL_SHARE_DIR/previous")"
    previous_version="${previous_version##*/}"
  fi

  for release_dir in "$SELFISHELL_RELEASES_DIR"/*; do
    [[ -d "$release_dir" && ! -L "$release_dir" ]] || continue
    release_version="${release_dir##*/}"
    [[ "$release_version" == "$current_version" || "$release_version" == "$previous_version" ]] && continue
    rm -rf "$release_dir"
    printf '%sRemoved inactive Selfishell release:%s %s.\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$release_version"
  done
}

release_install() {
  local version="$1"
  local platform architecture archive_name release_url temporary_dir archive checksum_file expected actual staging

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
  # A duplicate SHA256SUMS line for the same archive (even a legitimate,
  # identical one) would otherwise turn $expected into a multi-line value
  # that can never equal the single-line $actual; `sort -u` collapses
  # agreeing duplicates while still failing closed on genuinely conflicting
  # ones (the multi-line value then just stays a mismatch).
  expected="$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksum_file" | sort -u)"
  actual="$(dependency_sha256 "$archive")"
  if [[ -z "$expected" || "$actual" != "$expected" ]]; then
    cli_error "Checksum mismatch for $archive_name."
    rm -rf "$temporary_dir"
    return 1
  fi

  if [[ -d "$SELFISHELL_RELEASES_DIR/$version" ]]; then
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
  fi
  rm -rf "$temporary_dir"

  # A failure here only loses the rollback link, not the update itself, so
  # warn and continue rather than aborting an otherwise-successful update.
  if ! release_atomic_link "$(readlink "$SELFISHELL_SHARE_DIR/current")" "$SELFISHELL_SHARE_DIR/previous"; then
    cli_error "Failed to update the previous release link; continuing."
  fi
  release_atomic_link "releases/$version" "$SELFISHELL_SHARE_DIR/current" || {
    cli_error "Failed to activate Selfishell $version."
    return 1
  }
  printf '%sSelfishell CLI updated to %s.%s\n' "$SELFISHELL_COLOR_GREEN" "$version" "$SELFISHELL_COLOR_RESET"
  release_prune_inactive
}
