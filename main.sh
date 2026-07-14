#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "$(uname -s)" in
  Darwin)
    exec bash "$ROOT_DIR/mac/mac.sh" "$@"
    ;;

  Linux)
    if grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
      if [[ -r /etc/os-release ]]; then
        source /etc/os-release
        if [[ "${ID:-}" == "ubuntu" ]]; then
          exec bash "$ROOT_DIR/ubuntu/ubuntu.sh" "$@"
        fi
      fi

      printf 'Unsupported WSL distribution. Only Ubuntu is currently supported.\n' >&2
      exit 1
    fi

    printf 'Unsupported Linux environment.\n' >&2
    exit 1
    ;;

  *)
    printf 'Unsupported operating system: %s\n' "$(uname -s)" >&2
    exit 1
    ;;
esac
