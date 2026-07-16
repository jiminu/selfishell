#!/usr/bin/env bash

print_uninstall_help() {
  cat <<'EOF'
Usage:
  selfishell uninstall [--restore] [--purge] [--dry-run] [--yes]

Options:
  --restore  Restore configuration files backed up during installation
  --purge    Also remove the Selfishell CLI, releases, cache, and state
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
EOF
}

uninstall_link_target() {
  local link_path="$1"
  local target="$2"
  local target_dir

  [[ "$target" == /* ]] || target="$(dirname "$link_path")/$target"
  target_dir="$(cd "$(dirname "$target")" && pwd -P)" || return 1
  printf '%s/%s\n' "$target_dir" "$(basename "$target")"
}

uninstall_prepare_purge() {
  local link_path="$1"
  local expected_target="$2"
  local actual_target

  if [[ -e "$link_path" || -L "$link_path" ]]; then
    if [[ ! -L "$link_path" ]]; then
      cli_error "Refusing to remove non-Selfishell path: $link_path"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    actual_target="$(uninstall_link_target "$link_path" "$(readlink "$link_path")")" || return
    expected_target="$(uninstall_link_target "$link_path" "$expected_target")" || return
    if [[ "$actual_target" != "$expected_target" ]]; then
      cli_error "Refusing to remove non-Selfishell path: $link_path"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  fi
}

uninstall_purge() {
  local dry_run="$1"
  local prefix bin_dir

  release_installation_paths || return "$SELFISHELL_EXIT_ERROR"
  prefix="$(dirname "$(dirname "$SELFISHELL_SHARE_DIR")")"
  bin_dir="$prefix/bin"
  uninstall_prepare_purge "$bin_dir/selfishell" "$SELFISHELL_SHARE_DIR/current/bin/selfishell" || return
  uninstall_prepare_purge "$bin_dir/sfs" selfishell || return

  if [[ "$dry_run" == 1 ]]; then
    printf 'Would remove Selfishell CLI link: %s\n' "$bin_dir/sfs"
    printf 'Would remove Selfishell CLI link: %s\n' "$bin_dir/selfishell"
    printf 'Would remove Selfishell releases: %s\n' "$SELFISHELL_SHARE_DIR"
    printf 'Would remove Selfishell cache: %s\n' "$SELFISHELL_CACHE_DIR"
    printf 'Would remove Selfishell state: %s\n' "$SELFISHELL_STATE_DIR"
    return
  fi

  rm -f "$bin_dir/sfs" "$bin_dir/selfishell"
  rm -rf "$SELFISHELL_CACHE_DIR" "$SELFISHELL_STATE_DIR" "$SELFISHELL_SHARE_DIR"
  printf 'Selfishell CLI and remaining data removed.\n'
}

command_uninstall() {
  local assume_yes=0
  local dry_run=0
  local restore=0
  local purge=0
  local prefix
  local resource
  local result="$SELFISHELL_EXIT_OK"

  while (("$#" > 0)); do
    case "$1" in
      --restore) restore=1 ;;
      --purge) purge=1 ;;
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

  selfishell_initialize_paths
  if [[ "$purge" == 1 ]]; then
    release_installation_paths || return "$SELFISHELL_EXIT_ERROR"
    prefix="$(dirname "$(dirname "$SELFISHELL_SHARE_DIR")")"
    uninstall_prepare_purge "$prefix/bin/selfishell" "$SELFISHELL_SHARE_DIR/current/bin/selfishell" || return
    uninstall_prepare_purge "$prefix/bin/sfs" selfishell || return
  fi
  if [[ "$purge" == 1 ]]; then
    confirm_action "Uninstall and purge Selfishell?" "$assume_yes" "$dry_run" || return
  else
    confirm_action "Uninstall Selfishell configuration?" "$assume_yes" "$dry_run" || return
  fi

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

  if [[ "$result" == "$SELFISHELL_EXIT_OK" && "$purge" == 1 ]]; then
    uninstall_purge "$dry_run" || return
  fi

  return "$result"
}
