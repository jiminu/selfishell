#!/usr/bin/env bash

doctor_ok() {
  printf '%s[OK]%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$*"
}

doctor_error() {
  printf '%s[ERROR]%s %s\n' "$SELFISHELL_COLOR_RED" "$SELFISHELL_COLOR_RESET" "$*"
}

doctor_info() {
  printf '%s[INFO]%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$*"
}

doctor_report_package() {
  local package="$1"
  local manager="$2"
  local requirement="$3"
  local dependency_platform="$4"
  local architecture="$5"

  tool_status_detect "$manager" "$package" "$dependency_platform" "$architecture"
  if [[ "$TOOL_STATUS_INSTALLED" == missing ]]; then
    if [[ "$requirement" == required ]]; then
      doctor_error "Tool: $package is missing ($manager)"
      DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
    else
      doctor_info "Optional tool: $package is not installed ($manager)"
    fi
  else
    doctor_ok "Tool: $package $TOOL_STATUS_INSTALLED ($TOOL_STATUS_SOURCE)"
  fi
}

# Shell startup never downloads a Zsh plugin: it silently skips one whose
# Zinit checkout is missing or incomplete. Without this check a degraded shell
# has no visible cause, because the zinit package itself still reports as
# installed.
doctor_report_zinit_plugins() {
  local data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  local manifest record_type repository revision plugin_dir current_revision
  local missing=()
  local dirty=()
  local drifted=()

  [[ -s "$data_home/zinit/zinit.git/zinit.zsh" ]] || return 0
  manifest="$(dependencies_manifest_path)"
  [[ -r "$manifest" ]] || return 0

  # install_zinit_plugins only verifies HEAD against the manifest for a
  # checkout it just cloned; a checkout that already existed on a later run
  # is never re-verified (never auto-modified, per AGENTS.md's "never
  # overwrite user data" policy for existing paths). This is doctor's only
  # chance to surface a checkout that has since drifted or been modified.
  while read -r record_type repository revision _; do
    [[ "$record_type" == zsh-plugin ]] || continue
    plugin_dir="$data_home/zinit/plugins/${repository//\//---}"
    if [[ ! -d "$plugin_dir/.git" ]]; then
      missing+=("$repository")
    elif [[ -n "$(git -C "$plugin_dir" status --porcelain 2>/dev/null)" ]]; then
      dirty+=("$repository")
    elif ! current_revision="$(git -C "$plugin_dir" rev-parse HEAD 2>/dev/null)" || [[ "$current_revision" != "$revision" ]]; then
      drifted+=("$repository")
    fi
  done <"$manifest"

  if ((${#missing[@]} > 0)); then
    doctor_error "Zsh plugins: ${#missing[@]} not provisioned (${missing[*]})"
    printf "        Run '%sselfishell install%s' to provision them; shell startup never downloads plugins.\n" \
      "$SELFISHELL_COLOR_BOLD" "$SELFISHELL_COLOR_RESET"
    DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
    return
  fi
  if ((${#dirty[@]} > 0)); then
    doctor_error "Zsh plugins: ${#dirty[@]} modified locally (${dirty[*]})"
    printf "        These checkouts have uncommitted changes; Selfishell will not overwrite them automatically.\n"
    DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
  fi
  if ((${#drifted[@]} > 0)); then
    doctor_error "Zsh plugins: ${#drifted[@]} at an unapproved revision (${drifted[*]})"
    printf "        Remove the checkout and run '%sselfishell install%s' to reset it to the approved revision.\n" \
      "$SELFISHELL_COLOR_BOLD" "$SELFISHELL_COLOR_RESET"
    DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
  fi
  ((${#dirty[@]} > 0 || ${#drifted[@]} > 0)) || doctor_ok "Zsh plugins: provisioned"
}

# common/completion.zsh only re-runs the real compaudit scan once a day
# (see SELFISHELL_COMPLETION_CACHE_DIR); this gives an on-demand answer for
# the one completion directory Selfishell itself manages and adds to fpath,
# without depending on that cache or on sourcing the user's full shell
# startup (which would pull in whatever customizations they've added outside
# the managed block).
doctor_report_completion_security() {
  local completion_dir="${XDG_CACHE_HOME:-$HOME/.cache}/selfishell/completions"
  local insecure

  have_command zsh || return 0
  # compaudit exits non-zero when it finds an insecure directory, and this
  # runs under `set -e`, so the substitution's own failure must be caught
  # here rather than left to abort the rest of doctor.
  # compaudit reports every insecure entry across the whole fpath it is
  # given, so fpath is replaced (not prepended) with just our own directory
  # right before calling it -- otherwise this would also surface, and
  # misattribute to $completion_dir, any insecure *system* completion
  # directory this check has no business reporting on. `+X` force-loads
  # compaudit's own definition first, while fpath still has zsh's real
  # default (compaudit is itself autoloaded from there); without it,
  # restricting fpath beforehand would leave compaudit unable to find its
  # own function body.
  insecure="$(
    zsh -f -c '
      [[ -d "$1" ]] || exit 0
      autoload -Uz +X compaudit
      fpath=("$1")
      compaudit
    ' _ "$completion_dir" 2>/dev/null
  )" || true
  if [[ -n "$insecure" ]]; then
    doctor_error "Zsh completion directory has insecure permissions: $completion_dir"
    printf "        Fix with: chmod g-w,o-w %s\n" "$completion_dir"
    DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
    return
  fi
  doctor_ok "Zsh completion directory: secure"
}

command_doctor() {
  require_no_arguments doctor "$@" || return

  local platform
  local architecture
  local package_manager
  local profile profile_platform dependency_platform
  local result="$SELFISHELL_EXIT_OK"

  platform="$(detect_platform)"
  architecture="$(detect_architecture)"

  printf 'Selfishell doctor\n\n'

  if platform_is_supported "$platform"; then
    doctor_ok "Platform: $(platform_label "$platform")"
  else
    doctor_error "Platform: $(platform_label "$platform")"
    case "$platform" in
      unsupported-wsl)
        printf '        Only Ubuntu on WSL is currently supported.\n'
        ;;
      unsupported-linux)
        printf '        Ubuntu is the only supported native Linux distribution.\n'
        ;;
      *)
        printf '        Use macOS, Ubuntu, or Ubuntu on WSL.\n'
        ;;
    esac
    result="$SELFISHELL_EXIT_ERROR"
  fi

  case "$architecture" in
    amd64 | arm64)
      doctor_ok "Architecture: $architecture"
      ;;
    *)
      doctor_error "Architecture: $architecture (supported: amd64, arm64)"
      result="$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  package_manager="$(platform_package_manager "$platform")"
  if [[ "$package_manager" == "unknown" ]]; then
    doctor_error "Package manager: unavailable for this platform"
  elif have_command "$package_manager"; then
    doctor_ok "Package manager: $package_manager"
  else
    doctor_error "Package manager: $package_manager was not found"
    printf "        Run '%sselfishell install%s' to set up the supported toolchain.\n" \
      "$SELFISHELL_COLOR_BOLD" "$SELFISHELL_COLOR_RESET"
    result="$SELFISHELL_EXIT_ERROR"
  fi

  selfishell_initialize_paths
  if [[ -r "$SELFISHELL_STATE_DIR/profile" ]] && platform_is_supported "$platform"; then
    tool_status_reset_cache
    profile="$(<"$SELFISHELL_STATE_DIR/profile")"
    doctor_info "Installed profile: $profile"
    if [[ "$profile" == developer ]]; then
      doctor_info "Developer profile active: Neovim and mise-managed runtimes are enabled."
      if have_command gcc; then
        doctor_ok "C compiler: gcc ($(gcc --version | head -n 1))"
      elif have_command clang; then
        doctor_ok "C compiler: clang ($(clang --version | head -n 1))"
      else
        doctor_error "C compiler: gcc or clang was not found (required for compiling Treesitter parsers)"
        if [[ "$platform" == "macos" ]]; then
          printf "        Install Xcode Command Line Tools by running: %sxcode-select --install%s\n" \
            "$SELFISHELL_COLOR_BOLD" "$SELFISHELL_COLOR_RESET"
        else
          printf "        Install build tools by running: %ssudo apt install build-essential%s\n" \
            "$SELFISHELL_COLOR_BOLD" "$SELFISHELL_COLOR_RESET"
        fi
        result="$SELFISHELL_EXIT_ERROR"
      fi
    fi
    case "$platform" in
      ubuntu | ubuntu-wsl)
        dependency_platform=linux
        profile_platform=ubuntu
        ;;
      *)
        dependency_platform="$platform"
        profile_platform="$platform"
        ;;
    esac
    DOCTOR_RESULT="$result"
    selfishell_scan_profile_packages "$profile" "$dependency_platform" "$architecture" doctor_report_package "$profile_platform"
    doctor_report_zinit_plugins
    doctor_report_completion_security
    result="$DOCTOR_RESULT"
  fi

  return "$result"
}
