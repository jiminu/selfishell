# Aliases
source "$SELFISHELL_COMMON_DIR/aliases.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

# Writes "$@"'s stdout to $target through a temp file, validated non-empty
# and zsh-syntax-clean before the atomic rename, so a process killed
# mid-generation (a closed terminal, a signal) can never leave a partially
# written, non-empty cache that then gets sourced forever without
# regenerating (the caller's [[ -s ]] check can't tell "empty" from
# "truncated"). Only fits tools whose init output is exactly one command's
# stdout; fzf's fallback-to-a-copied-file shape doesn't, so it stays
# separate below rather than forcing it through this signature.
_selfishell_generate_zsh_cache() {
  local target="$1"
  shift
  local temporary="${target}.tmp.$$.$RANDOM"

  command mkdir -p "${target:h}" 2>/dev/null || return 1

  "$@" >|"$temporary" 2>/dev/null || {
    command rm -f "$temporary"
    return 1
  }
  [[ -s "$temporary" ]] || {
    command rm -f "$temporary"
    return 1
  }
  command zsh -n "$temporary" >/dev/null 2>&1 || {
    command rm -f "$temporary"
    return 1
  }
  command mv -f "$temporary" "$target" || {
    command rm -f "$temporary"
    return 1
  }
}

_selfishell_generate_fzf_cache() {
  local target="$1"
  local temporary="${target}.tmp.$$.$RANDOM"
  local fzf_init

  command mkdir -p "${target:h}" 2>/dev/null || return 1

  if fzf_init="$(fzf --zsh 2>/dev/null)" && [[ -n "$fzf_init" ]]; then
    print -r -- "$fzf_init" >|"$temporary" || {
      command rm -f "$temporary"
      return 1
    }
  elif [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]]; then
    command cp /usr/share/doc/fzf/examples/key-bindings.zsh "$temporary" 2>/dev/null || {
      command rm -f "$temporary"
      return 1
    }
  else
    return 1
  fi

  [[ -s "$temporary" ]] || {
    command rm -f "$temporary"
    return 1
  }
  command zsh -n "$temporary" >/dev/null 2>&1 || {
    command rm -f "$temporary"
    return 1
  }
  command mv -f "$temporary" "$target" || {
    command rm -f "$temporary"
    return 1
  }
}

if _selfishell_zoxide_bin="$(command -v zoxide)"; then
  _selfishell_zoxide_cache="$SELFISHELL_CACHE_DIR/zoxide-init.zsh"
  if [[ ! -s "$_selfishell_zoxide_cache" || "$_selfishell_zoxide_bin" -nt "$_selfishell_zoxide_cache" ]]; then
    _selfishell_generate_zsh_cache "$_selfishell_zoxide_cache" zoxide init zsh
  fi
  [[ -s "$_selfishell_zoxide_cache" ]] && source "$_selfishell_zoxide_cache"
  unset _selfishell_zoxide_cache
fi
unset _selfishell_zoxide_bin

if _selfishell_fzf_bin="$(command -v fzf)"; then
  _selfishell_fzf_cache="$SELFISHELL_CACHE_DIR/fzf-init.zsh"
  if [[ ! -s "$_selfishell_fzf_cache" || "$_selfishell_fzf_bin" -nt "$_selfishell_fzf_cache" ]]; then
    _selfishell_generate_fzf_cache "$_selfishell_fzf_cache"
  fi
  [[ -s "$_selfishell_fzf_cache" ]] && source "$_selfishell_fzf_cache"
  unset _selfishell_fzf_cache
fi
unset _selfishell_fzf_bin

if (($+functions[zinit])); then
  # Pinned to the commits recorded in dependencies.conf; keep the two in
  # sync (see tests/common_zsh_test.bash).
  # fzf-tab must be loaded synchronously (without wait) to ensure ZLE wrapping is applied in the correct order
  if command -v fzf >/dev/null 2>&1 && _selfishell_zinit_plugin_ready Aloxaf/fzf-tab; then
    zinit ice ver'24105b15714bfec37989ed5c5b6e60f572253019'
    zinit light Aloxaf/fzf-tab

    # fzf-tab only reads these styles when a completion actually runs, so the
    # block below adds no forks and no disk I/O to shell startup. Each preview
    # runs in an fzf worker process, so a slow one delays the preview pane
    # alone and never the selection itself. The rules stay per-command on
    # purpose: a catch-all ':fzf-tab:complete:*' would also fire for option
    # flags and other candidates that are not paths, refs, or PIDs.

    # Group headers ([files], [directories], ...) above each candidate block.
    zstyle ':completion:*:descriptions' format '[%d]'

    # fzf-tab exports $realpath for candidates zsh added as files; it holds the
    # full path, while $word is only the part after the common prefix. Both
    # branches cap their output so a deep tree or a large file cannot flood the
    # pane, and both fall back to coreutils when eza/bat are not installed.
    _selfishell_fzf_tab_path_preview='
      if [[ -d "$realpath" ]]; then
        if command -v eza >/dev/null 2>&1; then
          eza --tree --level=2 --color=always "$realpath" 2>/dev/null
        else
          ls -lA "$realpath" 2>/dev/null
        fi | head -n 200
      elif [[ -f "$realpath" ]]; then
        if command -v bat >/dev/null 2>&1; then
          bat --color=always --style=numbers --line-range=:200 "$realpath" 2>/dev/null
        elif command -v batcat >/dev/null 2>&1; then
          batcat --color=always --style=numbers --line-range=:200 "$realpath" 2>/dev/null
        else
          head -n 200 "$realpath" 2>/dev/null
        fi
      fi
    '
    zstyle ':fzf-tab:complete:(cd|z|__zoxide_z):*' fzf-preview "$_selfishell_fzf_tab_path_preview"
    zstyle ':fzf-tab:complete:(cat|bat|batcat|less|nano|vim|nvim|view):*' fzf-preview "$_selfishell_fzf_tab_path_preview"
    unset _selfishell_fzf_tab_path_preview

    # zsh's _git appends the subcommand to the completion context
    # (curcontext=${curcontext%:*}-$line[1]:), so `git switch` completes under
    # `git-switch`. Its candidates include remote branches stripped of their
    # remote (__git_remote_branch_names_noprefix), and `git log feature/x` does
    # not resolve those, so show-ref -- which searches refs/heads before
    # refs/remotes, like git itself -- maps the candidate to a hash first. The
    # $word fallback keeps raw commits and HEAD working, and a candidate that
    # resolves to nothing just previews as an empty pane.
    zstyle ':fzf-tab:complete:git-(switch|checkout):*' fzf-preview '
      ref="$(git show-ref --hash "$word" 2>/dev/null | head -n 1)"
      git log --oneline --decorate --color=always -10 "${ref:-$word}" 2>/dev/null
    '

    # What matters when staging or discarding is the pending change, not the
    # file's contents. Plain `git diff` is the working-tree change, which is
    # what all three commands act on by default and also what zsh offers as
    # candidates here (_git-add completes ls-files --modified, git restore and
    # git diff their changed-in-working-tree files). An option that moves the
    # target, `git restore --staged` above all, is not read: the preview would
    # have to parse the command line, and the candidate is simply previewed
    # empty instead. Untracked files have no diff and preview empty too.
    # $realpath is unset for candidates zsh did not add as files, hence the
    # $word fallback.
    zstyle ':fzf-tab:complete:git-(add|restore|diff):*' fzf-preview \
      'git diff --color=always -- "${realpath:-$word}" 2>/dev/null | head -n 200'

    # _git-stash appends its subcommand the same way _git does, so a stash
    # reference is completed under git-stash-<subcommand>. The bare
    # `git stash <TAB>` level completes subcommand names instead, which resolve
    # to nothing and preview empty. `-p` is explicit rather than relying on a
    # diff option to imply it, so the stash.showStat setting cannot turn the
    # patch back into a diffstat.
    zstyle ':fzf-tab:complete:git-stash-(show|pop|apply|drop|branch):*' fzf-preview \
      'git stash show -p --color=always "$word" 2>/dev/null | head -n 200'

    # `ps -p` with an explicit -o format is the subset BSD (macOS) and procps
    # (Ubuntu) agree on. $USERNAME rather than $USER because zsh always defines
    # it, while $USER comes from the environment and can be missing; `ps -u ''`
    # then swallows the next argument and complains instead of listing.
    zstyle ':completion:*:*:*:*:processes' command "ps -u $USERNAME -o pid,user,comm"
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-preview \
      'ps -p "$word" -o pid,user,%cpu,%mem,command 2>/dev/null'
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-flags --preview-window=down:4:wrap
  fi

  if _selfishell_zinit_plugin_ready zsh-users/zsh-autosuggestions; then
    zinit ice wait'0' lucid ver'85919cd1ffa7d2d5412f6d3fe437ebdbeeec4fc5'
    zinit light zsh-users/zsh-autosuggestions
  fi
  if _selfishell_zinit_plugin_ready zdharma-continuum/fast-syntax-highlighting; then
    zinit ice wait'0' lucid ver'4672ad5dd9ad68a7effc1476d65afb7c584ce2b3'
    zinit light zdharma-continuum/fast-syntax-highlighting
  fi
fi

if _selfishell_starship_bin="$(command -v starship)"; then
  _selfishell_starship_cache="$SELFISHELL_CACHE_DIR/starship-init.zsh"
  if [[ ! -s "$_selfishell_starship_cache" || "$_selfishell_starship_bin" -nt "$_selfishell_starship_cache" ]]; then
    _selfishell_generate_zsh_cache "$_selfishell_starship_cache" starship init zsh
  fi
  [[ -s "$_selfishell_starship_cache" ]] && source "$_selfishell_starship_cache"
  unset _selfishell_starship_cache
fi
unset _selfishell_starship_bin

unset SELFISHELL_CACHE_DIR
