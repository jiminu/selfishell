#!/usr/bin/env bash

# Duplicate original stdin (FD 0) to FD 3. Command loops such as
# `while ... done < <(selfishell_managed_resources)` redirect FD 0 to their
# process substitution for the loop's duration, which would otherwise make
# real interactive input unreachable from prompts issued inside the loop
# body (see managed_install_file's conflict prompt in lib/managed.sh).
exec 3<&0

# These constants are consumed by command modules after this file is sourced.
# shellcheck disable=SC2034
SELFISHELL_EXIT_OK=0
# shellcheck disable=SC2034
SELFISHELL_EXIT_ERROR=1
SELFISHELL_EXIT_USAGE=2

# Colors for [OK]/[ERROR]-style markers and suggested follow-up commands in
# `doctor` and `status` output. Left empty (and thus a no-op) unless stdout
# is a terminal, so redirected, piped, or test-captured output stays
# byte-identical to plain text.
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

# cli_error writes to stderr, which can be a terminal independently of
# stdout (e.g. `selfishell doctor | tee log.txt` redirects stdout but not
# stderr), so its color needs its own -t 2 check rather than reusing the
# stdout-gated variables above.
if [[ -t 2 && -z "${NO_COLOR:-}" ]]; then
  SELFISHELL_COLOR_RED_STDERR=$'\033[31m'
  SELFISHELL_COLOR_RESET_STDERR=$'\033[0m'
else
  SELFISHELL_COLOR_RED_STDERR=
  SELFISHELL_COLOR_RESET_STDERR=
fi

cli_error() {
  printf '%sselfishell:%s %s\n' "$SELFISHELL_COLOR_RED_STDERR" "$SELFISHELL_COLOR_RESET_STDERR" "$*" >&2
}

have_command() {
  command -v "$1" >/dev/null 2>&1
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

# FD 3 holds a copy of the real stdin made when this file was sourced (see
# `exec 3<&0` above). Checking and reading FD 3 instead of FD 0 keeps this
# check correct even when called from inside a loop that has redirected
# FD 0 away from the terminal. SELFISHELL_TEST_TTY lets tests drive real
# prompt/read logic over a piped FD 3 without an actual terminal attached.
selfishell_is_interactive() {
  [[ -t 3 || -n "${SELFISHELL_TEST_TTY:-}" ]]
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
  case "$answer" in
    y | Y | yes | YES) return 0 ;;
    *)
      printf 'Cancelled.\n'
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}
