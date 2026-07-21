# Runtime and versioned development tools are managed by mise. Project-local
# mise.toml files override these Selfishell defaults.
if _selfishell_command_path mise >/dev/null; then
  eval "$(command mise activate zsh)"
fi
