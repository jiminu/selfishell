#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/lib/common.sh"

version="${1:-}"
repository="${2:-jiminu/selfishell}"

if ! selfishell_version_is_valid "$version"; then
  printf 'Usage: scripts/verify-published-release.sh VERSION [OWNER/REPOSITORY]\n' >&2
  exit 2
fi
case "$repository" in
  */*) ;;
  *)
    printf 'Repository must use OWNER/REPOSITORY format: %s\n' "$repository" >&2
    exit 2
    ;;
esac

for required_command in gh curl; do
  command -v "$required_command" >/dev/null 2>&1 || {
    printf '%s is required to verify a published release.\n' "$required_command" >&2
    exit 1
  }
done

tag="v$version"
release_root="${SELFISHELL_VERIFY_RELEASE_ROOT:-https://github.com/$repository/releases}"
raw_root="${SELFISHELL_VERIFY_RAW_ROOT:-https://raw.githubusercontent.com/$repository}"
temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-published-release.XXXXXX")"
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM

metadata="$(gh release view "$tag" --repo "$repository" \
  --json tagName,isPrerelease,url \
  --jq '[.tagName, (.isPrerelease | tostring), .url] | @tsv')"
IFS=$'\t' read -r published_tag published_prerelease release_url <<<"$metadata"
[[ "$published_tag" == "$tag" ]] || {
  printf 'Published tag mismatch: expected %s, got %s\n' "$tag" "$published_tag" >&2
  exit 1
}

expected_prerelease=false
[[ "$version" != *-* ]] || expected_prerelease=true
[[ "$published_prerelease" == "$expected_prerelease" ]] || {
  printf 'Release classification mismatch for %s\n' "$tag" >&2
  exit 1
}

expected_assets="$temporary_root/expected-assets"
actual_assets="$temporary_root/actual-assets"
printf '%s\n' \
  SHA256SUMS \
  VERSION \
  "selfishell-$version-linux-amd64.tar.gz" \
  "selfishell-$version-linux-arm64.tar.gz" \
  "selfishell-$version-macos-amd64.tar.gz" \
  "selfishell-$version-macos-arm64.tar.gz" |
  LC_ALL=C sort >"$expected_assets"
gh release view "$tag" --repo "$repository" --json assets --jq '.assets[].name' |
  LC_ALL=C sort >"$actual_assets"
if ! cmp -s "$expected_assets" "$actual_assets"; then
  printf 'Published asset set mismatch for %s.\n' "$tag" >&2
  diff -u "$expected_assets" "$actual_assets" >&2 || true
  exit 1
fi

download_dir="$temporary_root/assets"
mkdir -p "$download_dir"
gh release download "$tag" --repo "$repository" --dir "$download_dir"
(
  cd "$download_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c SHA256SUMS
  else
    shasum -a 256 -c SHA256SUMS
  fi
)

if gh attestation verify --help >/dev/null 2>&1; then
  for archive in "$download_dir"/selfishell-"$version"-*.tar.gz; do
    gh attestation verify "$archive" --repo "$repository" >/dev/null
  done
  printf 'Artifact attestations verified.\n'
else
  printf 'Artifact attestation verification unavailable; skipped.\n'
fi

installer="$temporary_root/install.sh"
curl -fsSL "$raw_root/$tag/install.sh" -o "$installer"
test_home="$temporary_root/home"
prefix="$temporary_root/prefix"
mkdir -p "$test_home"
HOME="$test_home" \
  XDG_CACHE_HOME="$test_home/.cache" \
  XDG_CONFIG_HOME="$test_home/.config" \
  XDG_STATE_HOME="$test_home/.local/state" \
  SELFISHELL_RELEASE_ROOT="$release_root" \
  bash "$installer" --version "$version" --prefix "$prefix" >/dev/null
reported_version="$("$prefix/bin/selfishell" version)"
[[ "$reported_version" == "selfishell $version" ]] || {
  printf 'Installed CLI version mismatch: %s\n' "$reported_version" >&2
  exit 1
}

if [[ "$expected_prerelease" == false ]]; then
  latest_version="$(curl -fsSL "$release_root/latest/download/VERSION")"
  latest_version="${latest_version//$'\n'/}"
  [[ "$latest_version" == "$version" ]] || {
    printf 'Latest stable version mismatch: expected %s, got %s\n' "$version" "$latest_version" >&2
    exit 1
  }
elif latest_version="$(curl -fsSL "$release_root/latest/download/VERSION" 2>/dev/null)"; then
  latest_version="${latest_version//$'\n'/}"
  [[ "$latest_version" != "$version" ]] || {
    printf 'Pre-release unexpectedly replaced the latest stable version: %s\n' "$version" >&2
    exit 1
  }
fi

printf 'Published release %s verified: %s\n' "$version" "$release_url"
