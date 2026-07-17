# Aliases
source "$SELFISHELL_COMMON_DIR/aliases-common.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-git.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-kubectl.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

if command -v zoxide >/dev/null 2>&1; then
  _selfishell_zoxide_cache="$SELFISHELL_CACHE_DIR/zoxide-init.zsh"
  if [[ ! -s "$_selfishell_zoxide_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    zoxide init zsh >| "$_selfishell_zoxide_cache" 2>/dev/null
  fi
  source "$_selfishell_zoxide_cache"
  unset _selfishell_zoxide_cache
fi

if command -v fzf >/dev/null 2>&1; then
  _selfishell_fzf_cache="$SELFISHELL_CACHE_DIR/fzf-init.zsh"
  if [[ ! -s "$_selfishell_fzf_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    local fzf_init
    if fzf_init="$(fzf --zsh 2>/dev/null)" && [[ -n "$fzf_init" ]]; then
      print -r -- "$fzf_init" >| "$_selfishell_fzf_cache"
    elif [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]]; then
      command cp /usr/share/doc/fzf/examples/key-bindings.zsh "$_selfishell_fzf_cache"
    fi
  fi
  [[ -s "$_selfishell_fzf_cache" ]] && source "$_selfishell_fzf_cache"
  unset _selfishell_fzf_cache
fi

if (( $+functions[zinit] )); then
  zinit ice wait'0' lucid
  zinit light zsh-users/zsh-autosuggestions
  zinit ice wait'0' lucid
  zinit light zdharma-continuum/fast-syntax-highlighting
fi

if command -v starship >/dev/null 2>&1; then
  _selfishell_starship_cache="$SELFISHELL_CACHE_DIR/starship-init.zsh"
  if [[ ! -s "$_selfishell_starship_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    starship init zsh >| "$_selfishell_starship_cache" 2>/dev/null
  fi
  source "$_selfishell_starship_cache"
  unset _selfishell_starship_cache
fi

unset SELFISHELL_CACHE_DIR

# Private/company configuration is never managed by Selfishell.
SELFISHELL_LOCAL_ZSH="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh"
[[ -r "$SELFISHELL_LOCAL_ZSH" ]] && source "$SELFISHELL_LOCAL_ZSH"
unset SELFISHELL_LOCAL_ZSH
