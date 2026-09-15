#!/usr/bin/env bash

# Run in a subshell so cleanup's offline/trust settings cannot affect installs.
packages_prune_mise() (
  local platform="$1" dry_run="$2"
  local index package_platform mise_command previous version tracked ignored
  local tracked_config config_tracked=0
  local config="$SELFISHELL_ROOT/config/shared/mise.toml" previous_config=""
  local tools=()

  package_platform="$(platform_package_platform "$platform")" || return
  for ((index = 0; index < ${#PACKAGE_NAMES[@]}; index++)); do
    [[ "${PACKAGE_MANAGERS[$index]}" == mise ]] || continue
    [[ "${PACKAGE_PLATFORMS[$index]}" == all || "${PACKAGE_PLATFORMS[$index]}" == "$package_platform" ]] || continue
    tools+=("${PACKAGE_NAMES[$index]}")
  done
  # An empty argument list means ALL installed tools to mise prune.
  ((${#tools[@]} > 0)) || return 0
  if [[ "$dry_run" == 1 ]]; then
    # Even mise's read-only commands can write tracking/cache metadata.
    printf 'Would prune unused mise versions for: %s (keeping current and tracked project versions, not rollback-only versions).\n' "${tools[*]}"
    return 0
  fi
  if ((${#SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[@]} > 0)); then
    cli_warn "Skipping mise cleanup because some optional packages could not be installed."
    return 0
  fi
  mise_command="$(selfishell_mise_command)" || return

  if release_installation_paths 2>/dev/null; then
    previous="$SELFISHELL_SHARE_DIR/previous"
    if [[ -e "$previous" || -L "$previous" ]]; then
      [[ -L "$previous" ]] || return 1
      previous="$(readlink "$previous")" || return
      version="${previous##*/}"
      selfishell_version_is_valid "$version" || return
      [[ "$previous" == "releases/$version" || "$previous" == "$SELFISHELL_RELEASES_DIR/$version" ]] || return 1
      release_directory_is_valid "$version" || return
      previous_config="$SELFISHELL_RELEASES_DIR/$version/config/shared/mise.toml"
    fi
  fi

  export MISE_OFFLINE=1
  # Trust only the current release directory for this invocation, not HOME.
  [[ -f "$config" && -r "$config" ]] || return 1
  export MISE_TRUSTED_CONFIG_PATHS="${MISE_TRUSTED_CONFIG_PATHS:+$MISE_TRUSTED_CONFIG_PATHS:}${config%/*}"
  export MISE_GLOBAL_CONFIG_FILE="$config"
  ignored="$("$mise_command" -C "$SELFISHELL_ROOT/config/shared" settings get ignored_config_paths)" || return
  if [[ "$ignored" != '[]' ]]; then
    # Tracking entries survive ignored-config filtering; they aren't protection.
    cli_warn "Skipping mise cleanup because ignored_config_paths can exclude retained versions."
    return 1
  fi
  # Previous releases may already be tracked by mise. Ignore their pins only
  # during cleanup; leave the release files and mise's tracking state intact.
  if [[ -n "$previous_config" && "$previous_config" != "$config" && ! "$previous_config" -ef "$config" ]]; then
    # mise canonicalizes ignore paths; a replaced link could exclude a project.
    [[ ! -L "$SELFISHELL_RELEASES_DIR/$version/config" &&
      ! -L "$SELFISHELL_RELEASES_DIR/$version/config/shared" &&
      ! -L "$previous_config" ]] || return 1
    # mise parses this as an OS-separated list of glob-capable paths.
    case "$previous_config" in
      *:* | *'*'* | *'?'* | *'['* | *'{'*)
        cli_warn "Cannot safely exclude the rollback configuration from mise cleanup: $previous_config"
        return 1
        ;;
    esac
    export MISE_IGNORED_CONFIG_PATHS="$previous_config"
  fi
  "$mise_command" -C "${config%/*}" config ls >/dev/null || return
  tracked="$("$mise_command" -C "$SELFISHELL_ROOT/config/shared" config ls --tracked-configs)" || return
  # Require tracked pins despite warning-only failures; mise returns canonical paths.
  while IFS= read -r tracked_config; do
    if [[ "$tracked_config" -ef "$config" ]]; then
      config_tracked=1
      break
    fi
  done <<<"$tracked"
  ((config_tracked)) || return 1
  "$mise_command" -C "$SELFISHELL_ROOT/config/shared" prune --tools --yes "${tools[@]}"
)

packages_install() {
  local platform="$1"
  local dry_run="$2"
  local index
  local package_platform
  local architecture=""
  local required_apt=()
  local optional_apt=()
  local required_formula=()
  local optional_formula=()
  local required_cask=()
  local optional_cask=()
  local required_direct=()
  local optional_direct=()
  local required_mise=()
  local optional_mise=()

  # Consumed by the apt adapter after this module is sourced.
  # shellcheck disable=SC2034
  SELFISHELL_APT_UPDATED=0
  # Consumed by the Homebrew adapter after this module is sourced.
  # shellcheck disable=SC2034
  SELFISHELL_BREW_FORMULAE=""
  # shellcheck disable=SC2034
  SELFISHELL_BREW_CASKS=""
  # shellcheck disable=SC2034
  SELFISHELL_BREW_FORMULAE_READY=0
  # shellcheck disable=SC2034
  SELFISHELL_BREW_CASKS_READY=0
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  package_platform="$(platform_package_platform "$platform")"

  for ((index = 0; index < ${#PACKAGE_NAMES[@]}; index++)); do
    if [[ "${PACKAGE_PLATFORMS[$index]}" != "all" && "${PACKAGE_PLATFORMS[$index]}" != "$package_platform" ]]; then
      continue
    fi

    case "${PACKAGE_REQUIREMENTS[$index]}:${PACKAGE_MANAGERS[$index]}" in
      required:apt) required_apt+=("${PACKAGE_NAMES[$index]}") ;;
      optional:apt) optional_apt+=("${PACKAGE_NAMES[$index]}") ;;
      required:formula) required_formula+=("${PACKAGE_NAMES[$index]}") ;;
      optional:formula) optional_formula+=("${PACKAGE_NAMES[$index]}") ;;
      required:cask) required_cask+=("${PACKAGE_NAMES[$index]}") ;;
      optional:cask) optional_cask+=("${PACKAGE_NAMES[$index]}") ;;
      required:direct) required_direct+=("${PACKAGE_NAMES[$index]}") ;;
      optional:direct) optional_direct+=("${PACKAGE_NAMES[$index]}") ;;
      required:mise) required_mise+=("${PACKAGE_NAMES[$index]}") ;;
      optional:mise) optional_mise+=("${PACKAGE_NAMES[$index]}") ;;
    esac
  done

  ((${#required_apt[@]} == 0)) || apt_install_managed_packages required "$dry_run" "${required_apt[@]}"
  ((${#optional_apt[@]} == 0)) || apt_install_managed_packages optional "$dry_run" "${optional_apt[@]}"
  ((${#required_formula[@]} == 0)) || homebrew_install_packages required formula "$dry_run" "${required_formula[@]}"
  ((${#optional_formula[@]} == 0)) || homebrew_install_packages optional formula "$dry_run" "${optional_formula[@]}"
  ((${#required_cask[@]} == 0)) || homebrew_install_packages required cask "$dry_run" "${required_cask[@]}"
  ((${#optional_cask[@]} == 0)) || homebrew_install_packages optional cask "$dry_run" "${optional_cask[@]}"

  if [[ "$dry_run" == "0" ]] && ((${#required_direct[@]} > 0 || ${#optional_direct[@]} > 0)); then
    architecture="$(detect_architecture)"
  fi

  if ((${#required_direct[@]} > 0)); then
    for index in "${required_direct[@]}"; do
      install_direct_package required "$index" "$dry_run" "$platform" "$architecture"
    done
  fi
  if ((${#optional_direct[@]} > 0)); then
    for index in "${optional_direct[@]}"; do
      install_direct_package optional "$index" "$dry_run" "$platform" "$architecture"
    done
  fi

  ((${#required_mise[@]} == 0)) || install_mise_tools required "$dry_run" "${required_mise[@]}"
  ((${#optional_mise[@]} == 0)) || install_mise_tools optional "$dry_run" "${optional_mise[@]}"

  if ((${#SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[@]} > 0)); then
    cli_warn "Skipped optional packages: ${SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[*]}"
  fi
}
