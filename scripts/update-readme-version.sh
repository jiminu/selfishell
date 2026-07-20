#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${SELFISHELL_TEST_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
VERSION_FILE="$ROOT_DIR/VERSION"

if [[ ! -f "$VERSION_FILE" ]]; then
  printf 'Error: VERSION file not found at %s\n' "$VERSION_FILE" >&2
  exit 1
fi

NEW_VER=$(tr -d '[:space:]' <"$VERSION_FILE")
if [[ ! "$NEW_VER" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  printf 'Error: Invalid version format in VERSION file: %s\n' "$NEW_VER" >&2
  exit 1
fi

update_file_versions() {
  local target_file="$1"
  local ver="$2"

  if [[ ! -f "$target_file" ]]; then
    printf 'Warning: Target file not found at %s\n' "$target_file" >&2
    return
  fi

  # Perl is highly compatible across macOS and Linux for in-place editing
  if command -v perl >/dev/null 2>&1; then
    # 1. raw.githubusercontent.com/jiminu/selfishell/vX.Y.Z/install.sh -> raw.githubusercontent.com/jiminu/selfishell/v<NEW_VER>/install.sh
    perl -pi -e "s|raw\.githubusercontent\.com/jiminu/selfishell/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?/install\.sh|raw.githubusercontent.com/jiminu/selfishell/v${ver}/install.sh|g" "$target_file"
    # 2. --version X.Y.Z -> --version <NEW_VER>
    perl -pi -e "s|--version [0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?|--version ${ver}|g" "$target_file"
  else
    # Fallback to python3 if perl is missing
    python3 -c "
import re
with open('$target_file', 'r') as f:
    content = f.read()
content = re.sub(r'raw\.githubusercontent\.com/jiminu/selfishell/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?/install\.sh', 'raw.githubusercontent.com/jiminu/selfishell/v$ver/install.sh', content)
content = re.sub(r'--version [0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?', '--version $ver', content)
with open('$target_file', 'w') as f:
    f.write(content)
"
  fi
}

update_file_versions "$ROOT_DIR/README.md" "$NEW_VER"
update_file_versions "$ROOT_DIR/docs/INSTALLATION.md" "$NEW_VER"

printf 'Successfully updated version to %s in README.md and docs/INSTALLATION.md\n' "$NEW_VER"
