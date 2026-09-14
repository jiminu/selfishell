# Zinit contributes additional definitions, but standard completion does not
# depend on it being installed.
ZINIT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git"

if [[ -s "$ZINIT_HOME/zinit.zsh" ]]; then
  source "$ZINIT_HOME/zinit.zsh"

  if _selfishell_zinit_plugin_ready zsh-users/zsh-completions; then
    # Pinned to the commit recorded for zsh-users/zsh-completions in
    # dependencies.conf; keep the two in sync (see tests/common_zsh_test.bash).
    zinit ice blockf atpull'zinit creinstall -q .' ver'de02bb84ab0af51e328c6ae85ab5555397c31277'
    zinit light zsh-users/zsh-completions
  fi
fi

zstyle ':completion:*' matcher-list \
  '' \
  'm:{a-zA-Z}={A-Za-z}' \
  'l:|=* r:|=*'

autoload -Uz compinit compaudit
ZCOMPDUMP="${ZDOTDIR:-$HOME}/.zcompdump"

# The dump can be created by a noninteractive shell or reused without being
# rewritten, so its mtime cannot tell us when a security audit last ran.
_selfishell_completion_needs_audit() {
  setopt localoptions extendedglob
  [[ ! -s "$1" || ! -f "$1.audit" || -L "$1.audit" || -s "$1.audit" ||
     -n "$1.audit"(#qN.mh+24) ]]
}

if [[ -o interactive ]] && _selfishell_completion_needs_audit "$ZCOMPDUMP"; then
  # -i performs one audit and excludes insecure entries without prompting.
  # compinit sets _comp_secure when that audit finds insecure entries.
  unset _comp_secure
  if compinit -i -d "$ZCOMPDUMP"; then
    if [[ "${_comp_secure:-}" == yes ]]; then
      print -u2 "selfishell: insecure completion directories detected; run 'compaudit' for details."
    fi
    # Only an empty regular marker is ours. Preserve user replacements and
    # keep auditing instead of following a link or changing an occupied path.
    if [[ ! -L "$ZCOMPDUMP.audit" && ( ! -e "$ZCOMPDUMP.audit" ||
          ( -f "$ZCOMPDUMP.audit" && ! -s "$ZCOMPDUMP.audit" ) ) ]]; then
      if [[ "${_comp_secure:-}" == yes ]]; then
        # A new shell restores fpath: re-audit until it is clean so an excluded
        # directory cannot shadow a safe function while loading a cached dump.
        command rm -f "$ZCOMPDUMP.audit" 2>/dev/null
      elif [[ -s "$ZCOMPDUMP" ]]; then
        command touch "$ZCOMPDUMP.audit" 2>/dev/null
      fi
    fi
  fi
else
  compinit -C -d "$ZCOMPDUMP"
fi
unfunction _selfishell_completion_needs_audit

if [[ -s "$ZCOMPDUMP" && ( ! -s "$ZCOMPDUMP.zwc" || "$ZCOMPDUMP" -nt "$ZCOMPDUMP.zwc" ) ]]; then
  zcompile "$ZCOMPDUMP"
fi

if (( $+functions[zinit] )); then
  zinit cdreplay -q
fi

if _selfishell_command_path kubectl >/dev/null; then
  _selfishell_kubectl_completion() {
    local completion_source

    if completion_source="$(kubectl completion zsh 2>/dev/null)" &&
       [[ -n "$completion_source" ]]; then
      if eval "$completion_source" && (( $+functions[_kubectl] )); then
        unfunction _selfishell_kubectl_completion
        compdef _kubectl kubectl
        _kubectl "$@"
        return
      fi
    fi
    return 1
  }
  compdef _selfishell_kubectl_completion kubectl
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
