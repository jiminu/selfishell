# Prints when a lock went stale: its created_at, else the directory's mtime
# (an older Selfishell, or a writer that died before writing metadata).
# Fails if the lock isn't stale or its age is unknowable -- the caller must
# then leave it alone. zsh/stat, not `stat`: the flags differ across BSD.
_selfishell_update_lock_stale_since() {
  local lock_dir="$1"
  local lock_ttl="$2"
  local now="$3"
  local created_at=""
  local -A lock_stat

  zmodload zsh/stat 2>/dev/null
  [[ -r "$lock_dir/created_at" ]] && created_at="$(<"$lock_dir/created_at")"
  case "$created_at" in
    "" | *[!0-9]* | 0)
      zstat -H lock_stat +mtime -- "$lock_dir" 2>/dev/null || return 1
      created_at="$lock_stat[mtime]"
      ;;
  esac
  (( now - created_at >= lock_ttl )) || return 1
  printf '%s\n' "$created_at"
}

# Read cached release metadata during startup and refresh it in the background.
_selfishell_update_notice_refresh() {
  local cache_dir="$1"
  local checked_at="$2"
  local lock_dir="$cache_dir/update-check.lock"
  local lock_ttl="${SELFISHELL_UPDATE_LOCK_TTL:-600}"
  local available_file="$cache_dir/available-version"
  local checked_file="$cache_dir/update-checked-at"
  local temporary
  local available
  local lock_created_at
  local now

  case "$lock_ttl" in
    "" | *[!0-9]* | 0) lock_ttl=600 ;;
  esac

  command mkdir -p "$cache_dir" 2>/dev/null || return

  if ! command mkdir "$lock_dir" 2>/dev/null; then
    # A killed refresh leaves its lock behind and would wedge every future
    # check, so a lock older than the TTL is treated as abandoned.
    zmodload zsh/datetime 2>/dev/null
    now="${EPOCHSECONDS:-$(command date +%s)}"

    lock_created_at="$(_selfishell_update_lock_stale_since "$lock_dir" "$lock_ttl" "$now")" || return
    # Re-check before reclaiming: a changed signature means a concurrent
    # refresh renewed it. This narrows, but does not close, the race between
    # two reclaimers -- a slip means redundant checks, not corruption, since
    # the writes below are atomic.
    [[ "$(_selfishell_update_lock_stale_since "$lock_dir" "$lock_ttl" "$now")" == "$lock_created_at" ]] || return
    command rm -rf "$lock_dir" 2>/dev/null
    command mkdir "$lock_dir" 2>/dev/null || return
  fi

  {
    zmodload zsh/datetime 2>/dev/null
    print -r -- "$$" >| "$lock_dir/pid" 2>/dev/null
    print -r -- "${EPOCHSECONDS:-$(command date +%s)}" >| "$lock_dir/created_at" 2>/dev/null

    if available="$(command selfishell version --available 2>/dev/null)" &&
       [[ -n "$available" ]]; then
      temporary="$available_file.tmp.$$.$RANDOM"
      if print -r -- "$available" >| "$temporary"; then
        command mv -f "$temporary" "$available_file" || command rm -f "$temporary"
      else
        command rm -f "$temporary"
      fi
    fi

    temporary="$checked_file.tmp.$$.$RANDOM"
    if print -r -- "$checked_at" >| "$temporary"; then
      command mv -f "$temporary" "$checked_file" || command rm -f "$temporary"
    else
      command rm -f "$temporary"
    fi
  } always {
    # Release the lock, including its metadata, even on an early return.
    command rm -rf "$lock_dir" 2>/dev/null
  }
}

_selfishell_version_is_valid() {
  local version="$1"
  local core="$version"
  local prerelease=""
  local numeric_pattern='^(0|[1-9][0-9]*)$'
  local prerelease_pattern='^[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$'
  local identifier
  local -a core_parts prerelease_parts

  if [[ "$version" == *-* ]]; then
    core="${version%%-*}"
    prerelease="${version#*-}"
    [[ -n "$prerelease" && "$prerelease" =~ "$prerelease_pattern" ]] || return 1
  fi

  core_parts=("${(s:.:)core}")
  (( ${#core_parts} == 3 )) || return 1
  for identifier in "${core_parts[@]}"; do
    [[ "$identifier" =~ "$numeric_pattern" ]] || return 1
  done

  [[ -n "$prerelease" ]] || return 0
  prerelease_parts=("${(s:.:)prerelease}")
  for identifier in "${prerelease_parts[@]}"; do
    if [[ "$identifier" == <-> && ${#identifier} -gt 1 && "$identifier" == 0* ]]; then
      return 1
    fi
  done
}

# Bash twin: selfishell_version_is_newer; both read tests/fixtures/version-precedence.txt.
_selfishell_version_is_newer() {
  local LC_ALL=C
  local candidate="$1"
  local current="$2"
  local candidate_core="$candidate"
  local current_core="$current"
  local candidate_prerelease=""
  local current_prerelease=""
  local candidate_identifier current_identifier
  local candidate_numeric current_numeric
  local -a candidate_core_parts current_core_parts candidate_parts current_parts
  local index

  _selfishell_version_is_valid "$candidate" || return 1
  _selfishell_version_is_valid "$current" || return 1

  if [[ "$candidate" == *-* ]]; then
    candidate_core="${candidate%%-*}"
    candidate_prerelease="${candidate#*-}"
  fi
  if [[ "$current" == *-* ]]; then
    current_core="${current%%-*}"
    current_prerelease="${current#*-}"
  fi
  candidate_core_parts=("${(s:.:)candidate_core}")
  current_core_parts=("${(s:.:)current_core}")

  for index in 1 2 3; do
    [[ "${candidate_core_parts[index]}" == "${current_core_parts[index]}" ]] && continue
    (( ${#candidate_core_parts[index]} > ${#current_core_parts[index]} )) && return 0
    (( ${#candidate_core_parts[index]} < ${#current_core_parts[index]} )) && return 1
    [[ "${candidate_core_parts[index]}" > "${current_core_parts[index]}" ]] && return 0
    return 1
  done

  [[ -z "$candidate_prerelease" && -n "$current_prerelease" ]] && return 0
  [[ -n "$candidate_prerelease" && -z "$current_prerelease" ]] && return 1
  [[ -z "$candidate_prerelease" ]] && return 1

  candidate_parts=("${(s:.:)candidate_prerelease}")
  current_parts=("${(s:.:)current_prerelease}")
  for (( index = 1; index <= ${#candidate_parts} || index <= ${#current_parts}; index++ )); do
    (( index <= ${#candidate_parts} )) || return 1
    (( index <= ${#current_parts} )) || return 0
    candidate_identifier="${candidate_parts[index]}"
    current_identifier="${current_parts[index]}"
    [[ "$candidate_identifier" == "$current_identifier" ]] && continue

    candidate_numeric=0
    current_numeric=0
    [[ "$candidate_identifier" == <-> ]] && candidate_numeric=1
    [[ "$current_identifier" == <-> ]] && current_numeric=1
    if (( candidate_numeric && current_numeric )); then
      (( ${#candidate_identifier} > ${#current_identifier} )) && return 0
      (( ${#candidate_identifier} < ${#current_identifier} )) && return 1
      [[ "$candidate_identifier" > "$current_identifier" ]] && return 0
      return 1
    fi
    (( candidate_numeric )) && return 1
    (( current_numeric )) && return 0
    [[ "$candidate_identifier" > "$current_identifier" ]] && return 0
    return 1
  done
  return 1
}

_selfishell_current_version() {
  local executable
  local version_file
  local current_output

  if executable="$(_selfishell_command_path selfishell)"; then
    executable="${executable:A}"
    version_file="${executable:h:h}/VERSION"
    if [[ -r "$version_file" ]]; then
      print -r -- "$(<"$version_file")"
      return
    fi
  fi

  current_output="$(command selfishell version 2>/dev/null)" || return 1
  print -r -- "${current_output#selfishell }"
}

_selfishell_update_notice() {
  local enabled="${SELFISHELL_UPDATE_NOTICE:-1}"
  local interval="${SELFISHELL_UPDATE_CHECK_INTERVAL:-86400}"
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell"
  local available_file="$cache_dir/available-version"
  local checked_file="$cache_dir/update-checked-at"
  local current available checked_at=0 now
  local color_cyan='' color_bold='' color_reset=''

  case "${enabled:l}" in
    0 | false | no | off) return ;;
  esac
  _selfishell_command_path selfishell >/dev/null || return

  case "$interval" in
    "" | *[!0-9]*) interval=86400 ;;
  esac

  if [[ -r "$available_file" ]]; then
    available="$(<"$available_file")"
    if [[ -n "$available" ]]; then
      current="$(_selfishell_current_version)" || return
      if _selfishell_version_is_newer "$available" "$current"; then
        if [[ -t 2 && -z "${NO_COLOR:-}" ]]; then
          color_cyan=$'\033[36m'
          color_bold=$'\033[1m'
          color_reset=$'\033[0m'
        fi
        # stderr: `zsh -i -c` output captured by a script must not include it.
        print -u2 -r -- "${color_cyan}[Selfishell]${color_reset} $available is available. Run: ${color_bold}selfishell update${color_reset}"
      else
        command rm -f "$available_file"
      fi
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
