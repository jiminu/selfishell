# zsh-completions only contributes definitions; standard completion does not
# depend on any plugin being installed.
if [[ -d "$SELFISHELL_ZSH_PLUGINS/zsh-completions/src" ]]; then
  fpath=("$SELFISHELL_ZSH_PLUGINS/zsh-completions/src" ${fpath:#${(b)SELFISHELL_ZSH_PLUGINS}/zsh-completions/src})
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

# compinit -C skips rescanning; directory mtimes avoid ~7 ms of file globbing.
# An older directory newly added to fpath may wait for the daily audit.
# That bounded delay is an accepted tradeoff for fast startup.
_selfishell_completion_files_changed() {
  local directory

  for directory in $fpath; do
    [[ "$directory" -nt "$1" ]] && return 0
  done
  return 1
}

# zcompile unlinks and recreates the read-only .zwc in place, so concurrent
# shells fail with "can't write zwc file"; compile privately, then rename.
# mv would move the file into a directory occupying the target, so skip one.
_selfishell_completion_compile() {
  local temporary="$ZCOMPDUMP.tmp.$$.$RANDOM.zwc"

  [[ ! -d "$ZCOMPDUMP.zwc" ]] || return 1
  zcompile "$temporary" "$ZCOMPDUMP" && command mv -f "$temporary" "$ZCOMPDUMP.zwc" || {
    command rm -f "$temporary"
    return 1
  }
}

if [[ -o interactive ]] && { _selfishell_completion_needs_audit "$ZCOMPDUMP" ||
     _selfishell_completion_files_changed "$ZCOMPDUMP"; }; then
  # -i excludes insecure entries; -D rebuilds even when an unaudited dump has
  # the same file count. Recompile immediately to replace same-age bytecode.
  unset _comp_secure
  if compinit -i -d "$ZCOMPDUMP" -D; then
    if [[ "${_comp_secure:-}" == yes ]]; then
      print -u2 "selfishell: insecure completion directories detected; run 'compaudit' for details."
    fi
    # Only an empty regular marker is ours. Preserve user replacements and
    # keep auditing instead of following a link or changing an occupied path.
    if [[ ! -L "$ZCOMPDUMP.audit" && ( ! -e "$ZCOMPDUMP.audit" ||
          ( -f "$ZCOMPDUMP.audit" && ! -s "$ZCOMPDUMP.audit" ) ) ]]; then
      command rm -f "$ZCOMPDUMP.audit" 2>/dev/null
    fi
    # Invalidate the old marker before rebuilding; a failed write must not
    # let the next shell skip the audit and restore insecure fpath entries.
    if compdump && _selfishell_completion_compile && [[ "${_comp_secure:-}" != yes &&
         ! -e "$ZCOMPDUMP.audit" && ! -L "$ZCOMPDUMP.audit" ]]; then
      command touch "$ZCOMPDUMP.audit" 2>/dev/null
    fi
  fi
else
  compinit -C -d "$ZCOMPDUMP"
fi

if [[ -s "$ZCOMPDUMP" && ( ! -s "$ZCOMPDUMP.zwc" || "$ZCOMPDUMP" -nt "$ZCOMPDUMP.zwc" ) ]]; then
  _selfishell_completion_compile
fi
unfunction _selfishell_completion_needs_audit _selfishell_completion_files_changed \
  _selfishell_completion_compile

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
