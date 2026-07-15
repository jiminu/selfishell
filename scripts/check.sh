#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

bash_files=(
  bin/selfishell
  bootstrap.sh
  legacy/common.sh
  legacy/macos.sh
  legacy/ubuntu.sh
  lib/common.sh
  lib/managed.sh
  lib/paths.sh
  lib/platform.sh
  lib/commands/doctor.sh
  lib/commands/help.sh
  lib/commands/install.sh
  lib/commands/status.sh
  lib/commands/uninstall.sh
  lib/commands/version.sh
  lib/platforms/macos.sh
  lib/platforms/ubuntu.sh
  scripts/check.sh
  tests/cli_test.bash
  tests/common_test.bash
  tests/managed_install_test.bash
  tests/platform_test.bash
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
