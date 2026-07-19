#!/usr/bin/env bash

print_install_help() {
  cat <<'EOF'
Usage:
  selfishell install [--profile NAME] [--local-profile FILE]
                      [--skip-packages] [--dry-run] [--yes]

Options:
  --profile NAME       Select minimal or developer (default: minimal)
  --local-profile FILE Add private platform package records
  --skip-packages      Install configuration without package operations
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

# Install all Neovim configuration files as individual managed resources.
# Each file path under common/nvim/ maps to a resource name derived from the
# relative path (slashes and dots replaced with hyphens).
# Migrate a pre-directory-symlink nvim installation produced by an older
# Selfishell release.  The old layout managed a single init.lua file
# (nvim-config) and a symlink to that file (user-nvim -> nvim/init.lua).
# Both state records conflict with the new layout and must be cleared so
# that managed_install_link and managed_install_file can proceed cleanly.
migrate_nvim_state() {
  [[ "$1" == "0" ]] || return 0 # skip during dry-run
  if managed_read_state "user-nvim" 2>/dev/null; then
    if [[ "$MANAGED_STATE_TARGET" == */nvim/init.lua ]]; then
      managed_remove_state "user-nvim"
      managed_remove_state "nvim-config"
    fi
  fi
}

install_managed_configuration() {
  local platform="$1"
  local dry_run="$2"
  local profile="$3"
  local ghostty_enabled="${4:-0}"
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
        if [[ "$resource_name" == ghostty-config || "$resource_name" == user-ghostty ]]; then
          [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]] || continue
        fi
        managed_install_file "$resource_name" "$resource_source" "$resource_target" "$dry_run"
        ;;
      link)
        if [[ "$profile" != "developer" && "$resource_name" == user-nvim ]]; then
          continue
        fi
        if [[ "$platform" != "macos" && "$resource_name" == user-ghostty ]]; then
          continue
        fi
        managed_install_link "$resource_name" "$resource_target" "$resource_source" "$dry_run"
        ;;
    esac
  done < <(selfishell_managed_resources)

  # Migrate old single-file nvim state before installing the new layout.
  migrate_nvim_state "$dry_run"

  if [[ "$dry_run" == "0" ]]; then
    rm -f "$SELFISHELL_CACHE_DIR"/zoxide-init.zsh "$SELFISHELL_CACHE_DIR"/fzf-init.zsh "$SELFISHELL_CACHE_DIR"/starship-init.zsh 2>/dev/null
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
    printf 'Would set login shell to: %s\n' "$zsh_path"
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
    printf 'Set login shell to: %s\n' "$zsh_path"
  else
    printf 'Could not set login shell to Zsh.\n'
  fi
}

command_install() {
  local assume_yes=0
  local dry_run=0
  local profile=minimal
  local local_profile="${SELFISHELL_LOCAL_PROFILE:-}"
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
      --local-profile)
        shift
        if (("$#" == 0)); then
          cli_error "--local-profile requires a file"
          return "$SELFISHELL_EXIT_USAGE"
        fi
        local_profile="$1"
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
  profile_load "$profile" "$local_profile"

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

  install_managed_configuration "$platform" "$dry_run" "$profile" "$ghostty_enabled"
  if [[ "$skip_packages" == "0" ]]; then
    install_vim_plugins "$dry_run"
  fi
  install_default_shell "$dry_run" "$assume_yes"

  if [[ "$dry_run" == "0" ]]; then
    local profile_state
    local temporary_profile_state
    local ghostty_state
    local temporary_ghostty_state
    mkdir -p "$SELFISHELL_STATE_DIR"
    profile_state="$SELFISHELL_STATE_DIR/profile"
    temporary_profile_state="$(mktemp "${profile_state}.tmp.XXXXXX")"
    printf '%s\n' "$profile" >"$temporary_profile_state"
    mv "$temporary_profile_state" "$profile_state"
    ghostty_state="$SELFISHELL_STATE_DIR/ghostty"
    temporary_ghostty_state="$(mktemp "${ghostty_state}.tmp.XXXXXX")"
    printf '%s\n' "$ghostty_enabled" >"$temporary_ghostty_state"
    mv "$temporary_ghostty_state" "$ghostty_state"
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf 'Dry run complete; no files were changed.\n'
  else
    printf 'Selfishell configuration installed.\n'
  fi
}
