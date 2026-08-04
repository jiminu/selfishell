# Profiles

Profiles are cumulative. Choose the smallest profile that covers the machine's
role.

| Profile | Purpose |
| --- | --- |
| `minimal` | Core shell, Zinit, Vim, and macOS terminal fonts |
| `developer` | Minimal plus Neovim 0.12.4, Tree-sitter CLI 0.26.11, Node.js 24.18.0, Python 3.13.14, uv 0.5.21, FZF, Zoxide, Ripgrep, Eza, Bat, jq, and compiler tooling |

`developer` is selected when `--profile` is omitted. Choose `minimal`
explicitly for a lightweight shell setup without the larger development
toolchain.

The `developer` profile installs a pinned mise binary and activates it for
interactive Zsh. Selfishell keeps its defaults in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/selfishell.toml` (which is symlinked to `~/.config/mise/conf.d/selfishell.toml` so it is automatically loaded by `mise`); a project's
`mise.toml` can select different tool versions. Existing NVM, pyenv, and
system-Java installations are not removed.

Built-in mise tools use exact reviewed versions. Projects remain free to
override them in a local `mise.toml`. Updating these defaults requires a normal
Selfishell release and never happens during shell startup.

Preview without changing the machine:

```sh
selfishell install --dry-run
```

Install or change the selected profile explicitly:

```sh
selfishell install --profile minimal --yes
```

The active profile is recorded in the XDG state directory. `selfishell update`
uses that recorded profile to install missing Apt, Homebrew, and directly
managed tools before updating configuration. Apt and Homebrew retain
responsibility for versions of packages they already manage.

Profile package requirements have two failure policies:

- `required` packages must be available and install successfully;
- `optional` packages are recommended and attempted automatically, but an
  unavailable package or installation failure does not stop the rest of setup.

`optional` does not mean that Selfishell asks about each package. Ghostty is the
separate interactive installation choice on macOS.

On macOS, interactive installation separately asks whether to install Ghostty
and manage its configuration. `--yes` accepts that choice automatically. The
choice is saved and reused by `selfishell update`.

The former `kubernetes` and `full` profiles were removed during the beta. A
machine that recorded either profile should run `selfishell install --profile
developer --yes` once to select the new profile structure.

## Neovim workflow

The `developer` profile includes a pinned Neovim configuration whose leader key
is `Space`. In Normal mode, press `Space` and pause to open which-key. The popup
shows actions available in the current context; continue typing to narrow the
list. Every Selfishell mapping has a description, so which-key remains aligned
with the installed configuration without a separate shortcut list.

Lua, Python, Bash, sh, JavaScript, and TypeScript LSP support appears when a
configured server attaches. Neovim's standard LSP mappings remain available as
well.

Additional LSP servers can be declared in
`${XDG_CONFIG_HOME:-~/.config}/selfishell/nvim.user.lua`. This file is
optional and user-owned: `selfishell install --profile developer` creates it
once, with a commented example, if it does not already exist, then never
edits, checksums, or removes it — it survives `selfishell update` and
`selfishell uninstall`. Its schema is `return { servers = { <name> = {
filetypes = { ... } } } }`; `filetypes` is required per server, and the
server name must be one that mason-lspconfig / nvim-lspconfig recognize
(Selfishell does not maintain a catalog of allowed names). A malformed file
produces a startup notification and Neovim falls back to the default servers
(lua_ls, pyright, bashls, ts_ls) rather than blocking startup or having Selfishell
rewrite the file. If a declared server needs an external prerequisite
Selfishell does not manage (currently only `jdtls`, which needs a JDK),
Neovim warns at startup when it is missing from PATH rather than installing
anything.

New splits open to the right and below, four lines of context remain above and
below the cursor when possible, commands that would discard unsaved changes ask
for confirmation, and `:substitute` results preview in a split before they are
applied. Bufferline shows open buffers across the top; use `[b` and `]b` to move
between them, and `Space b d` to close the current buffer without closing its
editor window.

In the `developer` profile, `vim` resolves to Neovim while `vi` remains the
system editor.
