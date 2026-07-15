#!/usr/bin/env bash

status_resource() {
  local resource="$1"
  local current_checksum

  if ! managed_read_state "$resource"; then
    return 0
  fi

  SELFISHELL_STATUS_RESOURCE_COUNT=$((SELFISHELL_STATUS_RESOURCE_COUNT + 1))

  if [[ "$MANAGED_STATE_STATUS" != "active" ]]; then
    printf '[PENDING] %s\n' "$MANAGED_STATE_TARGET"
    SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
    return
  fi

  case "$MANAGED_STATE_TYPE" in
    link)
      if [[ -L "$MANAGED_STATE_TARGET" && "$(readlink "$MANAGED_STATE_TARGET")" == "$MANAGED_STATE_REFERENCE" ]]; then
        printf '[OK] %s -> %s\n' "$MANAGED_STATE_TARGET" "$MANAGED_STATE_REFERENCE"
      else
        printf '[CHANGED] %s\n' "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")"
      fi
      if [[ -n "$current_checksum" && "$current_checksum" == "$MANAGED_STATE_CHECKSUM" ]]; then
        printf '[OK] %s\n' "$MANAGED_STATE_TARGET"
      else
        printf '[CHANGED] %s\n' "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
  esac
}

command_status() {
  local resource

  require_no_arguments status "$@" || return
  selfishell_initialize_paths

  SELFISHELL_STATUS_RESOURCE_COUNT=0
  SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_OK"

  for resource in \
    zshrc-config zsh-common aliases-common aliases-git aliases-kubectl \
    starship-config vim-config ghostty-config \
    user-zshrc user-starship user-vim user-ghostty; do
    status_resource "$resource"
  done

  if ((SELFISHELL_STATUS_RESOURCE_COUNT == 0)); then
    printf 'Selfishell configuration is not installed.\n'
    return "$SELFISHELL_EXIT_ERROR"
  fi

  return "$SELFISHELL_STATUS_RESULT"
}
