#!/usr/bin/env bash

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Parse only, never run the configuration. Avoid mapfile for Bash 3.2.
lua_files=()
while IFS= read -r file; do
  lua_files+=("$file")
done < <(find config/shared/nvim -type f -name '*.lua' | sort)

if command -v nvim >/dev/null 2>&1; then
  parser=nvim
  nvim --clean --headless -l scripts/check-lua.lua "${lua_files[@]}"
elif command -v luac >/dev/null 2>&1; then
  parser=luac
  luac -p "${lua_files[@]}"
else
  # CI's Neovim job puts the pinned nvim on PATH so the check runs there.
  printf 'Skipped Lua syntax check: neither nvim nor luac is on PATH\n'
  exit 0
fi
printf 'Parsed %d Lua files with %s\n' "${#lua_files[@]}" "$parser"
