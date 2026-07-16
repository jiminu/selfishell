# Aliases
source "$SELFISHELL_COMMON_DIR/aliases-common.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-git.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-kubectl.zsh"

# Shell tools configure key bindings before interactive plugins load.
if command -v zoxide >/dev/null 2>&1; then
  eval "$(zoxide init zsh)"
fi

if command -v fzf >/dev/null 2>&1; then
  if FZF_ZSH_INIT="$(fzf --zsh 2>/dev/null)"; then
    eval "$FZF_ZSH_INIT"
  elif [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]]; then
    source /usr/share/doc/fzf/examples/key-bindings.zsh
  fi
  unset FZF_ZSH_INIT
fi

if (( $+functions[zinit] )); then
  zinit light zsh-users/zsh-autosuggestions
  zinit light zdharma-continuum/fast-syntax-highlighting
fi

if command -v starship >/dev/null 2>&1; then
  eval "$(starship init zsh)"
fi

# Private/company configuration is never managed by Selfishell.
SELFISHELL_LOCAL_ZSH="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh"
[[ -r "$SELFISHELL_LOCAL_ZSH" ]] && source "$SELFISHELL_LOCAL_ZSH"
unset SELFISHELL_LOCAL_ZSH
