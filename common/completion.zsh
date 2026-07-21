# Zinit contributes additional definitions, but standard completion does not
# depend on it being installed.
ZINIT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git"

if [[ -s "$ZINIT_HOME/zinit.zsh" ]]; then
  source "$ZINIT_HOME/zinit.zsh"
  # Pinned to the commit recorded for zsh-users/zsh-completions in
  # dependencies.conf; keep the two in sync (see tests/common_zsh_test.bash).
  zinit ice blockf atpull'zinit creinstall -q .' ver'9903bae60284072de3fa0e3e20965f22368c5694'
  zinit light zsh-users/zsh-completions
fi

zstyle ':completion:*' matcher-list \
  '' \
  'm:{a-zA-Z}={A-Za-z}' \
  'l:|=* r:|=*'

autoload -Uz compinit
ZCOMPDUMP="${ZDOTDIR:-$HOME}/.zcompdump"
SELFISHELL_COMPLETION_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell/completions"
[[ -d "$SELFISHELL_COMPLETION_DIR" ]] && fpath=("$SELFISHELL_COMPLETION_DIR" $fpath)
SELFISHELL_COMPLETION_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"
SELFISHELL_COMPLETION_CACHE_READY=0

selfishell_completion_prepare_cache() {
  if [[ "$SELFISHELL_COMPLETION_CACHE_READY" == 0 ]]; then
    command mkdir -p "$SELFISHELL_COMPLETION_CACHE_DIR" 2>/dev/null
    SELFISHELL_COMPLETION_CACHE_READY=1
  fi
}

if [[ ! -o interactive ]]; then
  compinit -u -C -d "$ZCOMPDUMP"
elif [[ -n "$ZCOMPDUMP"(#qN.mh+24) ]]; then
  compinit -u -d "$ZCOMPDUMP"
else
  compinit -u -C -d "$ZCOMPDUMP"
fi

if [[ -s "$ZCOMPDUMP" && ( ! -s "$ZCOMPDUMP.zwc" || "$ZCOMPDUMP" -nt "$ZCOMPDUMP.zwc" ) ]]; then
  zcompile "$ZCOMPDUMP"
fi

if (( $+functions[zinit] )); then
  zinit cdreplay -q
fi

if _selfishell_command_path kubectl >/dev/null; then
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

if _selfishell_command_path aws >/dev/null && _selfishell_command_path aws_completer >/dev/null; then
  autoload -Uz bashcompinit
  bashcompinit
  complete -C aws_completer aws
fi

if (( $+functions[zinit] )); then
  autoload -Uz _zinit
  compdef _zinit zinit
fi

unset SELFISHELL_COMPLETION_CACHE_READY
unset SELFISHELL_COMPLETION_CACHE_DIR
unset SELFISHELL_COMPLETION_DIR
