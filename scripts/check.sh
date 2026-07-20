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
  scripts/benchmark.sh
  scripts/build-release.sh
  scripts/release-check.sh
  scripts/next-patch-version.sh
  scripts/update-dependencies.sh
  scripts/update-readme-version.sh
  scripts/neovim-e2e.sh
  scripts/ubuntu-container-e2e.sh
  scripts/workflow-failure-issue.sh
  tests/cli_test.bash
  tests/common_zsh_test.bash
  tests/neovim_config_test.bash
  tests/managed_install_test.bash
  tests/installers_test.bash
  tests/package_adapters_test.bash
  tests/platform_test.bash
  tests/profiles_test.bash
  tests/proxy_test.bash
  tests/tool_status_test.bash
  tests/workflow_notifications_test.bash
  tests/release_bootstrap_test.bash
  tests/release_version_test.bash
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
  common/runtime.zsh
  common/completion.zsh
  common/interactive.zsh
  common/update-notice.zsh
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

# Verify version consistency
printf 'Verifying version consistency\n'

# 1. Selfishell version consistency check
version=$(tr -d '[:space:]' <VERSION)

# Helper function to check selfishell version in a file
verify_selfishell_version() {
  local file="$1"
  local expected="$2"

  # Check raw URLs
  local urls
  urls=$(grep -oE "raw\.githubusercontent\.com/jiminu/selfishell/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?/install\.sh" "$file" || true)
  if [[ -n "$urls" ]]; then
    while read -r url; do
      if [[ "$url" =~ selfishell/v([^/]+)/install\.sh ]]; then
        local found="${BASH_REMATCH[1]}"
        if [[ "$found" != "$expected" ]]; then
          printf 'Error: Version mismatch in %s URL. Expected: %s, Found: %s\n' "$file" "$expected" "$found" >&2
          exit 1
        fi
      fi
    done <<<"$urls"
  fi

  # Check --version argument
  local args
  args=$(grep -oE "\-\-version [0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?" "$file" || true)
  if [[ -n "$args" ]]; then
    while read -r arg; do
      if [[ "$arg" =~ --version[[:space:]]+([^[:space:]]+) ]]; then
        local found="${BASH_REMATCH[1]}"
        if [[ "$found" != "$expected" ]]; then
          printf 'Error: Version mismatch in %s --version argument. Expected: %s, Found: %s\n' "$file" "$expected" "$found" >&2
          exit 1
        fi
      fi
    done <<<"$args"
  fi
}

verify_selfishell_version "README.md" "$version"
verify_selfishell_version "docs/INSTALLATION.md" "$version"

printf 'Version consistency checks passed.\n'

printf 'Running tests\n'
bash tests/run.bash
