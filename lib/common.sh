#!/usr/bin/env bash

# Duplicate stdin to FD 3: loops like `while ... done < <(...)` redirect FD 0
# for their duration, cutting prompts in the loop body off from the terminal
# (see managed_install_file's conflict prompt in lib/managed.sh).
exec 3<&0

# These constants are consumed by command modules after this file is sourced.
# shellcheck disable=SC2034
SELFISHELL_EXIT_OK=0
# shellcheck disable=SC2034
SELFISHELL_EXIT_ERROR=1
SELFISHELL_EXIT_USAGE=2

# Markers and follow-up commands in `doctor`/`status`. Empty unless stdout is
# a terminal, so piped or test-captured output stays byte-identical.
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_GREEN=$'\033[32m'
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_RED=$'\033[31m'
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_YELLOW=$'\033[33m'
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_CYAN=$'\033[36m'
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_BOLD=$'\033[1m'
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_RESET=$'\033[0m'
else
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_GREEN=
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_RED=
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_YELLOW=
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_CYAN=
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_BOLD=
  # shellcheck disable=SC2034
  SELFISHELL_COLOR_RESET=
fi

# stderr can be a terminal independently of stdout (`doctor | tee log.txt`),
# so this needs its own -t 2 check rather than the stdout-gated vars above.
if [[ -t 2 && -z "${NO_COLOR:-}" ]]; then
  SELFISHELL_COLOR_RED_STDERR=$'\033[31m'
  SELFISHELL_COLOR_YELLOW_STDERR=$'\033[33m'
  SELFISHELL_COLOR_RESET_STDERR=$'\033[0m'
else
  SELFISHELL_COLOR_RED_STDERR=
  SELFISHELL_COLOR_YELLOW_STDERR=
  SELFISHELL_COLOR_RESET_STDERR=
fi

cli_error() {
  printf '%sselfishell:%s %s\n' "$SELFISHELL_COLOR_RED_STDERR" "$SELFISHELL_COLOR_RESET_STDERR" "$*" >&2
}

# git has no stall limit of its own; match selfishell_curl's low-speed abort
# for clones and plugin syncs. Values the user already set win.
selfishell_export_git_speed_limits() {
  local limit="${SELFISHELL_CURL_LOW_SPEED_LIMIT:-1024}"
  local time="${SELFISHELL_CURL_LOW_SPEED_TIME:-30}"

  [[ "$limit" =~ ^[1-9][0-9]*$ && "$time" =~ ^[1-9][0-9]*$ ]] || return 0
  export GIT_HTTP_LOW_SPEED_LIMIT="${GIT_HTTP_LOW_SPEED_LIMIT:-$limit}"
  export GIT_HTTP_LOW_SPEED_TIME="${GIT_HTTP_LOW_SPEED_TIME:-$time}"
}

# A checkout's HEAD commit without starting git: a detached SHA, a loose ref,
# or a packed ref. Anything else (a .git file, say) falls back to git.
selfishell_git_head() {
  local repository="$1"
  local head="" ref sha name

  if [[ -f "$repository/.git/HEAD" ]]; then
    IFS= read -r head <"$repository/.git/HEAD" || true
    if [[ "$head" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]]; then
      printf '%s\n' "$head"
      return 0
    fi
    if [[ "$head" == "ref: refs/"* ]]; then
      ref="${head#ref: }"
      if [[ -f "$repository/.git/$ref" ]]; then
        IFS= read -r sha <"$repository/.git/$ref" || true
        if [[ "$sha" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]]; then
          printf '%s\n' "$sha"
          return 0
        fi
      elif [[ -f "$repository/.git/packed-refs" ]]; then
        while IFS=' ' read -r sha name; do
          if [[ "$name" == "$ref" && "$sha" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]]; then
            printf '%s\n' "$sha"
            return 0
          fi
        done <"$repository/.git/packed-refs"
      fi
    fi
  fi
  git -C "$repository" rev-parse HEAD 2>/dev/null
}

# Tracked files edited or deleted in a checkout; untracked output is ignored.
# The explicit git dir keeps a broken .git from falling back to a parent repo.
selfishell_git_tracked_changes() {
  local repository="$1"
  shift
  GIT_DIR=.git GIT_WORK_TREE=. git -C "$repository" ls-files --deleted --modified -- "$@" 2>/dev/null
}

cli_warn() {
  printf '%sselfishell: warning:%s %s\n' "$SELFISHELL_COLOR_YELLOW_STDERR" "$SELFISHELL_COLOR_RESET_STDERR" "$*" >&2
}

have_command() {
  command -v "$1" >/dev/null 2>&1
}

# An unused path from $1, with an incrementing suffix on collision (mirroring
# install.sh's bootstrap_atomic_link). Names temp/backup paths for atomic
# swaps, so a leftover from a killed run never blocks a retry.
selfishell_unique_path() {
  local base="$1"
  local candidate="$base"
  local suffix=0

  while [[ -e "$candidate" || -L "$candidate" ]]; do
    suffix=$((suffix + 1))
    candidate="${base}.${suffix}"
  done
  printf '%s\n' "$candidate"
}

selfishell_version_is_valid() {
  local version="${1:-}"
  local prerelease identifier
  local identifiers=()

  [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]] ||
    return 1
  [[ "$version" == *-* ]] || return 0

  prerelease="${version#*-}"
  IFS=. read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && "$identifier" != 0 && "$identifier" == 0* ]]; then
      return 1
    fi
  done
}

# Numeric identifiers without leading zeroes: a longer one is larger, and
# equal lengths compare as strings, which cannot overflow arithmetic.
selfishell_numeric_identifier_compare() {
  if ((${#1} != ${#2})); then
    ((${#1} > ${#2})) && return 0
    return 1
  fi
  [[ "$1" > "$2" ]]
}

# SemVer precedence, kept equal to update-notice.zsh's
# _selfishell_version_is_newer by tests/fixtures/version-precedence.txt.
selfishell_version_is_newer() {
  local LC_ALL=C
  local candidate="$1" current="$2"
  local candidate_prerelease="" current_prerelease=""
  local candidate_parts=() current_parts=()
  local index=0 candidate_identifier current_identifier

  if ! selfishell_version_is_valid "$candidate" || ! selfishell_version_is_valid "$current"; then
    return 1
  fi
  [[ "$candidate" != *-* ]] || candidate_prerelease="${candidate#*-}"
  [[ "$current" != *-* ]] || current_prerelease="${current#*-}"

  IFS=. read -r -a candidate_parts <<<"${candidate%%-*}"
  IFS=. read -r -a current_parts <<<"${current%%-*}"
  for index in 0 1 2; do
    [[ "${candidate_parts[index]}" == "${current_parts[index]}" ]] && continue
    selfishell_numeric_identifier_compare "${candidate_parts[index]}" "${current_parts[index]}"
    return
  done

  [[ -z "$candidate_prerelease" && -n "$current_prerelease" ]] && return 0
  [[ -n "$candidate_prerelease" ]] || return 1
  [[ -n "$current_prerelease" ]] || return 1

  IFS=. read -r -a candidate_parts <<<"$candidate_prerelease"
  IFS=. read -r -a current_parts <<<"$current_prerelease"
  for ((index = 0; ; index++)); do
    ((index < ${#candidate_parts[@]})) || return 1
    ((index < ${#current_parts[@]})) || return 0
    candidate_identifier="${candidate_parts[index]}"
    current_identifier="${current_parts[index]}"
    [[ "$candidate_identifier" == "$current_identifier" ]] && continue

    if [[ "$candidate_identifier" =~ ^[0-9]+$ && "$current_identifier" =~ ^[0-9]+$ ]]; then
      selfishell_numeric_identifier_compare "$candidate_identifier" "$current_identifier"
      return
    fi
    [[ "$candidate_identifier" =~ ^[0-9]+$ ]] && return 1
    [[ "$current_identifier" =~ ^[0-9]+$ ]] && return 0
    [[ "$candidate_identifier" > "$current_identifier" ]]
    return
  done
}

selfishell_curl() {
  local mode="$1"
  local connect_timeout="${SELFISHELL_CURL_CONNECT_TIMEOUT:-10}"
  local low_speed_limit="${SELFISHELL_CURL_LOW_SPEED_LIMIT:-1024}"
  local low_speed_time="${SELFISHELL_CURL_LOW_SPEED_TIME:-30}"
  local metadata_max_time="${SELFISHELL_CURL_METADATA_MAX_TIME:-15}"
  local value
  local arguments=()
  shift

  for value in "$connect_timeout" "$low_speed_limit" "$low_speed_time" "$metadata_max_time"; do
    case "$value" in
      "" | *[!0-9]* | 0)
        cli_error "Selfishell curl timeout and speed settings must be positive integers."
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
  done

  arguments=(
    --connect-timeout "$connect_timeout"
    --speed-limit "$low_speed_limit"
    --speed-time "$low_speed_time"
  )
  case "$mode" in
    metadata) arguments+=(--max-time "$metadata_max_time") ;;
    transfer) ;;
    *)
      cli_error "Unknown Selfishell curl mode: $mode"
      return "$SELFISHELL_EXIT_USAGE"
      ;;
  esac

  curl -fsSL "${arguments[@]}" "$@"
}

require_no_arguments() {
  local command="$1"
  shift

  if (("$#" > 0)); then
    cli_error "$command does not accept arguments"
    return "$SELFISHELL_EXIT_USAGE"
  fi
}

# FD 3 is the copy of real stdin from `exec 3<&0` above; reading it instead of
# FD 0 stays correct inside a loop that redirected FD 0 away from the terminal.
# SELFISHELL_TEST_TTY lets tests drive this over a pipe with no terminal.
selfishell_is_interactive() {
  [[ -t 3 || -n "${SELFISHELL_TEST_TTY:-}" ]]
}

# Matches an affirmative prompt answer (y/Y/yes/YES). Pair with
# selfishell_answer_is_no for the opposite polarity -- an empty answer is
# neither, so callers decide what a bare Enter means for their own prompt.
selfishell_answer_is_yes() {
  case "$1" in
    y | Y | yes | YES) return 0 ;;
    *) return 1 ;;
  esac
}

selfishell_answer_is_no() {
  case "$1" in
    n | N | no | NO) return 0 ;;
    *) return 1 ;;
  esac
}

confirm_action() {
  local prompt="$1"
  local assume_yes="$2"
  local dry_run="$3"
  local answer=""

  if [[ "$dry_run" == "1" || "$assume_yes" == "1" ]]; then
    return 0
  fi

  if ! selfishell_is_interactive; then
    cli_error "Confirmation requires an interactive terminal; use --yes."
    return "$SELFISHELL_EXIT_USAGE"
  fi

  printf '%s [y/N] ' "$prompt"
  IFS= read -r answer <&3
  if selfishell_answer_is_yes "$answer"; then
    return 0
  fi
  printf '%sCancelled.%s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET"
  return "$SELFISHELL_EXIT_ERROR"
}
