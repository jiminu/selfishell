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

export PATH="$HOME/.rd/bin:$PATH"


# --------------------------------------------------
# Runtime environments
# --------------------------------------------------

# NVM (Homebrew installation)
load_nvm() {
  unfunction nvm node npm npx 2>/dev/null

  if command -v brew >/dev/null 2>&1; then
    local nvm_script="$(brew --prefix nvm 2>/dev/null)/nvm.sh"
    [[ -s "$nvm_script" ]] && source "$nvm_script"
  fi
}

if command -v brew >/dev/null 2>&1; then
  BREW_PREFIX="$(brew --prefix 2>/dev/null)"

  # Java 17
  JAVA_HOME_17="$BREW_PREFIX/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"

  if [[ -d "$JAVA_HOME_17" ]]; then
    export JAVA_HOME="$JAVA_HOME_17"
    export PATH="$JAVA_HOME/bin:$PATH"
  fi
fi


# --------------------------------------------------
# Common settings
# --------------------------------------------------

COMMON_ZSH="$HOME/.config/zsh/common.zsh"

if [[ -r "$COMMON_ZSH" ]]; then
  source "$COMMON_ZSH"
else
  print -u2 "Common Zsh configuration not found: $COMMON_ZSH"
fi
