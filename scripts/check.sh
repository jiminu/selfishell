#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Discover new files automatically; avoid mapfile for Bash 3.2 compatibility.
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
    printf '%s\n' config/macos/zshrc config/ubuntu/zshrc
    find config -type f -name '*.zsh'
  } | sort -u
)

printf 'Checking Bash syntax\n'
for file in "${bash_files[@]}"; do
  bash -n "$file"
done

printf 'Checking Zsh syntax\n'
for file in "${zsh_files[@]}"; do
  zsh -n "$file"
done

printf 'Running ShellCheck\n'
shellcheck -x "${bash_files[@]}"

printf 'Checking shell formatting\n'
shfmt -d -i 2 -ci "${bash_files[@]}"

printf 'Running tests\n'
bash tests/run.bash
