#!/usr/bin/env bash

install_direct_package() {
  local requirement="$1"
  local package="$2"
  local dry_run="$3"

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s direct package: %s\n' "$requirement" "$package"
    return
  fi

  case "$package" in
    starship | nvm | pyenv | pyenv-virtualenv | kubectl | zinit | vundle)
      local dependency_platform
      case "$(detect_platform)" in ubuntu | ubuntu-wsl) dependency_platform=linux ;; *) dependency_platform="$(detect_platform)" ;; esac
      dependency_install "$package" "$dependency_platform" "$(detect_architecture)"
      ;;
    *)
      cli_error "Unknown direct package: $package"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$package")
        return 0
      fi
      return 1
      ;;
  esac
}

install_vim_plugins() {
  local dry_run="$1"
  local plugin plugin_dir
  local plugins_missing=0

  [[ "${SELFISHELL_OFFLINE:-0}" != "1" ]] || return 0
  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install declared Vim plugins.\n'
    return
  fi
  if ! have_command vim; then
    cli_error "Vim is required to install declared plugins."
    return 1
  fi

  while IFS= read -r plugin; do
    [[ -n "$plugin" ]] || continue
    plugin_dir="${plugin##*/}"
    if [[ ! -d "$HOME/.vim/bundle/$plugin_dir" ]]; then
      plugins_missing=1
      break
    fi
  done < <(awk -F"'" '/^[[:space:]]*Plugin / { print $2 }' "$SELFISHELL_ROOT/common/.vimrc")

  [[ "$plugins_missing" == 1 ]] || return 0

  vim +PluginInstall +qall
}
