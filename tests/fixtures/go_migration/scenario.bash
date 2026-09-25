#!/usr/bin/env bash

set -euo pipefail

cli="$1"
scenario="$2"
captures="$3"
python="$4"
snapshot="$5"

# Run each reference in the SAME HOME and release path. Checksums and backup
# references can then be compared literally, without rewriting managed state.
cd "$HOME"
if [[ "$scenario" == existing ]]; then
  mkdir -p "$HOME/.config/nvim"
  printf 'export PERSONAL=kept\r\n' >"$HOME/.zshrc"
  printf 'set number' >"$HOME/.vimrc"
  printf 'personal editor\000bytes\n' >"$HOME/.config/nvim/init.lua"
  : >"$HOME/.config/starship.toml"
  chmod 0600 "$HOME/.zshrc" "$HOME/.vimrc"
fi
"$python" -B "$snapshot" "$HOME" >"$captures/initial.home.json"

capture() {
  local name="$1" expected_status="$2" status=0
  shift 2
  "$cli" "$@" >"$captures/$name.stdout" 2>"$captures/$name.stderr" || status=$?
  printf '%s\n' "$status" >"$captures/$name.status"
  if [[ "$status" != "$expected_status" ]]; then
    cat "$captures/$name.stderr" >&2
    printf 'Unexpected status for %s: %s (expected %s)\n' "$name" "$status" "$expected_status" >&2
    exit 1
  fi
  "$python" -B "$snapshot" "$HOME" >"$captures/$name.home.json"
}

capture help 0 help
capture version 0 version
capture unknown 2 'unknown command with spaces'
capture extra-argument 2 version extra
capture dry-run 0 install --skip-packages --dry-run --yes
cmp "$captures/initial.home.json" "$captures/dry-run.home.json"
capture install 0 install --skip-packages --yes
[[ -f "$HOME/.local/state/selfishell/configured" ]]
[[ -f "$HOME/.local/state/selfishell/resources/user-zshrc.state" ]]
[[ -L "$HOME/.config/nvim" ]]
capture reinstall 0 install --skip-packages --yes
cmp "$captures/install.home.json" "$captures/reinstall.home.json"
capture update-config 0 update --tools-only --skip-packages --yes
capture restore 0 uninstall --restore --yes
if [[ "$scenario" == existing ]]; then
  printf 'export PERSONAL=kept\r\n' | cmp - "$HOME/.zshrc"
  printf 'set number' | cmp - "$HOME/.vimrc"
  printf 'personal editor\000bytes\n' | cmp - "$HOME/.config/nvim/init.lua"
  [[ -f "$HOME/.config/starship.toml" && ! -s "$HOME/.config/starship.toml" ]]
fi
