#!/usr/bin/env bash

install_direct_package() {
  local requirement="$1"
  local package="$2"
  local dry_run="$3"

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s direct package: %s\n' "$requirement" "$package"
    return
  fi

  case "$package" in
    starship | mise | zinit | neovim)
      local dependency_platform
      case "$(detect_platform)" in ubuntu | ubuntu-wsl) dependency_platform=linux ;; *) dependency_platform="$(detect_platform)" ;; esac
      dependency_install "$package" "$dependency_platform" "$(detect_architecture)"
      ;;
    *)
      cli_error "Unknown direct package: $package"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$package")
        return 0
      fi
      return 1
      ;;
  esac
}

install_mise_tools() {
  local requirement="$1"
  local dry_run="$2"
  local mise_command
  shift 2

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s mise tools: %s\n' "$requirement" "$*"
    return
  fi
  if have_command mise; then
    mise_command="$(command -v mise)"
  elif [[ -x "$HOME/.local/bin/mise" ]]; then
    mise_command="$HOME/.local/bin/mise"
  else
    cli_error "mise is required to install mise-managed tools."
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi

  if ! MISE_GLOBAL_CONFIG_FILE="$SELFISHELL_ROOT/common/mise.toml" "$mise_command" install "$@"; then
    cli_error "Could not install $requirement mise tools: $*"
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi
}

install_vim_plugins() {
  local dry_run="$1"
  local nvim_command
  local lazypath
  local treesitter_languages

  lazypath="${XDG_DATA_HOME:-$HOME/.local/share}/nvim/lazy/lazy.nvim"

  [[ "${SELFISHELL_OFFLINE:-0}" != "1" ]] || return 0
  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install declared Neovim plugins.\n'
    printf 'Would install lazy.nvim bootstrap repository.\n'
    printf 'Would install Tree-sitter parsers.\n'
    return
  fi
  if [[ ! -d "$lazypath" ]]; then
    command mkdir -p "$(dirname "$lazypath")"
    git clone --filter=blob:none --branch=stable https://github.com/folke/lazy.nvim.git "$lazypath" >/dev/null 2>&1 || {
      cli_error "Could not install lazy.nvim."
      return 1
    }
  fi
  if have_command nvim; then
    nvim_command="nvim"
  elif [[ -x "$HOME/.local/bin/nvim" ]]; then
    nvim_command="$HOME/.local/bin/nvim"
  else
    return 0
  fi

  if ! "$nvim_command" --headless "+Lazy! sync" +qa >/dev/null 2>&1; then
    cli_error "Could not install Neovim plugins."
    return 1
  fi

  treesitter_languages="$(selfishell_nvim_treesitter_languages)"
  if [[ -n "$treesitter_languages" ]] &&
    ! "$nvim_command" --headless "+TSInstallSync $treesitter_languages" +qa >/dev/null 2>&1; then
    cli_error "Could not install Tree-sitter parsers."
    return 1
  fi
}

selfishell_nvim_treesitter_languages() {
  printf '%s\n' \
    'lua vim vimdoc query c cpp python java bash zsh javascript typescript tsx html css json jsonc yaml toml properties xml dockerfile hcl terraform helm markdown markdown_inline sql'
}

migrate_legacy_neovim_installation() {
  local dry_run="$1"
  local legacy_nvim="$HOME/.local/bin/nvim"
  local legacy_state="$SELFISHELL_STATE_DIR/dependencies/neovim"

  [[ -r "$legacy_state" && -e "$legacy_nvim" ]] || return 0

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would remove legacy Neovim installation: %s\n' "$legacy_nvim"
    return 0
  fi

  rm -f "$legacy_nvim"
  rm -f "$legacy_state"
  printf 'Removed legacy Neovim installation: %s\n' "$legacy_nvim"
}
