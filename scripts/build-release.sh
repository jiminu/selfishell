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

for platform in linux macos; do
  for architecture in amd64 arm64; do
    archive="selfishell-${version}-${platform}-${architecture}.tar.gz"
    tar -czf "$output_dir/$archive" -C "$payload_dir" .
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
