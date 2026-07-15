# --------------------------------------------------
# Runtime environments
# Define load_nvm in each OS-specific .zshrc before loading this file
# --------------------------------------------------

export NVM_DIR="$HOME/.nvm"

nvm()  { load_nvm; nvm "$@" }
node() { load_nvm; command node "$@" }
npm()  { load_nvm; command npm "$@" }
npx()  { load_nvm; command npx "$@" }


# Pyenv
export PYENV_ROOT="$HOME/.pyenv"

if [[ -d "$PYENV_ROOT/bin" ]]; then
  export PATH="$PYENV_ROOT/bin:$PATH"
fi

# Shims do not require shell initialization and keep python/pip immediately
# available while the pyenv shell function itself is loaded on demand.
if [[ -d "$PYENV_ROOT/shims" ]]; then
  export PATH="$PYENV_ROOT/shims:$PATH"
fi

if (( $+commands[pyenv] )); then
  load_pyenv() {
    unfunction pyenv load_pyenv 2>/dev/null
    eval "$(command pyenv init - --no-rehash zsh)"
  }

  pyenv() {
    load_pyenv
    pyenv "$@"
  }
fi


# --------------------------------------------------
# Zinit
# --------------------------------------------------

ZINIT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git"

if [[ -s "$ZINIT_HOME/zinit.zsh" ]]; then
  source "$ZINIT_HOME/zinit.zsh"

  # Additional completion definitions
  # Reinstall completion files when the plugin is updated
  zinit ice blockf atpull'zinit creinstall -q .'
  zinit light zsh-users/zsh-completions

  # Complete by prefix, case-insensitive match, then substring match
  zstyle ':completion:*' matcher-list \
    '' \
    'm:{a-zA-Z}={A-Za-z}' \
    'l:|=* r:|=*'

  # Initialize completion directly because Oh My Zsh is not loaded
  autoload -Uz compinit
  ZCOMPDUMP="${ZDOTDIR:-$HOME}/.zcompdump"
  SELFISHELL_COMPLETION_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell/completions"
  [[ -d "$SELFISHELL_COMPLETION_DIR" ]] && fpath=("$SELFISHELL_COMPLETION_DIR" $fpath)

  if [[ -n "$ZCOMPDUMP"(#qN.mh+24) ]]; then
    compinit -d "$ZCOMPDUMP"
  else
    compinit -C -d "$ZCOMPDUMP"
  fi

  # Zsh transparently uses the compiled dump on subsequent startups.
  if [[ -s "$ZCOMPDUMP" && ( ! -s "$ZCOMPDUMP.zwc" || "$ZCOMPDUMP" -nt "$ZCOMPDUMP.zwc" ) ]]; then
    zcompile "$ZCOMPDUMP"
  fi

  # Apply compdef calls deferred while plugins were loading
  zinit cdreplay -q

  # Prefer the completion generated during setup. If it is missing, defer
  # generation until the first completion request so startup stays fast.
  if (( $+commands[kubectl] )); then
    if [[ -r "$SELFISHELL_COMPLETION_DIR/_kubectl" ]]; then
      autoload -Uz _kubectl
      compdef _kubectl kubectl k
    else
      _selfishell_kubectl_completion() {
        unfunction _selfishell_kubectl_completion
        source <(kubectl completion zsh)
        compdef _kubectl kubectl k
        _kubectl "$@"
      }
      compdef _selfishell_kubectl_completion kubectl k
    fi
  fi

  # Run aws_completer only when completion is requested
  if (( $+commands[aws] && $+commands[aws_completer] )); then
    autoload -Uz bashcompinit
    bashcompinit
    complete -C aws_completer aws
  fi

  # Zinit command completion
  autoload -Uz _zinit
  compdef _zinit zinit
else
  print -u2 "Zinit not found: $ZINIT_HOME"
fi

unset SELFISHELL_COMPLETION_DIR


# --------------------------------------------------
# Aliases
# --------------------------------------------------

# Local, dependency-free aliases. Keeping aliases in separate files makes them
# easy to review without mixing them with completion and tool initialization.
SELFISHELL_COMMON_DIR="${${(%):-%x}:A:h}"
source "$SELFISHELL_COMMON_DIR/aliases-common.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-git.zsh"
source "$SELFISHELL_COMMON_DIR/aliases-kubectl.zsh"
unset SELFISHELL_COMMON_DIR


# --------------------------------------------------
# Shell tools
# --------------------------------------------------

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


# --------------------------------------------------
# Interactive plugins
# Initialize after tools that configure key bindings
# --------------------------------------------------

if (( $+functions[zinit] )); then
  zinit light zsh-users/zsh-autosuggestions

  # Load syntax highlighting as late as possible
  zinit light zdharma-continuum/fast-syntax-highlighting
fi


# --------------------------------------------------
# Prompt
# Initialize Starship after the other prompt-related settings
# --------------------------------------------------

if command -v starship >/dev/null 2>&1; then
  eval "$(starship init zsh)"
fi
