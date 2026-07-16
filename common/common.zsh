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
  path=("$PYENV_ROOT/bin" $path)
fi

# Shims do not require shell initialization and keep python/pip immediately
# available while the pyenv shell function itself is loaded on demand.
if [[ -d "$PYENV_ROOT/shims" ]]; then
  path=("$PYENV_ROOT/shims" $path)
fi

if (( $+commands[pyenv] )); then
  load_pyenv() {
    local virtualenv_init

    unfunction pyenv load_pyenv 2>/dev/null
    eval "$(command pyenv init - --no-rehash zsh)"
    if virtualenv_init="$(command pyenv virtualenv-init - 2>/dev/null)"; then
      eval "$virtualenv_init"
    fi
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
else
  print -u2 "Zinit not found: $ZINIT_HOME"
fi

# Completion is part of the minimal profile. Zinit only contributes additional
# definitions for larger profiles, so the standard completion system must not
# depend on Zinit being installed.
zstyle ':completion:*' matcher-list \
  '' \
  'm:{a-zA-Z}={A-Za-z}' \
  'l:|=* r:|=*'

# Initialize completion directly because Oh My Zsh is not loaded
autoload -Uz compinit
ZCOMPDUMP="${ZDOTDIR:-$HOME}/.zcompdump"
SELFISHELL_COMPLETION_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell/completions"
[[ -d "$SELFISHELL_COMPLETION_DIR" ]] && fpath=("$SELFISHELL_COMPLETION_DIR" $fpath)

if [[ ! -o interactive ]]; then
  # Automated checks and scripts have no terminal for compaudit prompts.
  compinit -C -d "$ZCOMPDUMP"
elif [[ -n "$ZCOMPDUMP"(#qN.mh+24) ]]; then
  compinit -d "$ZCOMPDUMP"
else
  compinit -C -d "$ZCOMPDUMP"
fi

# Zsh transparently uses the compiled dump on subsequent startups.
if [[ -s "$ZCOMPDUMP" && ( ! -s "$ZCOMPDUMP.zwc" || "$ZCOMPDUMP" -nt "$ZCOMPDUMP.zwc" ) ]]; then
  zcompile "$ZCOMPDUMP"
fi

if (( $+functions[zinit] )); then
  # Apply compdef calls deferred while plugins were loading
  zinit cdreplay -q
fi

# Prefer the completion generated during setup. If it is missing, defer
# generation until the first completion request so startup stays fast.
if (( $+commands[kubectl] )); then
  if [[ -r "$SELFISHELL_COMPLETION_DIR/_kubectl" ]]; then
    autoload -Uz _kubectl
    compdef _kubectl kubectl k
  else
    _selfishell_kubectl_completion() {
      local completion_source

      if completion_source="$(kubectl completion zsh 2>/dev/null)" &&
         [[ -n "$completion_source" ]]; then
        if eval "$completion_source" && (( $+functions[_kubectl] )); then
          unfunction _selfishell_kubectl_completion
          compdef _kubectl kubectl k
          _kubectl "$@"
          return
        fi
      fi

      return 1
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

if (( $+functions[zinit] )); then
  # Zinit command completion
  autoload -Uz _zinit
  compdef _zinit zinit
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


# --------------------------------------------------
# Local extension
# Kept outside managed resource state for private/company configuration
# --------------------------------------------------

SELFISHELL_LOCAL_ZSH="${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh"
[[ -r "$SELFISHELL_LOCAL_ZSH" ]] && source "$SELFISHELL_LOCAL_ZSH"
unset SELFISHELL_LOCAL_ZSH


# --------------------------------------------------
# Selfishell update notice
# Read cached metadata during startup and refresh it periodically in background.
# --------------------------------------------------

_selfishell_update_notice_refresh() {
  local cache_dir="$1"
  local checked_at="$2"
  local lock_dir="$cache_dir/update-check.lock"
  local available_file="$cache_dir/available-version"
  local checked_file="$cache_dir/update-checked-at"
  local temporary
  local available

  command mkdir -p "$cache_dir" 2>/dev/null || return
  command mkdir "$lock_dir" 2>/dev/null || return

  if available="$(command selfishell version --available 2>/dev/null)" &&
     [[ -n "$available" ]]; then
    temporary="$available_file.tmp.$$.$RANDOM"
    print -r -- "$available" >| "$temporary" &&
      command mv -f "$temporary" "$available_file"
  fi

  temporary="$checked_file.tmp.$$.$RANDOM"
  print -r -- "$checked_at" >| "$temporary" &&
    command mv -f "$temporary" "$checked_file"
  command rmdir "$lock_dir" 2>/dev/null
}

_selfishell_update_notice() {
  local enabled="${SELFISHELL_UPDATE_NOTICE:-1}"
  local interval="${SELFISHELL_UPDATE_CHECK_INTERVAL:-86400}"
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"
  local available_file="$cache_dir/available-version"
  local checked_file="$cache_dir/update-checked-at"
  local current available checked_at=0 now

  case "${enabled:l}" in
    0 | false | no | off) return ;;
  esac
  (( $+commands[selfishell] )) || return

  case "$interval" in
    "" | *[!0-9]*) interval=86400 ;;
  esac

  current="$(command selfishell version 2>/dev/null)" || return
  current="${current#selfishell }"
  if [[ -r "$available_file" ]]; then
    available="$(<"$available_file")"
    if [[ -n "$available" && "$available" != "$current" ]]; then
      print -r -- "[Selfishell] $available is available. Run: sfs update"
    fi
  fi

  zmodload zsh/datetime 2>/dev/null
  now="${EPOCHSECONDS:-$(command date +%s)}"
  [[ -r "$checked_file" ]] && checked_at="$(<"$checked_file")"
  case "$checked_at" in
    "" | *[!0-9]*) checked_at=0 ;;
  esac

  if (( now - checked_at >= interval )); then
    setopt localoptions
    unsetopt bg_nice
    (_selfishell_update_notice_refresh "$cache_dir" "$now") >/dev/null 2>&1 &!
  fi
}

if [[ -o interactive ]]; then
  _selfishell_update_notice
fi
