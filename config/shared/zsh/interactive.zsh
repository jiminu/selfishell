source "$SELFISHELL_COMMON_DIR/aliases.zsh"

# Shell tools configure key bindings before interactive plugins load.
SELFISHELL_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"

# Compare executable identity, not cache age: rollbacks can restore an older
# mtime. Keep the key in the cache itself so it activates with the validated code.
# The generators below consume the key set by this check, without another stat.
_selfishell_zsh_cache_current() {
  local target="$1" binary="${2:A}" header
  local -A info
  _selfishell_zsh_cache_key=""
  zmodload -F zsh/stat b:zstat 2>/dev/null || return 1
  zstat -H info -- "$binary" 2>/dev/null || return 1
  _selfishell_zsh_cache_key="# selfishell-tool ${(q)binary} $info[device] $info[inode] $info[size] $info[mtime] $info[ctime]"
  [[ -s "$target" ]] && IFS= read -r header <"$target" &&
    [[ "$header" == "$_selfishell_zsh_cache_key" ]]
}

# Validate before atomic activation so interrupted generation cannot leave a
# partial cache that subsequent startups would source.
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
  local init="$(<"$temporary")"
  print -r -- "${_selfishell_zsh_cache_key:-}"$'\n'"$init" >|"$temporary" || {
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
  local init="$(<"$temporary")"
  print -r -- "${_selfishell_zsh_cache_key:-}"$'\n'"$init" >|"$temporary" || {
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
  if ! _selfishell_zsh_cache_current "$_selfishell_zoxide_cache" "$_selfishell_zoxide_bin"; then
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
  if ! _selfishell_zsh_cache_current "$_selfishell_fzf_cache" "$_selfishell_fzf_bin"; then
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

    # Keep previews per-command so they do not run for option flags.

    # fzf-tab blanks FZF_DEFAULT_OPTS, so hand it the palette directly.
    # use-fzf-default-opts would forward the rest of the user's variable, and
    # --with-nth defeats the NUL encoding it uses for candidates.
    zstyle ':fzf-tab:*' fzf-flags --color=16

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

    # Preview unstaged changes. Command-line options such as --staged are not parsed.
    zstyle ':fzf-tab:complete:git-(add|restore|diff):*' fzf-preview \
      'git diff --color=always -- "${realpath:-$word}" 2>/dev/null | head -n 200'

    # _git-stash appends its subcommand too, so refs complete under
    # git-stash-<subcommand>; the bare level previews empty. `-p` is explicit
    # so stash.showStat cannot turn the patch back into a diffstat.
    zstyle ':fzf-tab:complete:git-stash-(show|pop|apply|drop|branch):*' fzf-preview \
      'git stash show -p --color=always "$word" 2>/dev/null | head -n 200'

    # Use a ps format shared by BSD and procps; zsh always defines USERNAME.
    zstyle ':completion:*:*:*:*:processes' command "ps -u $USERNAME -o pid,user,comm"
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-preview \
      'ps -p "$word" -o pid,user,%cpu,%mem,command 2>/dev/null'
    # zstyle answers with the most specific matching pattern rather than a
    # union, so this context has to repeat the palette the general rule sets.
    zstyle ':fzf-tab:complete:(kill|ps):argument-rest' fzf-flags --color=16 --preview-window=down:4:wrap
  fi

  # Rebinding ~700 widgets before every prompt cost 6-7 ms; bind once, after
  # syntax highlighting wraps them, so suggestions work from the first prompt.
  ZSH_AUTOSUGGEST_MANUAL_REBIND=1
  if _selfishell_zinit_plugin_ready zsh-users/zsh-autosuggestions; then
    zinit ice wait'0' lucid ver'85919cd1ffa7d2d5412f6d3fe437ebdbeeec4fc5'
    zinit light zsh-users/zsh-autosuggestions
  fi
  if _selfishell_zinit_plugin_ready zdharma-continuum/fast-syntax-highlighting; then
    zinit ice wait'0' lucid ver'4672ad5dd9ad68a7effc1476d65afb7c584ce2b3' \
      atload'(( ! $+functions[_zsh_autosuggest_bind_widgets] )) || _zsh_autosuggest_bind_widgets'
    zinit light zdharma-continuum/fast-syntax-highlighting
  fi
fi

# Sourcing starship's init twice makes its keymap wrapper call itself.
if (( ! $+functions[prompt_starship_precmd] )) && _selfishell_starship_bin="$(command -v starship)"; then
  _selfishell_starship_cache="$SELFISHELL_CACHE_DIR/starship-init.zsh"
  if ! _selfishell_zsh_cache_current "$_selfishell_starship_cache" "$_selfishell_starship_bin"; then
    _selfishell_generate_zsh_cache "$_selfishell_starship_cache" starship init zsh
  fi
  [[ -s "$_selfishell_starship_cache" ]] && source "$_selfishell_starship_cache"
  unset _selfishell_starship_cache
fi
unset _selfishell_starship_bin

unset SELFISHELL_CACHE_DIR _selfishell_zsh_cache_key
