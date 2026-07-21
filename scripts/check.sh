#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Discovered rather than hand-listed so a new lib/scripts/tests file is
# checked automatically; mapfile is intentionally avoided since it's not
# available on Bash 3.2 (macOS's default /bin/bash).
bash_files=()
while IFS= read -r file; do
  bash_files+=("$file")
done < <(
  {
    printf '%s\n' bin/selfishell install.sh
    find lib scripts tests -type f \( -name '*.sh' -o -name '*.bash' \)
  } | sort -u
)

zsh_files=()
while IFS= read -r file; do
  zsh_files+=("$file")
done < <(
  {
    printf '%s\n' mac/.zshrc ubuntu/.zshrc
    find common -type f -name '*.zsh'
  } | sort -u
)

printf 'Checking Bash syntax\n'
bash -n "${bash_files[@]}"

printf 'Checking Zsh syntax\n'
zsh -n "${zsh_files[@]}"

printf 'Running ShellCheck\n'
shellcheck -x "${bash_files[@]}"

printf 'Checking shell formatting\n'
shfmt -d -i 2 -ci "${bash_files[@]}"

# Verify version consistency
printf 'Verifying version consistency\n'

version=$(tr -d '[:space:]' <VERSION)
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  printf 'Error: Invalid version format in VERSION file: %s\n' "$version" >&2
  exit 1
fi

printf 'Version consistency checks passed.\n'

printf 'Running tests\n'
bash tests/run.bash
