#!/usr/bin/env bash

release_root_url() {
  printf '%s\n' "${SELFISHELL_RELEASE_ROOT:-https://github.com/jiminu/selfishell/releases}"
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

release_atomic_link() {
  local target="$1"
  local path="$2"
  local temporary="${path}.tmp.$$"
  ln -s "$target" "$temporary"
  if mv -fT "$temporary" "$path" 2>/dev/null; then
    return
  fi
  mv -fh "$temporary" "$path"
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

release_install() {
  local version="$1"
  local platform architecture archive_name release_url temporary_dir archive checksum_file expected actual staging

  release_installation_paths || return
  platform="$(release_platform)"
  architecture="$(detect_architecture)"
  archive_name="selfishell-${version}-${platform}-${architecture}.tar.gz"
  release_url="$(release_root_url)/download/v${version}"
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-update.XXXXXX")"
  archive="$temporary_dir/$archive_name"
  checksum_file="$temporary_dir/SHA256SUMS"

  curl -fsSL "$release_url/$archive_name" -o "$archive" || {
    rm -rf "$temporary_dir"
    return 1
  }
  curl -fsSL "$release_url/SHA256SUMS" -o "$checksum_file" || {
    rm -rf "$temporary_dir"
    return 1
  }
  expected="$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksum_file")"
  actual="$(dependency_sha256 "$archive")"
  if [[ -z "$expected" || "$actual" != "$expected" ]]; then
    cli_error "Checksum mismatch for $archive_name."
    rm -rf "$temporary_dir"
    return 1
  fi

  if [[ ! -d "$SELFISHELL_RELEASES_DIR/$version" ]]; then
    staging="$(mktemp -d "$SELFISHELL_RELEASES_DIR/.${version}.tmp.XXXXXX")"
    tar -xzf "$archive" -C "$staging"
    if [[ ! -x "$staging/bin/selfishell" || ! -r "$staging/VERSION" || "$(<"$staging/VERSION")" != "$version" ]]; then
      cli_error "Release archive is invalid or has the wrong version."
      rm -rf "$temporary_dir" "$staging"
      return 1
    fi
    mv "$staging" "$SELFISHELL_RELEASES_DIR/$version"
  fi
  rm -rf "$temporary_dir"

  release_atomic_link "$(readlink "$SELFISHELL_SHARE_DIR/current")" "$SELFISHELL_SHARE_DIR/previous"
  release_atomic_link "releases/$version" "$SELFISHELL_SHARE_DIR/current"
  printf 'Selfishell CLI updated to %s.\n' "$version"
}
