# Development Environment

Selfishell installs one development environment. `packages.conf` declares
its packages: Zsh, Git, Vim, Starship, Zinit, Neovim, CLI tools, language
runtimes, compiler tooling, and optional macOS terminal fonts.

Selfishell installs a pinned mise binary and activates it for
interactive Zsh. Selfishell keeps its defaults in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/selfishell.toml` (which is symlinked to `~/.config/mise/conf.d/selfishell.toml` so it is automatically loaded by `mise`); a project's
`mise.toml` can select different tool versions.

Developer tools managed by mise use exact reviewed versions pinned in
`config/shared/mise.toml`, the single source of truth for these versions. This
includes Starship, FZF, Zoxide, Ripgrep, Eza, Bat, jq, Neovim, Tree-sitter CLI, Node.js,
Python, uv, and GitHub CLI on both macOS and Ubuntu. Eza and Bat remain optional;
failure to install either does not stop setup. Projects remain free to override
the defaults in a local `mise.toml`. Updating the defaults requires a normal
Selfishell release and never happens during shell startup.

Mise manages the Starship executable and its version; Selfishell still manages
`starship.toml` and prompt initialization.

Directories that are not writable show a bold red `[ro]` immediately after the path.

The right prompt keeps short-lived command results before the more stable
environment context. Node and Java show their major version (`node:24`,
`java:21`). Python uses Starship's built-in module to show its major/minor
version (`py:3.13`) and, when active, the virtualenv (`py:3.13(myenv)`). Generic
`.venv` and `venv` names use their parent directory name. Python appears only
in detected Python directories or with an active virtualenv.
Kubernetes is disabled by default. If enabled, it uses file/folder detection
and includes the namespace when configured (`k8s:dev/payments`).

Preview without changing the machine:

```sh
selfishell install --dry-run
```

Install the environment:

```sh
selfishell install --yes
```

`selfishell update` uses `packages.conf` to install missing Apt, Homebrew, and directly
managed tools and synchronize mise tools before updating configuration. Apt and
Homebrew retain responsibility for packages still declared through them.
Copies of a tool left from an older release are not removed
automatically; after mise activation, its pinned tool version takes precedence.

Package requirements have two failure policies:

- `required` packages must be available and install successfully;
- `optional` packages are recommended and attempted automatically, but an
  unavailable package or installation failure does not stop the rest of setup.

`optional` does not mean that Selfishell asks about each package. Ghostty is the
separate interactive installation choice on macOS.

On macOS, interactive installation separately asks whether to install Ghostty
and manage its configuration. `--yes` accepts that choice automatically. The
choice is saved and reused by `selfishell update`.

## Neovim workflow

Selfishell includes a pinned Neovim configuration whose leader key
is `Space`. In Normal mode, press `Space` and pause to open which-key. The popup
shows actions available in the current context; continue typing to narrow the
list. Every Selfishell mapping has a description, so which-key remains aligned
with the installed configuration without a separate shortcut list.

Lua, Python, Bash, sh, JSON, YAML, TOML, and Markdown LSP support appears
when a configured server attaches. Neovim's standard LSP mappings remain
available as well.

Additional LSP servers are installed with `:LspInstall <server>`, the standard
mason-lspconfig command; installed servers auto-enable on the next matching
buffer. For a server Selfishell does not manage by default, customize its
settings by adding `~/.config/nvim/after/lsp/<server>.lua` (for example
`after/lsp/rust_analyzer.lua`), returning a config table Neovim's built-in
LSP client merges in (see `:help lsp-config`).

New splits open to the right and below, four lines of context remain above and
below the cursor when possible, commands that would discard unsaved changes ask
for confirmation, and `:substitute` results preview in a split before they are
applied. Bufferline shows open buffers across the top; use `[b` and `]b` to move
between them, and `Space b d` to close the current buffer without closing its
editor window.

When Neovim is available, `vim` resolves to Neovim while `vi` remains the
system editor.
