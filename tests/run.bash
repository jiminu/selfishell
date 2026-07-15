#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bash "$ROOT_DIR/tests/common_test.bash"
bash "$ROOT_DIR/tests/ubuntu_packages_test.bash"
