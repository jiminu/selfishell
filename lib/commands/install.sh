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

install_managed_configuration() {
  local platform="$1"
  local dry_run="$2"
  local ghostty_enabled="${3:-0}"
  local zsh_source

  case "$platform" in
    macos) zsh_source="$SELFISHELL_ROOT/mac/.zshrc" ;;
    ubuntu | ubuntu-wsl) zsh_source="$SELFISHELL_ROOT/ubuntu/.zshrc" ;;
    *)
      cli_error "Managed installation is unavailable on $(platform_label "$platform")."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  managed_install_file zshrc-config "$zsh_source" "$SELFISHELL_CONFIG_DIR/zsh/zshrc" "$dry_run"
  managed_install_file zshenv-config "$SELFISHELL_ROOT/common/zshenv" "$SELFISHELL_CONFIG_DIR/zsh/zshenv" "$dry_run"
  managed_install_file zsh-runtime "$SELFISHELL_ROOT/common/runtime.zsh" "$SELFISHELL_CONFIG_DIR/zsh/runtime.zsh" "$dry_run"
  managed_install_file mise-config "$SELFISHELL_ROOT/common/mise.toml" "$SELFISHELL_CONFIG_DIR/mise/config.toml" "$dry_run"
  managed_install_file zsh-completion "$SELFISHELL_ROOT/common/completion.zsh" "$SELFISHELL_CONFIG_DIR/zsh/completion.zsh" "$dry_run"
  managed_install_file zsh-interactive "$SELFISHELL_ROOT/common/interactive.zsh" "$SELFISHELL_CONFIG_DIR/zsh/interactive.zsh" "$dry_run"
  managed_install_file zsh-update-notice "$SELFISHELL_ROOT/common/update-notice.zsh" "$SELFISHELL_CONFIG_DIR/zsh/update-notice.zsh" "$dry_run"
  # Switch the entrypoint only after every module it sources is available.
  managed_install_file zsh-common "$SELFISHELL_ROOT/common/common.zsh" "$SELFISHELL_CONFIG_DIR/zsh/common.zsh" "$dry_run"
  managed_install_file aliases-common "$SELFISHELL_ROOT/common/aliases-common.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-common.zsh" "$dry_run"
  managed_install_file aliases-git "$SELFISHELL_ROOT/common/aliases-git.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-git.zsh" "$dry_run"
  managed_install_file aliases-kubectl "$SELFISHELL_ROOT/common/aliases-kubectl.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-kubectl.zsh" "$dry_run"
  managed_install_file starship-config "$SELFISHELL_ROOT/common/starship.toml" "$SELFISHELL_CONFIG_DIR/starship.toml" "$dry_run"
  managed_install_file nvim-config "$SELFISHELL_ROOT/common/init.lua" "$SELFISHELL_CONFIG_DIR/nvim/init.lua" "$dry_run"

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_install_file ghostty-config "$SELFISHELL_ROOT/mac/config.ghostty" "$SELFISHELL_CONFIG_DIR/ghostty/config" "$dry_run"
  fi

  managed_install_link user-zshrc "$HOME/.zshrc" "$SELFISHELL_CONFIG_DIR/zsh/zshrc" "$dry_run"
  managed_install_link user-zshenv "$HOME/.zshenv" "$SELFISHELL_CONFIG_DIR/zsh/zshenv" "$dry_run"
  managed_install_link user-starship "${XDG_CONFIG_HOME:-$HOME/.config}/starship.toml" "$SELFISHELL_CONFIG_DIR/starship.toml" "$dry_run"
  managed_install_link user-nvim "${XDG_CONFIG_HOME:-$HOME/.config}/nvim/init.lua" "$SELFISHELL_CONFIG_DIR/nvim/init.lua" "$dry_run"

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_install_link user-ghostty "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config" "$SELFISHELL_CONFIG_DIR/ghostty/config" "$dry_run"
  fi

  if [[ "$dry_run" == "0" ]]; then
    rm -f "$SELFISHELL_CACHE_DIR"/zoxide-init.zsh "$SELFISHELL_CACHE_DIR"/fzf-init.zsh "$SELFISHELL_CACHE_DIR"/starship-init.zsh 2>/dev/null
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

  install_managed_configuration "$platform" "$dry_run" "$ghostty_enabled"
  if [[ "$skip_packages" == "0" ]]; then
    install_vim_plugins "$dry_run"
  fi

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
