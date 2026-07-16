#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

bash_files=(
  bin/selfishell
  install.sh
  lib/common.sh
  lib/managed.sh
  lib/installers.sh
  lib/packages.sh
  lib/paths.sh
  lib/platform.sh
  lib/profiles.sh
  lib/dependencies.sh
  lib/tool_status.sh
  lib/releases.sh
  lib/package_managers/apt.sh
  lib/package_managers/homebrew.sh
  lib/commands/doctor.sh
  lib/commands/help.sh
  lib/commands/install.sh
  lib/commands/status.sh
  lib/commands/uninstall.sh
  lib/commands/version.sh
  lib/commands/update.sh
  lib/commands/rollback.sh
  scripts/check.sh
  scripts/build-release.sh
  scripts/release-check.sh
  scripts/update-dependencies.sh
  scripts/ubuntu-container-e2e.sh
  tests/cli_test.bash
  tests/common_zsh_test.bash
  tests/managed_install_test.bash
  tests/installers_test.bash
  tests/package_adapters_test.bash
  tests/platform_test.bash
  tests/profiles_test.bash
  tests/proxy_test.bash
  tests/tool_status_test.bash
  tests/release_bootstrap_test.bash
  tests/run.bash
  tests/test_helper.bash
  tests/updates_test.bash
  tests/dependency_updates_test.bash
  tests/lifecycle_e2e_test.bash
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
