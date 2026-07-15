#!/usr/bin/env bash

print_install_help() {
  cat <<'EOF'
Usage:
  selfishell install [--dry-run] [--yes]

Options:
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

install_managed_configuration() {
  local platform="$1"
  local dry_run="$2"
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
  managed_install_file zsh-common "$SELFISHELL_ROOT/common/common.zsh" "$SELFISHELL_CONFIG_DIR/zsh/common.zsh" "$dry_run"
  managed_install_file aliases-common "$SELFISHELL_ROOT/common/aliases-common.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-common.zsh" "$dry_run"
  managed_install_file aliases-git "$SELFISHELL_ROOT/common/aliases-git.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-git.zsh" "$dry_run"
  managed_install_file aliases-kubectl "$SELFISHELL_ROOT/common/aliases-kubectl.zsh" "$SELFISHELL_CONFIG_DIR/zsh/aliases-kubectl.zsh" "$dry_run"
  managed_install_file starship-config "$SELFISHELL_ROOT/common/starship.toml" "$SELFISHELL_CONFIG_DIR/starship.toml" "$dry_run"
  managed_install_file vim-config "$SELFISHELL_ROOT/common/.vimrc" "$SELFISHELL_CONFIG_DIR/vim/vimrc" "$dry_run"

  if [[ "$platform" == "macos" ]]; then
    managed_install_file ghostty-config "$SELFISHELL_ROOT/mac/config.ghostty" "$SELFISHELL_CONFIG_DIR/ghostty/config" "$dry_run"
  fi

  managed_install_link user-zshrc "$HOME/.zshrc" "$SELFISHELL_CONFIG_DIR/zsh/zshrc" "$dry_run"
  managed_install_link user-starship "${XDG_CONFIG_HOME:-$HOME/.config}/starship.toml" "$SELFISHELL_CONFIG_DIR/starship.toml" "$dry_run"
  managed_install_link user-vim "$HOME/.vimrc" "$SELFISHELL_CONFIG_DIR/vim/vimrc" "$dry_run"

  if [[ "$platform" == "macos" ]]; then
    managed_install_link user-ghostty "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config" "$SELFISHELL_CONFIG_DIR/ghostty/config" "$dry_run"
  fi
}

command_install() {
  local assume_yes=0
  local dry_run=0
  local platform

  while (("$#" > 0)); do
    case "$1" in
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
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
  install_managed_configuration "$platform" "$dry_run"

  if [[ "$dry_run" == "1" ]]; then
    printf 'Dry run complete; no files were changed.\n'
  else
    printf 'Selfishell configuration installed.\n'
  fi
}

command_legacy_install() {
  local platform
  platform="$(detect_platform)"

  case "$platform" in
    macos) macos_legacy_install "$@" ;;
    ubuntu-wsl) ubuntu_wsl_legacy_install "$@" ;;
    ubuntu)
      cli_error "Native Ubuntu bootstrap is not implemented yet."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
    *)
      cli_error "Installation is unavailable on $(platform_label "$platform")."
      cli_error "Run 'selfishell doctor' for supported platform details."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}
