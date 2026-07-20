#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bash "$ROOT_DIR/tests/cli_test.bash"
bash "$ROOT_DIR/tests/common_zsh_test.bash"
bash "$ROOT_DIR/tests/neovim_config_test.bash"
bash "$ROOT_DIR/tests/managed_install_test.bash"
bash "$ROOT_DIR/tests/installers_test.bash"
bash "$ROOT_DIR/tests/package_adapters_test.bash"
bash "$ROOT_DIR/tests/platform_test.bash"
bash "$ROOT_DIR/tests/profiles_test.bash"
bash "$ROOT_DIR/tests/proxy_test.bash"
bash "$ROOT_DIR/tests/tool_status_test.bash"
bash "$ROOT_DIR/tests/workflow_notifications_test.bash"
bash "$ROOT_DIR/tests/release_bootstrap_test.bash"
bash "$ROOT_DIR/tests/updates_test.bash"
bash "$ROOT_DIR/tests/dependency_updates_test.bash"
bash "$ROOT_DIR/tests/lifecycle_e2e_test.bash"
