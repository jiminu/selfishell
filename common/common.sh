#!/usr/bin/env bash

# Functions shared by the macOS and Ubuntu bootstrap scripts

log() {
  printf '\n==> %s\n' "$*"
}

warn() {
  printf '    %s\n' "$*"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

backup_path() {
  local target="$1"

  if [[ ! -e "$target" && ! -L "$target" ]]; then
    return
  fi

  local backup_base
  backup_base="${target}.backup.$(date +%Y%m%d%H%M%S)"
  local backup="$backup_base"
  local suffix=0

  while [[ -e "$backup" || -L "$backup" ]]; do
    suffix=$((suffix + 1))
    backup="${backup_base}.${suffix}"
  done

  warn "Backing up existing file: $target -> $backup"
  mv "$target" "$backup"
}

link_file() {
  local source_file="$1"
  local target_file="$2"

  if [[ ! -e "$source_file" ]]; then
    warn "Skipping missing source file: $source_file"
    return
  fi

  mkdir -p "$(dirname "$target_file")"

  if [[ -L "$target_file" ]]; then
    local current_target
    current_target="$(readlink "$target_file")"
    if [[ "$current_target" == "$source_file" ]]; then
      warn "Already linked: $target_file"
      return
    fi
    backup_path "$target_file"
  elif [[ -e "$target_file" ]]; then
    backup_path "$target_file"
  fi

  ln -s "$source_file" "$target_file"
  warn "Linked: $target_file -> $source_file"
}

install_zinit() {
  local zinit_home="$HOME/.local/share/zinit/zinit.git"

  if [[ -f "$zinit_home/zinit.zsh" ]]; then
    return
  fi

  log "Installing zinit"
  mkdir -p "$(dirname "$zinit_home")"
  git clone https://github.com/zdharma-continuum/zinit.git "$zinit_home"
}

install_kubectl_completion() {
  if ! have kubectl; then
    return
  fi

  local completion_dir="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell/completions"
  local completion_file="$completion_dir/_kubectl"
  local temporary_file="${completion_file}.tmp"

  mkdir -p "$completion_dir"
  if kubectl completion zsh >"$temporary_file"; then
    mv "$temporary_file" "$completion_file"
    warn "Generated kubectl completion: $completion_file"
  else
    rm -f "$temporary_file"
    warn "Could not generate kubectl completion"
  fi
}

install_vundle() {
  local vundle_dir="$HOME/.vim/bundle/Vundle.vim"

  if [[ -d "$vundle_dir" ]]; then
    return
  fi

  log "Installing Vundle"
  mkdir -p "$(dirname "$vundle_dir")"
  git clone https://github.com/VundleVim/Vundle.vim.git "$vundle_dir"
}

install_vim_plugins() {
  if ! have vim; then
    warn "Skipping because vim was not found"
    return
  fi

  log "Installing Vim plugins"
  vim +PluginInstall +qall
}
