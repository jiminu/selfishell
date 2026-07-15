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
path=("$HOME/.rd/bin" $path)


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

if (( $+commands[brew] )); then
  # Keep standard installations fast and ask Homebrew only when the executable
  # comes from a custom prefix or a wrapper.
  BREW_PATH="${commands[brew]:A}"

  if [[ -n "$HOMEBREW_PREFIX" ]]; then
    BREW_PREFIX="$HOMEBREW_PREFIX"
  elif [[ "$BREW_PATH" == /opt/homebrew/bin/brew ]]; then
    BREW_PREFIX="/opt/homebrew"
  elif [[ "$BREW_PATH" == /usr/local/bin/brew ]]; then
    BREW_PREFIX="/usr/local"
  else
    BREW_PREFIX="$(command brew --prefix 2>/dev/null)" || BREW_PREFIX=""
  fi

  # Java 17
  if [[ -n "$BREW_PREFIX" && "$BREW_PREFIX" == /* ]]; then
    JAVA_HOME_17="$BREW_PREFIX/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"

    if [[ -d "$JAVA_HOME_17" ]]; then
      export JAVA_HOME="$JAVA_HOME_17"
      path=("$JAVA_HOME/bin" $path)
    fi
  fi

  unset BREW_PATH
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
