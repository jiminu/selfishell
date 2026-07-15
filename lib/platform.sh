#!/usr/bin/env bash

source "$SELFISHELL_ROOT/lib/platforms/macos.sh"
source "$SELFISHELL_ROOT/lib/platforms/ubuntu.sh"

platform_system_name() {
  if [[ -n "${SELFISHELL_TEST_SYSTEM_NAME:-}" ]]; then
    printf '%s\n' "$SELFISHELL_TEST_SYSTEM_NAME"
  else
    uname -s
  fi
}

platform_machine_arch() {
  if [[ -n "${SELFISHELL_TEST_MACHINE_ARCH:-}" ]]; then
    printf '%s\n' "$SELFISHELL_TEST_MACHINE_ARCH"
  else
    uname -m
  fi
}

platform_os_release_file() {
  printf '%s\n' "${SELFISHELL_TEST_OS_RELEASE_FILE:-/etc/os-release}"
}

platform_proc_version_file() {
  printf '%s\n' "${SELFISHELL_TEST_PROC_VERSION_FILE:-/proc/version}"
}

platform_linux_distribution() {
  local os_release
  local key
  local value
  os_release="$(platform_os_release_file)"

  if [[ ! -r "$os_release" ]]; then
    printf 'unknown\n'
    return
  fi

  while IFS='=' read -r key value; do
    if [[ "$key" == "ID" ]]; then
      value="${value#\"}"
      value="${value%\"}"
      value="${value#\'}"
      value="${value%\'}"
      printf '%s\n' "${value:-unknown}"
      return
    fi
  done <"$os_release"

  printf 'unknown\n'
}

platform_is_wsl() {
  local proc_version
  proc_version="$(platform_proc_version_file)"

  [[ -r "$proc_version" ]] || return 1
  grep -qiE 'microsoft|wsl' "$proc_version"
}

detect_platform() {
  local system_name
  system_name="$(platform_system_name)"

  case "$system_name" in
    Darwin)
      printf 'macos\n'
      ;;
    Linux)
      case "$(platform_linux_distribution)" in
        ubuntu)
          if platform_is_wsl; then
            printf 'ubuntu-wsl\n'
          else
            printf 'ubuntu\n'
          fi
          ;;
        *)
          if platform_is_wsl; then
            printf 'unsupported-wsl\n'
          else
            printf 'unsupported-linux\n'
          fi
          ;;
      esac
      ;;
    *)
      printf 'unsupported\n'
      ;;
  esac
}

detect_architecture() {
  case "$(platform_machine_arch)" in
    arm64 | aarch64)
      printf 'arm64\n'
      ;;
    x86_64 | amd64)
      printf 'amd64\n'
      ;;
    *)
      platform_machine_arch
      ;;
  esac
}

platform_label() {
  case "$1" in
    macos) printf 'macOS\n' ;;
    ubuntu) printf 'Ubuntu\n' ;;
    ubuntu-wsl) printf 'Ubuntu on WSL\n' ;;
    unsupported-wsl) printf 'Unsupported WSL distribution\n' ;;
    unsupported-linux) printf 'Unsupported Linux distribution\n' ;;
    *) printf 'Unsupported operating system\n' ;;
  esac
}

platform_is_supported() {
  case "$1" in
    macos | ubuntu | ubuntu-wsl) return 0 ;;
    *) return 1 ;;
  esac
}

platform_package_manager() {
  case "$1" in
    macos) macos_package_manager ;;
    ubuntu | ubuntu-wsl) ubuntu_package_manager ;;
    *) printf 'unknown\n' ;;
  esac
}
