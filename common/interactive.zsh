# Aliases
source "$SELFISHELL_COMMON_DIR/aliases-common.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-editor.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-git.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-kubectl.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

if command -v zoxide >/dev/null 2>&1; then
  _selfishell_zoxide_cache="$SELFISHELL_CACHE_DIR/zoxide-init.zsh"
  if [[ ! -s "$_selfishell_zoxide_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    zoxide init zsh >|"$_selfishell_zoxide_cache" 2>/dev/null
  fi
  [[ -s "$_selfishell_zoxide_cache" ]] && source "$_selfishell_zoxide_cache"
  unset _selfishell_zoxide_cache
fi

if command -v fzf >/dev/null 2>&1; then
  _selfishell_fzf_cache="$SELFISHELL_CACHE_DIR/fzf-init.zsh"
  if [[ ! -s "$_selfishell_fzf_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    local fzf_init
    if fzf_init="$(fzf --zsh 2>/dev/null)" && [[ -n "$fzf_init" ]]; then
      print -r -- "$fzf_init" >|"$_selfishell_fzf_cache"
    elif [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]]; then
      command cp /usr/share/doc/fzf/examples/key-bindings.zsh "$_selfishell_fzf_cache"
    fi
  fi
  [[ -s "$_selfishell_fzf_cache" ]] && source "$_selfishell_fzf_cache"
  unset _selfishell_fzf_cache
fi

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

if command -v starship >/dev/null 2>&1; then
  _selfishell_starship_cache="$SELFISHELL_CACHE_DIR/starship-init.zsh"
  if [[ ! -s "$_selfishell_starship_cache" ]]; then
    command mkdir -p "$SELFISHELL_CACHE_DIR" 2>/dev/null
    starship init zsh >|"$_selfishell_starship_cache" 2>/dev/null
  fi
  [[ -s "$_selfishell_starship_cache" ]] && source "$_selfishell_starship_cache"
  unset _selfishell_starship_cache
fi

unset SELFISHELL_CACHE_DIR
