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
  local lock_created_at=0
  local now

  command mkdir -p "$cache_dir" 2>/dev/null || return

  if ! command mkdir "$lock_dir" 2>/dev/null; then
    # A prior refresh may have been killed (e.g. the terminal was closed)
    # before it could remove its own lock, which would otherwise wedge every
    # future check silently forever. If the lock is older than the TTL,
    # treat it as abandoned and take over; a second concurrent recovery
    # racing this one just means two redundant checks, not corruption, since
    # available-version/update-checked-at are still written atomically below.
    zmodload zsh/datetime 2>/dev/null
    now="${EPOCHSECONDS:-$(command date +%s)}"
    [[ -r "$lock_dir/created_at" ]] && lock_created_at="$(<"$lock_dir/created_at")"
    case "$lock_created_at" in
      "" | *[!0-9]*) lock_created_at=0 ;;
    esac
    (( lock_created_at > 0 && now - lock_created_at >= lock_ttl )) || return
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
      print -r -- "$available" >| "$temporary" &&
        command mv -f "$temporary" "$available_file"
    fi

    temporary="$checked_file.tmp.$$.$RANDOM"
    print -r -- "$checked_at" >| "$temporary" &&
      command mv -f "$temporary" "$checked_file"
  } always {
    # Guaranteed to run even if the block above returns early or errors, so
    # a normal failure (e.g. no network) can't leak the lock the same way a
    # killed process does.
    # rm -rf, not rmdir: the lock directory now holds pid/created_at files,
    # so a plain rmdir would silently fail to remove it every time, wedging
    # the very next check behind a "fresh-looking" lock it can never clear.
    command rm -rf "$lock_dir" 2>/dev/null
  }
}

_selfishell_version_is_newer() {
  local candidate="$1"
  local current="$2"
  local version_pattern='^([0-9]+)\.([0-9]+)\.([0-9]+)(-([A-Za-z]+)\.([0-9]+))?$'
  local -a candidate_parts current_parts
  local index

  [[ "$candidate" =~ "$version_pattern" ]] || return 1
  candidate_parts=("${match[@]}")
  [[ "$current" =~ "$version_pattern" ]] || return 1
  current_parts=("${match[@]}")

  for index in 1 2 3; do
    (( candidate_parts[index] > current_parts[index] )) && return 0
    (( candidate_parts[index] < current_parts[index] )) && return 1
  done

  [[ -z "${candidate_parts[4]}" && -n "${current_parts[4]}" ]] && return 0
  [[ -n "${candidate_parts[4]}" && -z "${current_parts[4]}" ]] && return 1
  [[ -z "${candidate_parts[4]}" ]] && return 1
  [[ "${candidate_parts[5]}" == "${current_parts[5]}" ]] || return 1
  (( candidate_parts[6] > current_parts[6] ))
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

  case "${enabled:l}" in
    0 | false | no | off) return ;;
  esac
  _selfishell_command_path selfishell >/dev/null || return

  case "$interval" in
    "" | *[!0-9]*) interval=86400 ;;
  esac

  current="$(_selfishell_current_version)" || return
  if [[ -r "$available_file" ]]; then
    available="$(<"$available_file")"
    if [[ -n "$available" ]] && _selfishell_version_is_newer "$available" "$current"; then
      print -r -- "[Selfishell] $available is available. Run: selfishell update"
    elif [[ -n "$available" ]]; then
      command rm -f "$available_file"
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
