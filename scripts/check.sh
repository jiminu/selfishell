#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

bash_files=(
  main.sh
  mac/mac.sh
  ubuntu/ubuntu.sh
  common/common.sh
  scripts/check.sh
  tests/common_test.bash
  tests/run.bash
  tests/test_helper.bash
  tests/ubuntu_packages_test.bash
)

zsh_files=(
  mac/.zshrc
  ubuntu/.zshrc
  common/common.zsh
  common/aliases-common.zsh
  common/aliases-git.zsh
  common/aliases-kubectl.zsh
)

printf 'Checking Bash syntax\n'
bash -n "${bash_files[@]}"

printf 'Checking Zsh syntax\n'
zsh -n "${zsh_files[@]}"

printf 'Running ShellCheck\n'
shellcheck -x "${bash_files[@]}"

printf 'Checking shell formatting\n'
shfmt -d -i 2 -ci "${bash_files[@]}"

printf 'Running tests\n'
bash tests/run.bash
