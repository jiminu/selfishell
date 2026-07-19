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
    starship | mise | zinit)
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

selfishell_nvim_plugin_record() {
  local repository="$1"
  local manifest

  manifest="$(dependencies_manifest_path)"
  awk -v repository="$repository" '
    $1 == "nvim-plugin" && $2 == repository { print $3, $6; found = 1; exit }
    END { exit(found ? 0 : 1) }
  ' "$manifest"
}

install_lazy_nvim() {
  local lazypath="$1"
  local record revision source current_revision temporary previous

  record="$(selfishell_nvim_plugin_record folke/lazy.nvim)" || {
    cli_error "No approved lazy.nvim revision is declared."
    return 1
  }
  revision="${record%% *}"
  source="${record#* }"
  [[ "$revision" =~ ^[0-9a-f]{40}$ ]] || {
    cli_error "Invalid approved lazy.nvim revision: $revision"
    return 1
  }

  if [[ -d "$lazypath/.git" ]]; then
    if [[ -n "$(git -C "$lazypath" status --porcelain)" ]]; then
      cli_error "lazy.nvim checkout was modified; preserving it: $lazypath"
      return 1
    fi
    current_revision="$(git -C "$lazypath" rev-parse HEAD)"
    [[ "$current_revision" != "$revision" ]] || return 0
  elif [[ -e "$lazypath" || -L "$lazypath" ]]; then
    cli_error "lazy.nvim path is not an approved Git checkout; preserving it: $lazypath"
    return 1
  fi

  temporary="${lazypath}.tmp.$$"
  previous="${lazypath}.previous.$$"
  [[ ! -e "$temporary" && ! -e "$previous" ]] || {
    cli_error "Temporary lazy.nvim path already exists."
    return 1
  }
  mkdir -p "$(dirname "$lazypath")"
  git clone --quiet --filter=blob:none "$source" "$temporary" || return
  git -C "$temporary" checkout --quiet --detach "$revision" || {
    rm -rf "$temporary"
    return 1
  }
  if [[ -e "$lazypath" ]]; then
    mv "$lazypath" "$previous" || return
  fi
  if ! mv "$temporary" "$lazypath"; then
    [[ ! -e "$previous" ]] || mv "$previous" "$lazypath"
    return 1
  fi
  rm -rf "$previous"
  printf 'Installed approved lazy.nvim revision: %s\n' "$revision"
}

install_neovim_plugins() {
  local dry_run="$1"
  local log_file
  local nvim_command
  local lazypath
  local treesitter_languages

  lazypath="${XDG_DATA_HOME:-$HOME/.local/share}/selfishell/nvim/lazy/lazy.nvim"

  [[ "${SELFISHELL_OFFLINE:-0}" != "1" ]] || return 0
  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install declared Neovim plugins.\n'
    printf 'Would install lazy.nvim bootstrap repository.\n'
    printf 'Would install Tree-sitter parsers.\n'
    return
  fi

  if ! nvim_command="$(selfishell_nvim_command)"; then
    cli_error "Could not locate Neovim after installing the developer profile."
    return 1
  fi

  install_lazy_nvim "$lazypath" || return
  log_file="$(mktemp "${TMPDIR:-/tmp}/selfishell-nvim.XXXXXX")" || return 1

  if ! "$nvim_command" --headless \
    '+lua local ok, message = pcall(vim.cmd, "Lazy! sync"); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end' \
    +qa >"$log_file" 2>&1; then
    cat "$log_file" >&2
    rm -f "$log_file"
    cli_error "Could not install Neovim plugins."
    return 1
  fi

  treesitter_languages="$(selfishell_nvim_treesitter_languages)"
  if [[ -n "$treesitter_languages" ]] &&
    ! SELFISHELL_NVIM_TREESITTER_LANGUAGES="$treesitter_languages" "$nvim_command" --headless \
      '+lua local ok, message = xpcall(function() local languages = vim.split(vim.env.SELFISHELL_NVIM_TREESITTER_LANGUAGES, "%s+", { trimempty = true }); require("nvim-treesitter").install(languages):wait(300000) end, debug.traceback); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end' \
      +qa >"$log_file" 2>&1; then
    cat "$log_file" >&2
    rm -f "$log_file"
    cli_error "Could not install Tree-sitter parsers."
    return 1
  fi
  rm -f "$log_file"
}

selfishell_nvim_treesitter_languages() {
  printf '%s\n' \
    'lua vim vimdoc query c cpp python java bash zsh javascript typescript tsx html css json yaml toml properties xml dockerfile hcl terraform markdown sql gitcommit git_rebase git_config gitignore gitattributes diff'
}

selfishell_nvim_command() {
  local mise_command=""
  local resolved

  if have_command mise; then
    mise_command="$(command -v mise)"
  elif [[ -x "$HOME/.local/bin/mise" ]]; then
    mise_command="$HOME/.local/bin/mise"
  fi

  if [[ -n "$mise_command" ]]; then
    resolved="$(MISE_GLOBAL_CONFIG_FILE="$SELFISHELL_ROOT/common/mise.toml" \
      "$mise_command" which nvim 2>/dev/null)" || true
    if [[ -n "$resolved" && -x "$resolved" ]]; then
      printf '%s\n' "$resolved"
      return 0
    fi
  fi

  if have_command nvim; then
    printf '%s\n' "$(command -v nvim)"
    return 0
  fi

  if [[ -x "$HOME/.local/bin/nvim" ]]; then
    printf '%s\n' "$HOME/.local/bin/nvim"
    return 0
  fi

  return 1
}
