#!/usr/bin/env bash

print_install_help() {
  cat <<'EOF'
Usage:
  selfishell install [--profile NAME] [--skip-packages] [--dry-run] [--yes]

Options:
  --profile NAME  Select minimal or developer (default: developer)
  --skip-packages Install configuration without package operations
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

install_managed_configuration() {
  local platform="$1"
  local dry_run="$2"
  local profile="$3"
  local ghostty_enabled="${4:-0}"
  local assume_yes="${5:-0}"
  local zsh_source
  local resource_kind resource_name resource_target resource_source

  case "$platform" in
    macos) zsh_source="$SELFISHELL_ROOT/mac/.zshrc" ;;
    ubuntu | ubuntu-wsl) zsh_source="$SELFISHELL_ROOT/ubuntu/.zshrc" ;;
    *)
      cli_error "Managed installation is unavailable on $(platform_label "$platform")."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  while IFS=$'\t' read -r resource_kind resource_name resource_target resource_source; do
    case "$resource_kind" in
      file)
        if [[ "$profile" != "developer" && "$resource_name" == nvim-* ]]; then
          continue
        fi
        if [[ "$resource_name" == "zshrc-config" ]]; then
          resource_source="$zsh_source"
        fi
        if [[ "$resource_name" == ghostty-config ]]; then
          [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]] || continue
        fi
        managed_install_file "$resource_name" "$resource_source" "$resource_target" "$dry_run" "$assume_yes"
        ;;
      link)
        if [[ "$profile" != "developer" && ("$resource_name" == user-nvim || "$resource_name" == mise-config-link) ]]; then
          continue
        fi
        managed_install_link "$resource_name" "$resource_target" "$resource_source" "$dry_run"
        ;;
      block)
        if [[ "$resource_name" == user-ghostty ]]; then
          [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]] || continue
        fi
        managed_install_block "$resource_name" "$resource_target" "$dry_run" "$assume_yes"
        ;;
    esac
  done < <(selfishell_managed_resources)

  if [[ "$dry_run" == "0" ]]; then
    rm -f "$SELFISHELL_CACHE_DIR"/zoxide-init.zsh "$SELFISHELL_CACHE_DIR"/fzf-init.zsh "$SELFISHELL_CACHE_DIR"/starship-init.zsh 2>/dev/null
    selfishell_mise_trust
  fi
}

install_default_shell() {
  local dry_run="$1"
  local assume_yes="$2"
  local zsh_path
  local current_shell
  local answer
  local current_user

  zsh_path="$(command -v zsh 2>/dev/null)" || return 0
  current_shell="${SHELL:-}"
  [[ "$current_shell" == "$zsh_path" ]] && return 0

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould set login shell to:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$zsh_path"
    return
  fi

  if [[ "$assume_yes" == "1" ]]; then
    :
  elif [[ -t 0 ]]; then
    printf 'Set login shell to Zsh? [Y/n] '
    IFS= read -r answer
    case "$answer" in
      n | N | no | NO)
        return 0
        ;;
    esac
  else
    return 0
  fi

  current_user="$(id -un)"
  if chsh -s "$zsh_path" "$current_user" >/dev/null 2>&1; then
    printf '%sSet login shell to:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$zsh_path"
  else
    printf '%sCould not set login shell to Zsh.%s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET"
  fi
}

# Read-only counterpart to install_atomic_user_file_once: fails fast, before
# any package installation or managed-configuration writes, if the target
# path exists as something install_atomic_user_file_once would refuse to
# touch (anything but a regular file or symlink).
preflight_atomic_user_file_once() {
  local target_file="$1"
  local label="$2"

  if [[ -L "$target_file" || -f "$target_file" ]]; then
    return 0
  fi

  if [[ -e "$target_file" ]]; then
    cli_error "$label path is not a regular file or symlink: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi
}

preflight_mise_global_config() {
  preflight_atomic_user_file_once "${XDG_CONFIG_HOME:-$HOME/.config}/mise/config.toml" "user mise config"
}

# Atomically creates $target_file with the content read from stdin, but
# only if it doesn't already exist -- Selfishell never edits or removes it
# afterward. Used by install_mise_global_config.
install_atomic_user_file_once() {
  local dry_run="$1"
  local target_file="$2"
  local label="$3"
  local parent_dir
  local temporary_file

  if [[ -L "$target_file" || -f "$target_file" ]]; then
    if [[ "$dry_run" == "1" ]]; then
      printf '%s%s exists; preserving it:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$label" "$SELFISHELL_COLOR_RESET" "$target_file"
    fi
    return 0
  fi

  if [[ -e "$target_file" ]]; then
    cli_error "$label path is not a regular file or symlink: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould create %s:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$label" "$SELFISHELL_COLOR_RESET" "$target_file"
    return 0
  fi

  parent_dir="$(dirname "$target_file")"
  mkdir -p "$parent_dir" || return "$SELFISHELL_EXIT_ERROR"

  temporary_file="$(mktemp "${target_file}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"

  if ! cat >"$temporary_file"; then
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  if ln "$temporary_file" "$target_file" 2>/dev/null && [[ -f "$target_file" ]]; then
    rm -f "$temporary_file"
    printf '%sCreated %s:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$label" "$SELFISHELL_COLOR_RESET" "$target_file"
    return 0
  fi

  rm -f "$temporary_file"

  if [[ -L "$target_file" || -f "$target_file" ]]; then
    printf '%s%s appeared concurrently; preserving it:%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$label" "$SELFISHELL_COLOR_RESET" "$target_file"
    return 0
  fi

  cli_error "Failed to create $label: $target_file"
  return "$SELFISHELL_EXIT_ERROR"
}

install_mise_global_config() {
  install_atomic_user_file_once "$1" "${XDG_CONFIG_HOME:-$HOME/.config}/mise/config.toml" "user mise config" \
    </dev/null
}

command_install() {
  local assume_yes=0
  local dry_run=0
  local profile=developer
  local skip_packages=0
  local platform
  local ghostty_enabled=0
  local ghostty_answer

  while (("$#" > 0)); do
    case "$1" in
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
      --skip-packages) skip_packages=1 ;;
      --profile)
        shift
        if (("$#" == 0)); then
          cli_error "--profile requires a value"
          return "$SELFISHELL_EXIT_USAGE"
        fi
        profile="$1"
        ;;
      help | --help | -h)
        print_install_help
        return
        ;;
      *)
        cli_error "Unknown install option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done

  platform="$(detect_platform)"
  if ! platform_is_supported "$platform"; then
    cli_error "Managed installation is unavailable on $(platform_label "$platform")."
    return "$SELFISHELL_EXIT_ERROR"
  fi

  confirm_action "Install Selfishell configuration?" "$assume_yes" "$dry_run" || return
  selfishell_initialize_paths
  managed_preflight_zsh_loader "$assume_yes" "$dry_run" || return
  managed_preflight_block_target user-zprofile "$HOME/.zprofile" "$assume_yes" "$dry_run" || return
  profile_load "$profile"
  if [[ "$profile" == "developer" ]]; then
    preflight_mise_global_config || return
  fi

  if [[ "$platform" == "macos" ]]; then
    if [[ -r "$SELFISHELL_STATE_DIR/ghostty" ]]; then
      ghostty_enabled="$(<"$SELFISHELL_STATE_DIR/ghostty")"
      [[ "$ghostty_enabled" == "1" ]] || ghostty_enabled=0
    elif [[ "$assume_yes" == "1" || "$dry_run" == "1" ]]; then
      ghostty_enabled=1
    elif [[ -t 0 ]]; then
      printf 'Install Ghostty terminal and managed configuration? [y/N] '
      IFS= read -r ghostty_answer
      case "$ghostty_answer" in y | Y | yes | YES) ghostty_enabled=1 ;; esac
    fi
  fi

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_preflight_block_target user-ghostty \
      "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty" "$assume_yes" "$dry_run" || return
  fi

  if [[ "${SELFISHELL_OFFLINE:-0}" == "1" ]]; then
    skip_packages=1
  fi

  if [[ "$skip_packages" == "1" ]]; then
    printf 'Skipping package installation.\n'
  else
    packages_install_profile "$platform" "$dry_run"
    if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
      homebrew_install_packages optional cask "$dry_run" ghostty
    fi
  fi

  install_managed_configuration "$platform" "$dry_run" "$profile" "$ghostty_enabled" "$assume_yes"
  if [[ "$profile" == "developer" ]]; then
    install_mise_global_config "$dry_run" || return
  fi
  if [[ "$skip_packages" == "0" && "$profile" == "developer" ]]; then
    install_neovim_plugins "$dry_run" || return
  fi
  install_default_shell "$dry_run" "$assume_yes"

  if [[ "$dry_run" == "0" ]]; then
    local profile_state
    local temporary_profile_state
    local ghostty_state
    local temporary_ghostty_state
    mkdir -p "$SELFISHELL_STATE_DIR" || return "$SELFISHELL_EXIT_ERROR"

    profile_state="$SELFISHELL_STATE_DIR/profile"
    temporary_profile_state="$(mktemp "${profile_state}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"
    printf '%s\n' "$profile" >"$temporary_profile_state" || {
      rm -f "$temporary_profile_state"
      return "$SELFISHELL_EXIT_ERROR"
    }
    mv "$temporary_profile_state" "$profile_state" || {
      rm -f "$temporary_profile_state"
      return "$SELFISHELL_EXIT_ERROR"
    }

    ghostty_state="$SELFISHELL_STATE_DIR/ghostty"
    temporary_ghostty_state="$(mktemp "${ghostty_state}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"
    printf '%s\n' "$ghostty_enabled" >"$temporary_ghostty_state" || {
      rm -f "$temporary_ghostty_state"
      return "$SELFISHELL_EXIT_ERROR"
    }
    mv "$temporary_ghostty_state" "$ghostty_state" || {
      rm -f "$temporary_ghostty_state"
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf '%sDry run complete; no files were changed.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
  else
    printf '%sSelfishell configuration installed.%s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET"
  fi
}
