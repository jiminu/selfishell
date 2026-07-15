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
    starship)
      if have_command starship || [[ -x "$HOME/.local/bin/starship" ]]; then
        printf 'Already installed: starship\n'
        return
      fi
      mkdir -p "$HOME/.local/bin"
      curl -fsSL https://starship.rs/install.sh | sh -s -- -y -b "$HOME/.local/bin"
      ;;
    nvm)
      if [[ -s "$HOME/.nvm/nvm.sh" ]]; then
        printf 'Already installed: nvm\n'
        return
      fi
      PROFILE=/dev/null bash -c "$(curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh)"
      ;;
    pyenv)
      if [[ -x "$HOME/.pyenv/bin/pyenv" ]]; then
        printf 'Already installed: pyenv\n'
        return
      fi
      if [[ -e "$HOME/.pyenv" ]]; then
        cli_error "Incomplete pyenv installation found: $HOME/.pyenv"
        if [[ "$requirement" == "optional" ]]; then
          SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$package")
          return 0
        fi
        return 1
      fi
      git clone https://github.com/pyenv/pyenv.git "$HOME/.pyenv"
      ;;
    kubectl)
      local kubectl_version
      local architecture
      local temporary_file

      if have_command kubectl || [[ -x "$HOME/.local/bin/kubectl" ]]; then
        printf 'Already installed: kubectl\n'
        return
      fi
      architecture="$(detect_architecture)"
      kubectl_version="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
      mkdir -p "$HOME/.local/bin"
      temporary_file="$(mktemp "$HOME/.local/bin/kubectl.tmp.XXXXXX")"
      curl -fsSL "https://dl.k8s.io/release/${kubectl_version}/bin/linux/${architecture}/kubectl" -o "$temporary_file"
      chmod 0755 "$temporary_file"
      mv "$temporary_file" "$HOME/.local/bin/kubectl"
      ;;
    zinit)
      if [[ -f "$HOME/.local/share/zinit/zinit.git/zinit.zsh" ]]; then
        printf 'Already installed: zinit\n'
        return
      fi
      mkdir -p "$HOME/.local/share/zinit"
      git clone https://github.com/zdharma-continuum/zinit.git "$HOME/.local/share/zinit/zinit.git"
      ;;
    vundle)
      if [[ -d "$HOME/.vim/bundle/Vundle.vim" ]]; then
        printf 'Already installed: vundle\n'
        return
      fi
      mkdir -p "$HOME/.vim/bundle"
      git clone https://github.com/VundleVim/Vundle.vim.git "$HOME/.vim/bundle/Vundle.vim"
      if have_command vim; then
        vim +PluginInstall +qall
      fi
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
