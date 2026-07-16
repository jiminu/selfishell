# Runtime environments. load_nvm is defined by each platform .zshrc.
export NVM_DIR="$HOME/.nvm"

nvm() { load_nvm; nvm "$@" }
node() { load_nvm; command node "$@" }
npm() { load_nvm; command npm "$@" }
npx() { load_nvm; command npx "$@" }

export PYENV_ROOT="$HOME/.pyenv"

if [[ -d "$PYENV_ROOT/bin" ]]; then
  path=("$PYENV_ROOT/bin" $path)
fi

if [[ -d "$PYENV_ROOT/shims" ]]; then
  path=("$PYENV_ROOT/shims" $path)
fi

if (( $+commands[pyenv] )); then
  load_pyenv() {
    local virtualenv_init

    unfunction pyenv load_pyenv 2>/dev/null
    eval "$(command pyenv init - --no-rehash zsh)"
    if virtualenv_init="$(command pyenv virtualenv-init - 2>/dev/null)"; then
      eval "$virtualenv_init"
    fi
  }

  pyenv() {
    load_pyenv
    pyenv "$@"
  }
fi
