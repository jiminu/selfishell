#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="$(<"$ROOT_DIR/VERSION")"
output_dir="$ROOT_DIR/dist"

while (("$#" > 0)); do
  case "$1" in
    --version)
      shift
      version="${1:-}"
      ;;
    --output)
      shift
      output_dir="${1:-}"
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      exit 2
      ;;
  esac
  shift
done

[[ -n "$version" ]] || {
  printf 'Version is required.\n' >&2
  exit 2
}
[[ "$output_dir" == /* ]] || output_dir="$ROOT_DIR/$output_dir"

staging_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-release.XXXXXX")"
trap 'rm -rf "$staging_root"' EXIT HUP INT TERM
payload_dir="$staging_root/payload"
archive_files="$staging_root/archive-files"
mkdir -p "$payload_dir" "$output_dir"

cp -R \
  "$ROOT_DIR/bin" \
  "$ROOT_DIR/lib" \
  "$ROOT_DIR/profiles" \
  "$ROOT_DIR/common" \
  "$ROOT_DIR/mac" \
  "$ROOT_DIR/ubuntu" \
  "$payload_dir/"
cp "$ROOT_DIR/dependencies.conf" "$payload_dir/"
printf '%s\n' "$version" >"$payload_dir/VERSION"
chmod 0644 "$payload_dir/VERSION"

# Normalize archive inputs so builds do not depend on checkout ownership,
# staging timestamps, filesystem traversal order, or gzip headers.
TZ=UTC find "$payload_dir" -exec touch -h -t 200001010000 {} +
(
  cd "$payload_dir"
  find . \( -type f -o -type l \) -print | LC_ALL=C sort >"$archive_files"
)

create_release_archive() {
  local destination="$1"

  if tar --version 2>/dev/null | head -n 1 | grep -q 'GNU tar'; then
    tar --format=ustar --owner=0 --group=0 --numeric-owner --mtime=@946684800 \
      -cf - -C "$payload_dir" -T "$archive_files" | gzip -n >"$destination"
  else
    COPYFILE_DISABLE=1 tar --format=ustar --uid 0 --gid 0 --uname root --gname root \
      -cf - -C "$payload_dir" -T "$archive_files" | gzip -n >"$destination"
  fi
}

for platform in linux macos; do
  for architecture in amd64 arm64; do
    archive="selfishell-${version}-${platform}-${architecture}.tar.gz"
    create_release_archive "$output_dir/$archive"
  done
done

(
  cd "$output_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum selfishell-"$version"-*.tar.gz >SHA256SUMS
  else
    shasum -a 256 selfishell-"$version"-*.tar.gz >SHA256SUMS
  fi
)
printf '%s\n' "$version" >"$output_dir/VERSION"

printf 'Built Selfishell %s release artifacts in %s\n' "$version" "$output_dir"
