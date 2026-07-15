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
             [--profile NAME] [--skip-packages]

Options:
  --version VERSION  Install an exact Selfishell release
  --prefix PATH      Installation prefix (default: $HOME/.local)
  --setup            Run 'selfishell install' after installing the CLI
  --yes              Skip setup confirmation when used with --setup
  --profile NAME     Profile passed to setup (default: developer)
  --skip-packages    Pass configuration-only mode to setup
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

bootstrap_validate_version() {
  case "$1" in
    "" | -* | *[!0-9A-Za-z.-]*)
      bootstrap_error "Invalid version: $1"
      return 1
      ;;
  esac
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

main() {
  local version=""
  local prefix="${HOME}/.local"
  local setup=0
  local assume_yes=0
  local profile=developer
  local skip_packages=0
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
    version="$(curl -fsSL "$SELFISHELL_RELEASE_ROOT/latest/download/VERSION")"
    version="${version#v}"
  fi
  bootstrap_validate_version "$version"

  release_url="$SELFISHELL_RELEASE_ROOT/download/v${version}"
  archive_name="selfishell-${version}-${platform}-${architecture}.tar.gz"
  SELFISHELL_TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-install.XXXXXX")"
  trap bootstrap_cleanup EXIT HUP INT TERM
  archive_file="$SELFISHELL_TEMP_DIR/$archive_name"
  checksum_file="$SELFISHELL_TEMP_DIR/SHA256SUMS"

  printf 'Downloading Selfishell %s for %s/%s\n' "$version" "$platform" "$architecture"
  curl -fsSL "$release_url/$archive_name" -o "$archive_file"
  curl -fsSL "$release_url/SHA256SUMS" -o "$checksum_file"

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

  bootstrap_atomic_link "releases/$version" "$share_dir/current"
  bootstrap_atomic_link "$share_dir/current/bin/selfishell" "$bin_dir/selfishell"
  bootstrap_atomic_link selfishell "$bin_dir/sfs"

  printf 'Installed Selfishell %s\n' "$version"
  if [[ ":$PATH:" != *":$bin_dir:"* ]]; then
    printf 'Add %s to PATH to use selfishell from a new shell.\n' "$bin_dir"
  fi

  if [[ "$setup" == "1" ]]; then
    setup_args=(install --profile "$profile")
    [[ "$skip_packages" == "1" ]] && setup_args+=(--skip-packages)
    [[ "$assume_yes" == "1" ]] && setup_args+=(--yes)
    "$bin_dir/selfishell" "${setup_args[@]}"
  fi
}

main "$@"
