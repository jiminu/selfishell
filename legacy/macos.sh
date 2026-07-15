#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHELL_DIR="${SHELL_DIR:-$ROOT_DIR/mac}"
COMMON_DIR="${COMMON_DIR:-$ROOT_DIR/common}"
ZSH_SOURCE="${ZSH_SOURCE:-$SHELL_DIR/.zshrc}"

source "$ROOT_DIR/legacy/common.sh"

brew_shellenv() {
  if [[ -x /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
    return
  fi

  if [[ -x /usr/local/bin/brew ]]; then
    eval "$(/usr/local/bin/brew shellenv)"
  fi
}

install_homebrew() {
  if have brew; then
    return
  fi

  log "Installing Homebrew"
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  brew_shellenv
}

brew_install_formulae() {
  local formula
  for formula in "$@"; do
    if brew list --formula "$formula" >/dev/null 2>&1; then
      warn "Already installed: $formula"
      continue
    fi
    log "Installing: $formula"
    brew install "$formula"
  done
}

brew_install_casks() {
  local cask
  for cask in "$@"; do
    if brew list --cask "$cask" >/dev/null 2>&1; then
      warn "Already installed: $cask"
      continue
    fi
    log "Installing: $cask"
    brew install --cask "$cask"
  done
}

install_shell_configs() {
  log "Linking shell configuration"
  mkdir -p "$HOME/.nvm"
  link_file "$COMMON_DIR/common.zsh" "$HOME/.config/zsh/common.zsh"
  link_file "$ZSH_SOURCE" "$HOME/.zshrc"
  link_file "$COMMON_DIR/starship.toml" "$HOME/.config/starship.toml"
  link_file "$SHELL_DIR/config.ghostty" "$HOME/.config/ghostty/config"
  link_file "$COMMON_DIR/.vimrc" "$HOME/.vimrc"
}

main() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    printf 'This script supports macOS only.\n' >&2
    exit 1
  fi

  install_homebrew

  brew_install_formulae \
    git \
    starship \
    zoxide \
    fzf \
    eza \
    bat \
    pyenv \
    nvm \
    kubectl \
    kubectx \
    openjdk@17

  brew_install_casks ghostty
  brew_install_casks \
    font-meslo-lg-nerd-font \
    font-noto-sans-cjk-kr

  install_zinit
  install_kubectl_completion
  install_vundle
  install_shell_configs
  install_vim_plugins

  log "Setup complete"
  warn "Open a new terminal, or run 'source ~/.zshrc' if needed."
  warn "Restart Ghostty to apply its configuration."
}

main "$@"
