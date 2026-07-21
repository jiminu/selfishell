# Aliases
source "$SELFISHELL_COMMON_DIR/aliases-common.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-editor.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-git.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-kubectl.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

# Writes "$@"'s stdout to $target through a temp file, validated non-empty
# and zsh-syntax-clean before the atomic rename, so a process killed
# mid-generation (a closed terminal, a signal) can never leave a partially
# written, non-empty cache that then gets sourced forever without
# regenerating (the caller's [[ -s ]] check can't tell "empty" from
# "truncated"). Only fits tools whose init output is exactly one command's
# stdout; fzf's fallback-to-a-copied-file shape doesn't, so it stays
# separate below rather than forcing it through this signature.
_selfishell_generate_zsh_cache() {
  local target="$1"
  shift
  local temporary="${target}.tmp.$$.$RANDOM"

  command mkdir -p "${target:h}" 2>/dev/null || return 1

  "$@" >|"$temporary" 2>/dev/null || {
    command rm -f "$temporary"
    return 1
  }
  [[ -s "$temporary" ]] || {
    command rm -f "$temporary"
    return 1
  }
  command zsh -n "$temporary" >/dev/null 2>&1 || {
    command rm -f "$temporary"
    return 1
  }
  command mv -f "$temporary" "$target"
}

_selfishell_generate_fzf_cache() {
  local target="$1"
  local temporary="${target}.tmp.$$.$RANDOM"
  local fzf_init

  command mkdir -p "${target:h}" 2>/dev/null || return 1

  if fzf_init="$(fzf --zsh 2>/dev/null)" && [[ -n "$fzf_init" ]]; then
    print -r -- "$fzf_init" >|"$temporary"
  elif [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]]; then
    command cp /usr/share/doc/fzf/examples/key-bindings.zsh "$temporary" 2>/dev/null
  else
    return 1
  fi

  [[ -s "$temporary" ]] || {
    command rm -f "$temporary"
    return 1
  }
  command zsh -n "$temporary" >/dev/null 2>&1 || {
    command rm -f "$temporary"
    return 1
  }
  command mv -f "$temporary" "$target"
}

if _selfishell_zoxide_bin="$(command -v zoxide)"; then
  _selfishell_zoxide_cache="$SELFISHELL_CACHE_DIR/zoxide-init.zsh"
  if [[ ! -s "$_selfishell_zoxide_cache" || "$_selfishell_zoxide_bin" -nt "$_selfishell_zoxide_cache" ]]; then
    _selfishell_generate_zsh_cache "$_selfishell_zoxide_cache" zoxide init zsh
  fi
  [[ -s "$_selfishell_zoxide_cache" ]] && source "$_selfishell_zoxide_cache"
  unset _selfishell_zoxide_cache
fi
unset _selfishell_zoxide_bin

if _selfishell_fzf_bin="$(command -v fzf)"; then
  _selfishell_fzf_cache="$SELFISHELL_CACHE_DIR/fzf-init.zsh"
  if [[ ! -s "$_selfishell_fzf_cache" || "$_selfishell_fzf_bin" -nt "$_selfishell_fzf_cache" ]]; then
    _selfishell_generate_fzf_cache "$_selfishell_fzf_cache"
  fi
  [[ -s "$_selfishell_fzf_cache" ]] && source "$_selfishell_fzf_cache"
  unset _selfishell_fzf_cache
fi
unset _selfishell_fzf_bin

if (($+functions[zinit])); then
  # Pinned to the commits recorded in dependencies.conf; keep the two in
  # sync (see tests/common_zsh_test.bash).
  # fzf-tab must be loaded synchronously (without wait) to ensure ZLE wrapping is applied in the correct order
  if command -v fzf >/dev/null 2>&1; then
    zinit ice ver'24105b15714bfec37989ed5c5b6e60f572253019'
    zinit light Aloxaf/fzf-tab
  fi
  zinit ice wait'0' lucid ver'85919cd1ffa7d2d5412f6d3fe437ebdbeeec4fc5'
  zinit light zsh-users/zsh-autosuggestions
  zinit ice wait'0' lucid ver'3d574ccf48804b10dca52625df13da5edae7f553'
  zinit light zdharma-continuum/fast-syntax-highlighting
fi

if _selfishell_starship_bin="$(command -v starship)"; then
  _selfishell_starship_cache="$SELFISHELL_CACHE_DIR/starship-init.zsh"
  if [[ ! -s "$_selfishell_starship_cache" || "$_selfishell_starship_bin" -nt "$_selfishell_starship_cache" ]]; then
    _selfishell_generate_zsh_cache "$_selfishell_starship_cache" starship init zsh
  fi
  [[ -s "$_selfishell_starship_cache" ]] && source "$_selfishell_starship_cache"
  unset _selfishell_starship_cache
fi
unset _selfishell_starship_bin

unset SELFISHELL_CACHE_DIR
