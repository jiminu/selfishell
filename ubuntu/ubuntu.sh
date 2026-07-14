#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHELL_DIR="${SHELL_DIR:-$ROOT_DIR/ubuntu}"
COMMON_DIR="${COMMON_DIR:-$ROOT_DIR/common}"
ZSH_SOURCE="${ZSH_SOURCE:-$SHELL_DIR/.zshrc}"

source "$ROOT_DIR/common/common.sh"

is_ubuntu_wsl() (
  grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null || return 1
  [[ -r /etc/os-release ]] || return 1
  source /etc/os-release
  [[ "${ID:-}" == "ubuntu" ]]
)

apt_install_packages() {
  if ! have apt-get; then
    warn "Skipping because apt-get was not found"
    return
  fi

  local package
  local install_packages=()

  for package in "$@"; do
    if dpkg -s "$package" >/dev/null 2>&1; then
      warn "Already installed: $package"
      continue
    fi

    if apt-cache show "$package" >/dev/null 2>&1; then
      install_packages+=("$package")
    else
      warn "Skipping unavailable apt package: $package"
    fi
  done

  if (( ${#install_packages[@]} == 0 )); then
    return
  fi

  log "Installing apt packages"
  sudo apt-get update
  sudo apt-get install -y "${install_packages[@]}"
}

install_starship() {
  if have starship || [[ -x "$HOME/.local/bin/starship" ]]; then
    return
  fi

  log "Installing starship"
  mkdir -p "$HOME/.local/bin"
  curl -fsSL https://starship.rs/install.sh | sh -s -- -y -b "$HOME/.local/bin"
}

install_nvm() {
  local nvm_dir="$HOME/.nvm"

  if [[ -s "$nvm_dir/nvm.sh" ]]; then
    return
  fi

  log "Installing nvm"
  PROFILE=/dev/null bash -c "$(curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh)"
}

install_pyenv() {
  local pyenv_dir="$HOME/.pyenv"

  if [[ -d "$pyenv_dir" ]]; then
    return
  fi

  log "Installing pyenv"
  git clone https://github.com/pyenv/pyenv.git "$pyenv_dir"
}

set_default_shell() {
  if ! have zsh; then
    warn "Skipping because zsh was not found"
    return
  fi

  local zsh_path
  zsh_path="$(command -v zsh)"

  local current_shell
  current_shell="$(getent passwd "$USER" | cut -d: -f7)"

  if [[ "$current_shell" == "$zsh_path" ]]; then
    warn "zsh is already the default shell"
    return
  fi

  if ! grep -qxF "$zsh_path" /etc/shells 2>/dev/null; then
    warn "Skipping because zsh is not listed in /etc/shells"
    return
  fi

  log "Setting zsh as the default shell"
  chsh -s "$zsh_path" "$USER"
}

install_shell_configs() {
  log "Linking shell configuration"
  mkdir -p "$HOME/.nvm"
  link_file "$COMMON_DIR/common.zsh" "$HOME/.config/zsh/common.zsh"
  link_file "$ZSH_SOURCE" "$HOME/.zshrc"
  link_file "$COMMON_DIR/starship.toml" "$HOME/.config/starship.toml"
  link_file "$COMMON_DIR/.vimrc" "$HOME/.vimrc"
}

main() {
  if ! is_ubuntu_wsl; then
    printf 'This script supports Ubuntu on WSL only.\n' >&2
    exit 1
  fi

  apt_install_packages \
    zsh \
    git \
    curl \
    unzip \
    build-essential \
    ca-certificates \
    fzf \
    zoxide \
    eza \
    bat \
    vim

  install_starship
  install_nvm
  install_pyenv
  install_zinit
  install_vundle
  install_shell_configs
  set_default_shell
  install_vim_plugins

  log "Setup complete"
  warn "Open a new terminal, or run 'source ~/.zshrc' if needed."
}

main "$@"
