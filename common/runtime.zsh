# Runtime and versioned development tools are managed by mise. Project-local
# mise.toml files override these Selfishell defaults.
if _selfishell_command_path mise >/dev/null; then
  _selfishell_profile_state="${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/profile"
  if [[ -r "$_selfishell_profile_state" && "$(<"$_selfishell_profile_state")" == developer ]]; then
    export MISE_GLOBAL_CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/config.toml"
  elif [[ "${MISE_GLOBAL_CONFIG_FILE:-}" == "${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/config.toml" ]]; then
    unset MISE_GLOBAL_CONFIG_FILE
  fi
  eval "$(command mise activate zsh)"
  unset _selfishell_profile_state
fi

# On Linux systems without FUSE support (e.g. Docker containers), run AppImage
# by extracting to a temporary directory instead of mounting. Has no effect on
# macOS or Linux environments where /dev/fuse is available.
if [[ "$(uname -s)" == "Linux" && ! -c /dev/fuse ]]; then
  export APPIMAGE_EXTRACT_AND_RUN=1
fi

