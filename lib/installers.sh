#!/usr/bin/env bash

install_direct_package() {
  local requirement="$1"
  local package="$2"
  local dry_run="$3"

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould install %s direct package:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$requirement" "$SELFISHELL_COLOR_RESET" "$package"
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
    printf '%sWould install %s mise tools:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$requirement" "$SELFISHELL_COLOR_RESET" "$*"
    return
  fi
  if ! mise_command="$(selfishell_mise_command)"; then
    cli_error "mise is required to install mise-managed tools."
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi

  selfishell_mise_trust

  if ! MISE_GLOBAL_CONFIG_FILE="$SELFISHELL_ROOT/common/mise.toml" "$mise_command" install "$@"; then
    cli_error "Could not install $requirement mise tools: $*"
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi
}

selfishell_mise_command() {
  if have_command mise; then
    command -v mise
  elif [[ -x "$HOME/.local/bin/mise" ]]; then
    printf '%s\n' "$HOME/.local/bin/mise"
  else
    return 1
  fi
}

selfishell_mise_trust() {
  local mise_command
  if mise_command="$(selfishell_mise_command)"; then
    local config_link="${XDG_CONFIG_HOME:-$HOME/.config}/mise/conf.d/selfishell.toml"
    if [[ -L "$config_link" || -f "$config_link" ]]; then
      "$mise_command" trust "$config_link" >/dev/null 2>&1 || true
    fi
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
  printf '%sInstalled approved lazy.nvim revision:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$revision"
}

verify_neovim_plugins() {
  local data_home
  local manifest
  local plugin_dir
  local plugin_name
  local repository
  local revision
  local source
  local type
  local current_revision

  data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  manifest="$(dependencies_manifest_path)"

  while read -r type repository revision _ _ source _; do
    [[ "$type" == "nvim-plugin" ]] || continue

    if [[ "$repository" == "folke/lazy.nvim" ]]; then
      plugin_dir="$data_home/selfishell/nvim/lazy/lazy.nvim"
    else
      plugin_name="${source##*/}"
      plugin_name="${plugin_name%.git}"
      plugin_dir="$data_home/nvim/lazy/$plugin_name"
    fi

    if [[ ! -d "$plugin_dir/.git" ]]; then
      cli_error "Neovim plugin checkout is missing after sync: $repository"
      return 1
    fi
    current_revision="$(git -C "$plugin_dir" rev-parse HEAD 2>/dev/null)" || {
      cli_error "Could not inspect Neovim plugin after sync: $repository"
      return 1
    }
    if [[ "$current_revision" != "$revision" ]]; then
      cli_error "Neovim plugin revision does not match after sync: $repository"
      return 1
    fi
  done <"$manifest"
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
    printf '%sWould install declared Neovim plugins.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
    printf '%sWould install lazy.nvim bootstrap repository.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
    printf '%sWould install Tree-sitter parsers.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
    return
  fi

  if ! nvim_command="$(selfishell_nvim_command)"; then
    cli_error "Could not locate Neovim after installing the developer profile."
    return 1
  fi

  install_lazy_nvim "$lazypath" || return
  log_file="$(mktemp "${TMPDIR:-/tmp}/selfishell-nvim.XXXXXX")" || return 1

  if ! selfishell_run_nvim "$nvim_command" --headless \
    '+lua local ok, message = pcall(vim.cmd, "Lazy! sync"); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end' \
    +qa >"$log_file" 2>&1; then
    cat "$log_file" >&2
    rm -f "$log_file"
    cli_error "Could not install Neovim plugins."
    return 1
  fi
  if ! verify_neovim_plugins; then
    cat "$log_file" >&2
    rm -f "$log_file"
    return 1
  fi

  treesitter_languages="$(selfishell_nvim_treesitter_languages)"
  if [[ -n "$treesitter_languages" ]] &&
    ! SELFISHELL_NVIM_TREESITTER_LANGUAGES="$treesitter_languages" selfishell_run_nvim "$nvim_command" --headless \
      '+lua local ok, message = xpcall(function() local languages = vim.split(vim.env.SELFISHELL_NVIM_TREESITTER_LANGUAGES, "%s+", { trimempty = true }); require("config.treesitter").install(languages) end, debug.traceback); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end' \
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

  mise_command="$(selfishell_mise_command)" || mise_command=""

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

selfishell_run_nvim() {
  local nvim_command="$1"
  local mise_command=""
  shift

  mise_command="$(selfishell_mise_command)" || mise_command=""
  if [[ -n "$mise_command" ]]; then
    MISE_GLOBAL_CONFIG_FILE="$SELFISHELL_ROOT/common/mise.toml" \
      "$mise_command" exec -- "$nvim_command" "$@"
  else
    "$nvim_command" "$@"
  fi
}
