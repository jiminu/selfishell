# Zinit contributes additional definitions, but standard completion does not
# depend on it being installed.
ZINIT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git"

if [[ -s "$ZINIT_HOME/zinit.zsh" ]]; then
  source "$ZINIT_HOME/zinit.zsh"

  if _selfishell_zinit_plugin_ready zsh-users/zsh-completions; then
    # Pinned to the commit recorded for zsh-users/zsh-completions in
    # dependencies.conf; keep the two in sync (see tests/common_zsh_test.bash).
    zinit ice blockf atpull'zinit creinstall -q .' ver'e099c4a2287cd829f43d87fbedb1c5a74791a6e2'
    zinit light zsh-users/zsh-completions
  fi
fi

zstyle ':completion:*' matcher-list \
  '' \
  'm:{a-zA-Z}={A-Za-z}' \
  'l:|=* r:|=*'

autoload -Uz compinit compaudit
ZCOMPDUMP="${ZDOTDIR:-$HOME}/.zcompdump"

# The (#q) glob qualifier is only recognized when EXTENDED_GLOB is set, and it
# is off by default. Without it this test never globs, reports every dump as
# stale, and makes compinit re-run compaudit on every interactive startup --
# about 10ms. Enable the option locally so the once-a-day audit works as
# intended (see tests/common_zsh_test.bash).
_selfishell_zcompdump_is_stale() {
  setopt localoptions extendedglob
  [[ -n "$1"(#qN.mh+24) ]]
}

if [[ ! -o interactive ]]; then
  # -C skips compaudit entirely regardless of -u/-i, so there is no security
  # check to perform (or bypass) on this path.
  compinit -C -d "$ZCOMPDUMP"
elif _selfishell_zcompdump_is_stale "$ZCOMPDUMP"; then
  # Run the real insecure-directory scan ourselves so we can decide how to
  # respond: warn and continue (-i, silent) rather than let compinit's
  # default behavior block startup on a `read -q` prompt, or -u's blanket
  # "treat everything as secure" skip the scan entirely.
  if [[ -n "$(compaudit 2>/dev/null)" ]]; then
    print -u2 "selfishell: insecure completion directories detected; run 'compaudit' for details."
    compinit -i -d "$ZCOMPDUMP"
  else
    compinit -d "$ZCOMPDUMP"
  fi
else
  compinit -C -d "$ZCOMPDUMP"
fi
unfunction _selfishell_zcompdump_is_stale

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
        compdef _kubectl kubectl k
        _kubectl "$@"
        return
      fi
    fi
    return 1
  }
  compdef _selfishell_kubectl_completion kubectl k
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
