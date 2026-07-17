# --------------------------------------------------
# PATH
# Prefer user-local binaries on Ubuntu/WSL
# --------------------------------------------------

typeset -U path PATH
path=("$HOME/.local/bin" "$HOME/.rd/bin" $path)

# Windows PATH entries remain available after startup, but probing them while
# initializing Linux tools is expensive on WSL's mounted filesystem.
if [[ -n "${WSL_DISTRO_NAME:-}" ]]; then
  SELFISHELL_WINDOWS_PATH=("${(@M)path:#/mnt/[a-zA-Z]/*}")
  path=("${(@)path:#/mnt/[a-zA-Z]/*}")
fi


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

if (( ${#SELFISHELL_WINDOWS_PATH[@]} )); then
  path+=("${SELFISHELL_WINDOWS_PATH[@]}")
fi
unset SELFISHELL_WINDOWS_PATH
