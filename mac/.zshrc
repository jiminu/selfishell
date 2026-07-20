# --------------------------------------------------
# Homebrew
# Detect Homebrew even when a new Mac does not have it in PATH yet
# --------------------------------------------------

if ! command -v brew >/dev/null 2>&1; then
  if [[ -x /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  elif [[ -x /usr/local/bin/brew ]]; then
    eval "$(/usr/local/bin/brew shellenv)"
  fi
fi


# --------------------------------------------------
# PATH
# Register paths before plugins look for commands such as kubectl
# --------------------------------------------------

typeset -U path PATH
path=("$HOME/.local/bin" "$HOME/.rd/bin" $path)



# --------------------------------------------------
# Common settings
# --------------------------------------------------

COMMON_ZSH="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/zsh/common.zsh"
if [[ ! -r "$COMMON_ZSH" ]]; then
  COMMON_ZSH="$HOME/.config/zsh/common.zsh"
fi

if [[ -r "$COMMON_ZSH" ]]; then
  source "$COMMON_ZSH"
else
  print -u2 "Common Zsh configuration not found: $COMMON_ZSH"
fi
