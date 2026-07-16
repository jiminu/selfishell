#!/usr/bin/env bash

print_uninstall_help() {
  cat <<'EOF'
Usage:
  selfishell uninstall [--restore] [--dry-run] [--yes]

Options:
  --restore  Restore configuration files backed up during installation
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

command_uninstall() {
  local assume_yes=0
  local dry_run=0
  local restore=0
  local resource
  local result="$SELFISHELL_EXIT_OK"

  while (("$#" > 0)); do
    case "$1" in
      --restore) restore=1 ;;
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
      help | --help | -h)
        print_uninstall_help
        return
        ;;
      *)
        cli_error "Unknown uninstall option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done

  confirm_action "Uninstall Selfishell configuration?" "$assume_yes" "$dry_run" || return
  selfishell_initialize_paths

  for resource in \
    user-ghostty user-vim user-starship user-zshrc \
    ghostty-config vim-config starship-config aliases-kubectl aliases-git \
    aliases-common zsh-common zshrc-config; do
    managed_validate_uninstall_resource "$resource" || result="$SELFISHELL_EXIT_ERROR"
  done

  if [[ "$result" != "$SELFISHELL_EXIT_OK" ]]; then
    cli_error "Uninstall cancelled because managed resources were changed."
    return "$result"
  fi

  for resource in user-ghostty user-vim user-starship user-zshrc; do
    managed_uninstall_resource "$resource" "$restore" "$dry_run" || result="$SELFISHELL_EXIT_ERROR"
  done

  for resource in ghostty-config vim-config starship-config aliases-kubectl aliases-git aliases-common zsh-common zshrc-config; do
    managed_uninstall_resource "$resource" "$restore" "$dry_run" || result="$SELFISHELL_EXIT_ERROR"
  done

  if [[ "$dry_run" == "0" ]]; then
    rm -f "$SELFISHELL_STATE_DIR/profile" "$SELFISHELL_STATE_DIR/ghostty"
    rmdir "$SELFISHELL_CONFIG_DIR/ghostty" 2>/dev/null || true
    rmdir "$SELFISHELL_CONFIG_DIR/vim" 2>/dev/null || true
    rmdir "$SELFISHELL_CONFIG_DIR/zsh" 2>/dev/null || true
    rmdir "$SELFISHELL_CONFIG_DIR" 2>/dev/null || true
    rmdir "$SELFISHELL_RESOURCE_STATE_DIR" 2>/dev/null || true
    rmdir "$SELFISHELL_STATE_DIR" 2>/dev/null || true
  fi

  if [[ "$result" == "$SELFISHELL_EXIT_OK" ]]; then
    if [[ "$dry_run" == "1" ]]; then
      printf 'Dry run complete; no files were changed.\n'
    else
      printf 'Selfishell configuration uninstalled.\n'
    fi
  fi

  return "$result"
}
