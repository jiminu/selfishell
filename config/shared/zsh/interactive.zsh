# Aliases
source "$SELFISHELL_COMMON_DIR/aliases.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

# Writes "$@"'s stdout to $target through a temp file, validated non-empty and
# zsh-syntax-clean before the atomic rename: the caller's [[ -s ]] can't tell
# "empty" from "truncated", so a kill mid-generation would otherwise leave a
# partial cache sourced forever. fzf's copied-file fallback doesn't fit this
# shape and stays separate below.
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
  # Scheme 16 keeps fzf to the terminal's own colors, as the prompt does by
  # naming colors. The environment wins, so this is a default, not a policy,
  # and it reaches only Ctrl-T and Ctrl-R.
  export FZF_DEFAULT_OPTS="${FZF_DEFAULT_OPTS:---color=16}"

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

    # These styles are read only when a completion runs, so startup pays
    # nothing, and each preview runs in an fzf worker. Rules stay per-command
    # on purpose: a catch-all would fire for option flags too.

    # fzf-tab blanks FZF_DEFAULT_OPTS, so hand it the palette directly.
    # use-fzf-default-opts would forward the rest of the user's variable, and
    # --with-nth defeats the NUL encoding it uses for candidates.
    zstyle ':fzf-tab:*' fzf-flags --color=16

    # Group headers ([files], [directories], ...) above each candidate block.
    zstyle ':completion:*:descriptions' format '[%d]'

    # $realpath is the full path; $word is only the part after the common
    # prefix. Both branches cap output and fall back to coreutils.
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

    # _git appends the subcommand to the context, so this completes under
    # `git-switch`. Candidates include remote branches stripped of their remote,
    # which `git log` cannot resolve, so show-ref maps them to a hash first.
    # The $word fallback keeps raw commits and HEAD working.
    zstyle ':fzf-tab:complete:git-(switch|checkout):*' fzf-preview '
      ref="$(git show-ref --hash "$word" 2>/dev/null | head -n 1)"
      git log --oneline --decorate --color=always -10 "${ref:-$word}" 2>/dev/null
    '

    # The pending change matters here, not the file's contents, and plain
    # `git diff` is what all three commands act on by default. Options that
    # move the target (`restore --staged`) would need command-line parsing and
    # are not read; those candidates preview empty, as untracked files do.
    zstyle ':fzf-tab:complete:git-(add|restore|diff):*' fzf-preview \
      'git diff --color=always -- "${realpath:-$word}" 2>/dev/null | head -n 200'

    # _git-stash appends its subcommand too, so refs complete under
    # git-stash-<subcommand>; the bare level previews empty. `-p` is explicit
    # so stash.showStat cannot turn the patch back into a diffstat.
    zstyle ':fzf-tab:complete:git-stash-(show|pop|apply|drop|branch):*' fzf-preview \
      'git stash show -p --color=always "$word" 2>/dev/null | head -n 200'

    # An explicit -o format is the subset BSD (macOS) and procps (Ubuntu)
    # agree on. $USERNAME because zsh always defines it, unlike $USER: `ps -u ''`
    # swallows the next argument and complains instead of listing.
    zstyle ':completion:*:*:*:*:processes' command "ps -u $USERNAME -o pid,user,comm"
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-preview \
      'ps -p "$word" -o pid,user,%cpu,%mem,command 2>/dev/null'
    # zstyle answers with the most specific matching pattern rather than a
    # union, so this context has to repeat the palette the general rule sets.
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-flags --color=16 --preview-window=down:4:wrap
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
