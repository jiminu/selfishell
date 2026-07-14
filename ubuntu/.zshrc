# --------------------------------------------------
# PATH
# Prefer user-local binaries on Ubuntu/WSL
# --------------------------------------------------

export PATH="$HOME/.local/bin:$HOME/.rd/bin:$PATH"


# --------------------------------------------------
# Runtime environments
# --------------------------------------------------

load_nvm() {
  unfunction nvm node npm npx 2>/dev/null
  [[ -s "$NVM_DIR/nvm.sh" ]] && source "$NVM_DIR/nvm.sh"
}


# --------------------------------------------------
# Common settings
# --------------------------------------------------

COMMON_ZSH="$HOME/.config/zsh/common.zsh"

if [[ -r "$COMMON_ZSH" ]]; then
  source "$COMMON_ZSH"
else
  print -u2 "Common Zsh configuration not found: $COMMON_ZSH"
fi
