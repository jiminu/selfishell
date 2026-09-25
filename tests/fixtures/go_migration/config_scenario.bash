#!/usr/bin/env bash

set -euo pipefail

cli="$1"
scenario="$2"
captures="$3"
python="$4"
snapshot="$5"

cd "$HOME"
if [[ "$scenario" == custom ]]; then
  export XDG_CONFIG_HOME="$HOME/xdg/config"
  export XDG_DATA_HOME="$HOME/xdg/data"
  export XDG_STATE_HOME="$HOME/xdg/state"
  export XDG_CACHE_HOME="$HOME/xdg/cache"
fi
if [[ "$scenario" != empty ]]; then
  mkdir -p "$XDG_CONFIG_HOME/nvim"
  printf 'export PERSONAL=kept\r\n' >"$HOME/.zshrc"
  printf 'set number' >"$HOME/.vimrc"
  printf 'personal editor\000bytes\n' >"$XDG_CONFIG_HOME/nvim/init.lua"
  : >"$XDG_CONFIG_HOME/starship.toml"
  chmod 0600 "$HOME/.zshrc" "$HOME/.vimrc"
fi

capture() {
  local name="$1" expected="$2" code=0
  shift 2
  "$cli" "$@" >"$captures/$name.stdout" 2>"$captures/$name.stderr" || code=$?
  printf '%s\n' "$code" >"$captures/$name.status"
  [[ "$code" == "$expected" ]] || {
    cat "$captures/$name.stderr" >&2
    printf 'Unexpected %s status: %s (expected %s)\n' "$name" "$code" "$expected" >&2
    exit 1
  }
  "$python" -B "$snapshot" "$HOME" >"$captures/$name.home.json"
}

"$python" -B "$snapshot" "$HOME" >"$captures/initial.home.json"
capture help 0 help
capture version 0 version
if [[ "$scenario" == malformed-package ]]; then
  printf 'execute unsafe\n' >>"${cli%/bin/selfishell}/packages.conf"
  capture malformed-install 1 install --skip-packages --yes
  cmp "$captures/initial.home.json" "$captures/malformed-install.home.json"
  exit 0
fi
if [[ "$scenario" == malformed-dependency ]]; then
  printf 'execute unsafe\n' >>"${cli%/bin/selfishell}/dependencies.conf"
  capture malformed-install 1 install --skip-packages --yes
  cmp "$captures/initial.home.json" "$captures/malformed-install.home.json"
  exit 0
fi
if [[ "$scenario" == late-preflight ]]; then
  printf '" >>> Selfishell vimrc >>>\n' >>"$HOME/.vimrc"
  "$python" -B "$snapshot" "$HOME" >"$captures/preflight.home.json"
  capture blocked-install 1 install --skip-packages --yes
  cmp "$captures/preflight.home.json" "$captures/blocked-install.home.json"
  exit 0
fi
capture dry-run 0 install --skip-packages --dry-run --yes
cmp "$captures/initial.home.json" "$captures/dry-run.home.json"
capture install 0 install --skip-packages --yes
[[ -f "$XDG_STATE_HOME/selfishell/configured" ]]
[[ -f "$XDG_STATE_HOME/selfishell/resources/user-zshrc.state" ]]
[[ -L "$XDG_CONFIG_HOME/nvim" ]]
capture reinstall 0 install --skip-packages --yes
cmp "$captures/install.home.json" "$captures/reinstall.home.json"
if [[ "$scenario" == purge ]]; then
  prefix="${cli%/bin/selfishell}"
  prefix="${prefix%/share/selfishell/current}"
  "$python" -B "$snapshot" "$prefix" >"$captures/pre-purge.prefix.json"
  capture purge-dry-run 0 uninstall --restore --purge --dry-run --yes
  "$python" -B "$snapshot" "$prefix" >"$captures/purge-dry-run.prefix.json"
  cmp "$captures/pre-purge.prefix.json" "$captures/purge-dry-run.prefix.json"
  rm "$captures/pre-purge.prefix.json" "$captures/purge-dry-run.prefix.json"
  capture purge 0 uninstall --restore --purge --yes
  [[ ! -e "$prefix/bin/selfishell" && ! -L "$prefix/bin/selfishell" ]]
  [[ ! -e "$prefix/share/selfishell" ]]
  "$python" -B "$snapshot" "$prefix" >"$captures/purge.prefix.json"
  exit 0
fi
case "$scenario" in
  changed-file)
    printf 'user changed managed content\n' >"$XDG_CONFIG_HOME/selfishell/zsh/history.zsh"
    capture changed-uninstall 1 uninstall --restore --yes
    exit 0
    ;;
  changed-link)
    rm "$XDG_CONFIG_HOME/nvim"
    ln -s "$XDG_CONFIG_HOME/starship.toml" "$XDG_CONFIG_HOME/nvim"
    capture changed-uninstall 1 uninstall --restore --yes
    exit 0
    ;;
  changed-block)
    printf 'user changed shell config\n' >"$HOME/.zshrc"
    capture changed-uninstall 1 uninstall --restore --yes
    exit 0
    ;;
  pending)
    state="$XDG_STATE_HOME/selfishell/resources/user-nvim.state"
    sed '3s/active/pending/' "$state" >"$state.next"
    mv "$state.next" "$state"
    rm "$XDG_CONFIG_HOME/nvim"
    capture recover 0 install --skip-packages --yes
    capture restore 0 uninstall --restore --yes
    ;;
  *)
    capture restore 0 uninstall --restore --yes
    ;;
esac
if [[ "$scenario" != empty ]]; then
  printf 'export PERSONAL=kept\r\n' | cmp - "$HOME/.zshrc"
  printf 'set number' | cmp - "$HOME/.vimrc"
  printf 'personal editor\000bytes\n' | cmp - "$XDG_CONFIG_HOME/nvim/init.lua"
  [[ -f "$XDG_CONFIG_HOME/starship.toml" && ! -s "$XDG_CONFIG_HOME/starship.toml" ]]
fi
