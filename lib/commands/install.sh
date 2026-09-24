#!/usr/bin/env bash

print_install_help() {
  cat <<'EOF'
Usage:
  selfishell install [--skip-packages] [--dry-run] [--yes]

Options:
  --skip-packages Skip package and tool installation and apply managed configuration only
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

# preflight=1 checks every managed file and link, asking conflict questions
# up front, so a conflict stops the command before packages or any file change.
install_managed_configuration() {
  local platform="$1"
  local dry_run="$2"
  local ghostty_enabled="${3:-0}"
  local assume_yes="${4:-0}"
  local preflight="${5:-0}"
  local zsh_source
  local resource_kind resource_name resource_target resource_source

  case "$platform" in
    macos) zsh_source="$SELFISHELL_ROOT/config/macos/zshrc" ;;
    ubuntu | ubuntu-wsl) zsh_source="$SELFISHELL_ROOT/config/ubuntu/zshrc" ;;
    *)
      cli_error "Managed installation is unavailable on $(platform_label "$platform")."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  while IFS=$'\t' read -r resource_kind resource_name resource_target resource_source; do
    case "$resource_kind" in
      file)
        if [[ "$resource_name" == "zshrc-config" ]]; then
          resource_source="$zsh_source"
        fi
        if [[ "$resource_name" == ghostty-config ]]; then
          [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]] || continue
        fi
        # Invalidate before replacing the generator so an interrupted install
        # cannot leave new configuration paired with old initialization code.
        if [[ "$dry_run" == "0" && "$preflight" == 0 && "$resource_name" == zsh-interactive ]] &&
          ! cmp -s "$resource_source" "$resource_target"; then
          rm -f "$SELFISHELL_CACHE_DIR"/zoxide-init.zsh "$SELFISHELL_CACHE_DIR"/fzf-init.zsh "$SELFISHELL_CACHE_DIR"/starship-init.zsh 2>/dev/null
        fi
        managed_install_file "$resource_name" "$resource_source" "$resource_target" "$dry_run" "$assume_yes" "$preflight"
        ;;
      link)
        managed_install_link "$resource_name" "$resource_target" "$resource_source" "$dry_run" "$preflight"
        ;;
      block)
        # Blocks have their own preflight, which also runs before packages.
        [[ "$preflight" == 0 ]] || continue
        if [[ "$resource_name" == user-ghostty ]]; then
          [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]] || continue
        fi
        if [[ "$resource_name" == user-zshenv ]]; then
          case "$platform" in
            ubuntu | ubuntu-wsl) ;;
            *) continue ;;
          esac
        fi
        managed_install_block "$resource_name" "$resource_target" "$dry_run" "$assume_yes"
        ;;
    esac
  done < <(selfishell_managed_resources)

  if [[ "$dry_run" == "0" && "$preflight" == 0 ]]; then
    selfishell_mise_trust
  fi
}

# chsh refuses unlisted shells for non-root users, so a PATH zsh such as
# Homebrew's is not a valid target.
install_listed_zsh() {
  local shells_file="${SELFISHELL_TEST_SHELLS_FILE:-/etc/shells}"
  local entry

  [[ -r "$shells_file" ]] || return 1
  while IFS= read -r entry; do
    if [[ "$entry" == /* && "${entry##*/}" == zsh && -x "$entry" ]]; then
      printf '%s\n' "$entry"
      return 0
    fi
  done <"$shells_file"
  return 1
}

# chsh prompts for a password on the terminal; without one it would wait on
# an invisible prompt. A piped `install.sh --setup --yes` still has /dev/tty.
install_login_shell_terminal() {
  local terminal="${SELFISHELL_TEST_TERMINAL:-/dev/tty}"

  { : <"$terminal"; } 2>/dev/null || return 1
  printf '%s\n' "$terminal"
}

install_default_shell() {
  local dry_run="$1"
  local assume_yes="$2"
  local zsh_path
  local terminal
  local answer=""
  local current_user

  # Any zsh counts: Homebrew's zsh ahead of /bin/zsh on PATH is no reason to switch.
  [[ "${SHELL##*/}" == zsh ]] && return 0
  if ! zsh_path="$(install_listed_zsh)"; then
    if have_command zsh; then
      printf '%sZsh is not listed in /etc/shells; the login shell was not changed.%s\n' \
        "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET"
    fi
    return 0
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould set login shell to:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$zsh_path"
    return
  fi

  if [[ "$assume_yes" != "1" ]]; then
    selfishell_is_interactive || return 0
    printf 'Set login shell to Zsh? [Y/n] '
    IFS= read -r answer <&3 || answer=""
    selfishell_answer_is_no "$answer" && return 0
  fi

  if ! terminal="$(install_login_shell_terminal)"; then
    printf '%sTo use Zsh as your login shell, run:%s chsh -s %s\n' \
      "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$zsh_path"
    return 0
  fi

  current_user="$(id -un)"
  if chsh -s "$zsh_path" "$current_user" <"$terminal"; then
    printf '%sSet login shell to:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$zsh_path"
  else
    printf '%sCould not set login shell to Zsh.%s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET"
  fi
}

# Read-only counterpart to install_atomic_user_file_once: fails before any
# package install or configuration write if the target is something that
# function would refuse to touch (anything but a regular file or symlink).
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
  local skip_packages=0
  local platform
  local ghostty_enabled=0
  local ghostty_answer

  SELFISHELL_UNCHANGED_COUNT=0

  while (("$#" > 0)); do
    case "$1" in
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
      --skip-packages) skip_packages=1 ;;
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

  package_manifest_load || return
  platform="$(detect_platform)"
  if ! platform_is_supported "$platform"; then
    cli_error "Managed installation is unavailable on $(platform_label "$platform")."
    return "$SELFISHELL_EXIT_ERROR"
  fi

  confirm_action "Install Selfishell configuration?" "$assume_yes" "$dry_run" || return
  selfishell_initialize_paths
  managed_preflight_zsh_loader "$assume_yes" "$dry_run" || return
  managed_preflight_block_target user-zprofile "$HOME/.zprofile" "$assume_yes" "$dry_run" || return
  case "$platform" in
    ubuntu | ubuntu-wsl)
      managed_preflight_block_target user-zshenv "$HOME/.zshenv" "$assume_yes" "$dry_run" || return
      ;;
  esac
  managed_preflight_block_target user-vimrc "$HOME/.vimrc" "$assume_yes" "$dry_run" || return
  preflight_mise_global_config || return

  if [[ "$platform" == "macos" ]]; then
    if [[ -r "$SELFISHELL_STATE_DIR/ghostty" ]]; then
      ghostty_enabled="$(<"$SELFISHELL_STATE_DIR/ghostty")"
      [[ "$ghostty_enabled" == "1" ]] || ghostty_enabled=0
    elif [[ "$assume_yes" == "1" || "$dry_run" == "1" ]]; then
      ghostty_enabled=1
    elif [[ -t 0 ]]; then
      printf 'Install Ghostty terminal and managed configuration? [y/N] '
      IFS= read -r ghostty_answer
      selfishell_answer_is_yes "$ghostty_answer" && ghostty_enabled=1
    fi
  fi

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_preflight_block_target user-ghostty \
      "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty" "$assume_yes" "$dry_run" || return
  fi
  # Not under `||`: errexit must stay active inside to stop on a conflict.
  install_managed_configuration "$platform" "$dry_run" "$ghostty_enabled" "$assume_yes" 1

  if [[ "$skip_packages" == "1" ]]; then
    printf '%sSkipping package and tool installation.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
  else
    packages_install "$platform" "$dry_run"
    if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
      homebrew_install_packages optional cask "$dry_run" ghostty
    fi
  fi

  install_managed_configuration "$platform" "$dry_run" "$ghostty_enabled" "$assume_yes"
  install_mise_global_config "$dry_run" || return
  if [[ "$skip_packages" == "0" ]]; then
    install_neovim_plugins "$dry_run" || return
  fi
  install_default_shell "$dry_run" "$assume_yes"

  if [[ "$dry_run" == "0" ]]; then
    local configured_state
    local temporary_configured_state
    local ghostty_state
    local temporary_ghostty_state
    mkdir -p "$SELFISHELL_STATE_DIR" || return "$SELFISHELL_EXIT_ERROR"

    configured_state="$SELFISHELL_STATE_DIR/configured"
    temporary_configured_state="$(mktemp "${configured_state}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"
    printf '1\n' >"$temporary_configured_state" || {
      rm -f "$temporary_configured_state"
      return "$SELFISHELL_EXIT_ERROR"
    }
    mv "$temporary_configured_state" "$configured_state" || {
      rm -f "$temporary_configured_state"
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

  ((SELFISHELL_UNCHANGED_COUNT == 0)) ||
    printf '%s%d items unchanged.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_UNCHANGED_COUNT" "$SELFISHELL_COLOR_RESET"
  if [[ "$dry_run" == "1" ]]; then
    printf '%sDry run complete; no files were changed.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
  else
    printf '%sSelfishell configuration installed.%s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET"
  fi
}
