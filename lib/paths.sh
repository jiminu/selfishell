#!/usr/bin/env bash

selfishell_initialize_paths() {
  # These paths are consumed by command and managed-resource modules.
  # shellcheck disable=SC2034
  SELFISHELL_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell"
  SELFISHELL_STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/selfishell"
  # shellcheck disable=SC2034
  SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"
  # shellcheck disable=SC2034
  SELFISHELL_RESOURCE_STATE_DIR="$SELFISHELL_STATE_DIR/resources"
}
