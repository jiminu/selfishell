#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-neovim-e2e.XXXXXX")"
MISE_COMMAND="$(command -v mise 2>/dev/null || true)"
MISE_DATA_ROOT="${MISE_DATA_DIR:-$HOME/.local/share/mise}"

cleanup() {
  rm -rf "$TEST_ROOT"
}

fail() {
  printf 'Neovim E2E failed: %s\n' "$*" >&2
  exit 1
}

verify_mise_global_config_ownership() {
  local selfishell_mise_config
  local user_mise_config
  local mise_config_link
  local selfishell_mise_before

  selfishell_mise_config="$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml"
  user_mise_config="$XDG_CONFIG_HOME/mise/config.toml"
  mise_config_link="$XDG_CONFIG_HOME/mise/conf.d/selfishell.toml"
  selfishell_mise_before="$TEST_ROOT/selfishell-mise-before.toml"

  unset MISE_GLOBAL_CONFIG_FILE
  unset MISE_DEFAULT_CONFIG_FILENAME
  unset MISE_OVERRIDE_CONFIG_FILENAMES

  [[ -f "$selfishell_mise_config" ]] ||
    fail "Selfishell-managed mise config is missing"

  [[ -f "$user_mise_config" ]] ||
    fail "User-owned mise global config is missing"

  [[ -L "$mise_config_link" ]] ||
    fail "Selfishell mise conf.d link is missing"

  cp "$selfishell_mise_config" "$selfishell_mise_before"

  "$MISE_COMMAND" settings set pin true >/dev/null

  [[ -f "$user_mise_config" ]] ||
    fail "mise global settings did not write to the user config"

  grep -Eq \
    '^[[:space:]]*pin[[:space:]]*=[[:space:]]*true[[:space:]]*$' \
    "$user_mise_config" ||
    fail "mise did not persist the global pin setting in the user config"

  cmp -s "$selfishell_mise_before" "$selfishell_mise_config" ||
    fail "mise global settings modified the Selfishell-managed config"

  [[ -L "$mise_config_link" ]] ||
    fail "mise global settings replaced the Selfishell conf.d link"

  [[ "$(readlink "$mise_config_link")" == "$selfishell_mise_config" ]] ||
    fail "mise global settings changed the Selfishell conf.d link target"
}

trap cleanup EXIT HUP INT TERM

command -v nvim >/dev/null 2>&1 || fail "Neovim is unavailable"
command -v tree-sitter >/dev/null 2>&1 || fail "Tree-sitter CLI is unavailable"
[[ -x "$MISE_COMMAND" ]] || fail "mise is unavailable"
command -v git >/dev/null 2>&1 || fail "Git is unavailable"
command -v cc >/dev/null 2>&1 || fail "a C compiler is unavailable"

export HOME="$TEST_ROOT/home"
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export NVIM_LOG_FILE="$TEST_ROOT/nvim.log"
export TMPDIR="$TEST_ROOT/tmp"
export MISE_DATA_DIR="$MISE_DATA_ROOT"
mkdir -p "$HOME/.local/bin" "$TMPDIR"
ln -s "$MISE_COMMAND" "$HOME/.local/bin/mise"

if zsh_path="$(command -v zsh 2>/dev/null)"; then
  export SHELL="$zsh_path"
fi

bash "$ROOT_DIR/bin/selfishell" install --profile developer --skip-packages --yes >/dev/null

verify_mise_global_config_ownership

PATH=/usr/local/bin:/usr/bin:/bin bash -c '
  set -euo pipefail
  SELFISHELL_ROOT="$1"
  export SELFISHELL_ROOT
  source "$SELFISHELL_ROOT/lib/common.sh"
  source "$SELFISHELL_ROOT/lib/paths.sh"
  source "$SELFISHELL_ROOT/lib/dependencies.sh"
  source "$SELFISHELL_ROOT/lib/installers.sh"
  selfishell_initialize_paths
  install_neovim_plugins 0
' _ "$ROOT_DIR"

while read -r type repository revision _ _ source _; do
  [[ "$type" == nvim-plugin ]] || continue
  if [[ "$repository" == folke/lazy.nvim ]]; then
    plugin_dir="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  else
    plugin_name="${source##*/}"
    plugin_name="${plugin_name%.git}"
    plugin_dir="$XDG_DATA_HOME/nvim/lazy/$plugin_name"
  fi
  [[ -d "$plugin_dir/.git" ]] || fail "plugin checkout is missing: $repository"
  [[ "$(git -C "$plugin_dir" rev-parse HEAD)" == "$revision" ]] ||
    fail "plugin revision does not match: $repository"
done <"$ROOT_DIR/dependencies.conf"

[[ -r "$XDG_STATE_HOME/selfishell/nvim/lazy-lock.json" ]] || fail "lazy.nvim runtime lock is missing"
[[ ! -e "$XDG_CONFIG_HOME/selfishell/nvim/lazy-lock.json" ]] || fail "lazy.nvim lock polluted managed configuration"

# nvim-treesitter parsers are no longer bulk-installed ahead of time; config.autocmds
# installs each parser lazily, in the background, the first time its filetype is
# opened. The smoke checks below poll with vim.wait() to give that install time to
# finish instead of asserting the parser is present immediately.
#
# config.autocmds no longer re-fires FileType on the buffer whose install it
# triggered (that used to force rainbow-delimiters, which only attaches from
# its own FileType autocmd, to retry once a parser became available). So the
# first-ever open of a language only proves the parser install and Tree-sitter
# highlighting itself; rainbow-delimiters is checked separately, in a second
# Neovim process opening a Python file whose parser is already installed --
# the ordinary case that doesn't depend on that dropped guarantee.

printf 'terraform { required_version = ">= 1.0" }\n' >"$TEST_ROOT/main.tf"
if ! smoke_output="$(nvim --headless "$TEST_ROOT/main.tf" \
  '+lua local language = vim.treesitter.language.get_lang(vim.bo.filetype); assert(vim.bo.filetype == "tf" or vim.bo.filetype == "terraform", "unexpected filetype: " .. vim.bo.filetype); assert(language == "terraform", "unexpected language: " .. tostring(language)); local parser_ok, parser_error; local ready = vim.wait(60000, function() parser_ok, parser_error = pcall(vim.treesitter.get_parser, 0, language); return parser_ok end, 100); assert(ready, "Tree-sitter parser did not finish auto-installing: " .. tostring(parser_error)); print("Neovim developer smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$smoke_output" >&2
  fail "Terraform Tree-sitter smoke failed"
fi
[[ "$smoke_output" == *'Neovim developer smoke: OK'* ]] || {
  printf '%s\n' "$smoke_output" >&2
  fail "Terraform Tree-sitter smoke did not complete"
}

printf 'def nested(value):\n    return {"items": [(value,)]}\n' >"$TEST_ROOT/main.py"
if ! python_smoke_output="$(nvim --headless "$TEST_ROOT/main.py" \
  '+lua local bufnr = vim.api.nvim_get_current_buf(); assert(vim.bo.filetype == "python", "unexpected filetype: " .. vim.bo.filetype); local parser_ok, parser_error; local parser_ready = vim.wait(60000, function() parser_ok, parser_error = pcall(vim.treesitter.get_parser, bufnr, "python"); return parser_ok end, 100); assert(parser_ready, "Tree-sitter parser did not finish auto-installing: " .. tostring(parser_error)); local highlighter_ready = vim.wait(60000, function() return vim.treesitter.highlighter.active[bufnr] ~= nil end, 100); assert(highlighter_ready, "Tree-sitter highlighting is not active for Python"); print("Python highlighting smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$python_smoke_output" >&2
  fail "Python Tree-sitter smoke failed"
fi
[[ "$python_smoke_output" == *'Python highlighting smoke: OK'* ]] || {
  printf '%s\n' "$python_smoke_output" >&2
  fail "Python highlighting smoke did not complete"
}

# A second, independent Neovim process: the Python parser installed above is
# already on disk, so this exercises the ordinary FileType flow (no in-flight
# install, no dropped-guarantee gap) and confirms rainbow-delimiters attaches
# the way it does for every already-installed language in normal use.
#
# vim.treesitter.start() attaches a LanguageTree but -- like any fresh one --
# doesn't parse it immediately; that happens lazily, normally the next time
# Neovim redraws the screen. rainbow-delimiters' own FileType-time attach (see
# rainbow-delimiters.nvim's plugin/rainbow-delimiters.lua) runs synchronously
# right after and reads whatever tree state exists at that instant, so in a
# real interactive session the redraw that follows startup is what makes the
# first highlight appear; headless has no such redraw to fall back on. Calling
# parser:parse() directly is what a real redraw would trigger internally
# (vim.treesitter.highlighter's decoration-provider callbacks call it too) --
# it fires the same on_changedtree callback rainbow-delimiters already
# registered when it attached, which is what actually populates its marks.
if ! rainbow_smoke_output="$(nvim --headless "$TEST_ROOT/main.py" \
  '+lua local bufnr = vim.api.nvim_get_current_buf(); assert(vim.bo.filetype == "python", "unexpected filetype: " .. vim.bo.filetype); local parser_ok, parser = pcall(vim.treesitter.get_parser, bufnr, "python"); assert(parser_ok, "Python parser was not already installed for the second process"); parser:parse(); local rainbow = require("rainbow-delimiters.lib"); local attached = vim.wait(5000, function() local settings = rainbow.buffers[bufnr]; if not settings then return false end; local marks = vim.api.nvim_buf_get_extmarks(bufnr, rainbow.nsids.python, 0, -1, { details = true }); for _, mark in ipairs(marks) do local hl = mark[4].hl_group; if type(hl) == "string" and hl:find("RainbowDelimiter", 1, true) == 1 then return true end end return false end); assert(attached, "rainbow-delimiters did not highlight a Python buffer with an already-installed parser"); print("Rainbow-delimiters smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$rainbow_smoke_output" >&2
  fail "Rainbow-delimiters smoke failed"
fi
[[ "$rainbow_smoke_output" == *'Rainbow-delimiters smoke: OK'* ]] || {
  printf '%s\n' "$rainbow_smoke_output" >&2
  fail "Rainbow-delimiters smoke did not complete"
}

# Snacks only binds its own enable() call to BufReadPost, which never fires
# for a path that doesn't exist on disk yet -- so a brand-new file opened as
# the first buffer (a real BufNewFile, not BufReadPost) is the case that
# regresses if indent.enable() isn't also called directly after setup. This
# only checks that the module is enabled, not scope detection itself, which
# is covered elsewhere.
if ! bufnewfile_smoke_output="$(nvim --headless "$TEST_ROOT/brand-new.py" \
  '+lua assert(vim.fn.filereadable(vim.fn.expand("%:p")) == 0, "the target file must not already exist for this to be a real BufNewFile check"); assert(require("snacks.indent").enabled == true, "Snacks indent was not enabled for a brand-new file"); print("BufNewFile indent smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$bufnewfile_smoke_output" >&2
  fail "BufNewFile indent smoke failed"
fi
[[ "$bufnewfile_smoke_output" == *'BufNewFile indent smoke: OK'* ]] || {
  printf '%s\n' "$bufnewfile_smoke_output" >&2
  fail "BufNewFile indent smoke did not complete"
}

printf 'PASS: pinned Neovim developer installation\n'
