# Runtime and versioned development tools are managed by mise. Project-local
# mise.toml files override these Selfishell defaults.
if _selfishell_command_path mise >/dev/null; then
  SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"
  _selfishell_mise_cache="$SELFISHELL_CACHE_DIR/mise-init.zsh"
  if [[ ! -s "$_selfishell_mise_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    command mise activate zsh >|"$_selfishell_mise_cache" 2>/dev/null
  fi
  # The cached script still ends with a live call that re-detects the
  # current directory's tool versions on every shell start; only the
  # surrounding wrapper/hook-registration boilerplate is being cached here.
  [[ -s "$_selfishell_mise_cache" ]] && source "$_selfishell_mise_cache"
  unset _selfishell_mise_cache SELFISHELL_CACHE_DIR
fi
