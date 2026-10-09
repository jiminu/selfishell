# Development Environment

Selfishell installs one development environment. `packages.conf` declares
its packages: Zsh, Git, Vim, Starship, Zinit, Neovim, CLI tools, language
runtimes, compiler tooling, and optional terminal fonts on macOS and WSL.

Selfishell installs a pinned mise binary and activates it for interactive Zsh.
Its defaults live under `${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/`,
linked into mise's `conf.d/selfishell.toml`. Project-local `mise.toml` files
can override these defaults.

`config/shared/mise.toml` pins the reviewed versions of Starship, FZF, Zoxide,
Ripgrep, Eza, Bat, jq, Neovim, Tree-sitter CLI, Node.js, Python, uv, GitHub CLI,
and Lazygit on both macOS and Ubuntu. These defaults change through Selfishell
releases; shell startup never updates them.

Mise manages the Starship executable and its version; Selfishell still manages
`starship.toml` and prompt initialization.

Directories that are not writable show a red Nerd Font lock icon immediately
after the path.

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

See [Updates and rollback](UPDATES.md) for synchronization and cleanup behavior.
Copies of tools left from older package managers are preserved; after mise
activation, its pinned versions take precedence.

Package requirements have two failure policies:

- `required` packages must be available and install successfully;
- `optional` packages are recommended and attempted automatically, but an
  unavailable package or installation failure does not stop the rest of setup.

Eza and Bat are optional. Ghostty has a separate saved installation choice:
on macOS, the first `selfishell install` asks whether to install it and manage
its configuration. If Ghostty is already present on PATH or in the system or
user Applications directory, the prompt offers configuration only and the app
is preserved. Ghostty is the recommended terminal, so pressing Enter
accepts; `--yes` also accepts, and a non-interactive run without `--yes`
declines. The answer is saved in
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/ghostty` and reused by later
`install` and `update` runs without asking again. To add Ghostty after
declining it, run `selfishell install --ghostty`. To turn it off,
`selfishell uninstall` clears the saved choice, and the next
`selfishell install` asks again. Uninstall leaves the Ghostty app installed.

On WSL, the optional Windows Terminal choice is saved separately in
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/windows-terminal.json`.
It also records the Windows local application-data path, WSL distribution,
settings path, and existing profile GUID. A Windows Terminal fragment sets
only that profile's font and the built-in Dark+ scheme; its path and checksum
are recorded in `windows-terminal-fragment.json`. Tools updates reuse the choice
without asking again; `selfishell install --windows-terminal` enables a
previously declined choice. Uninstall clears the choice and removes an
unchanged fragment, preserving installed fonts. See
[Windows Terminal on WSL](INSTALLATION.md#windows-terminal-on-wsl).

Colors do not depend on Ghostty. The prompt, FZF, completion previews, and
command-line highlighting use the terminal's own palette, and Neovim switches
between the light and dark VS Code theme to match the background the terminal
reports. The bundled Ghostty configuration uses the dark `Dark+` theme; see
[Ghostty customization](INSTALLATION.md#ghostty-customization) to change it.

Many light palettes draw yellow, green, and cyan too faintly on a white
background. In iTerm2, raise Settings → Profiles → Colors → Minimum contrast,
which adjusts only the colors that need it and keeps their hue, and clear
Brighten bold text, which otherwise draws bold text in the lighter bright
colors. Ghostty's `minimum-contrast` instead turns such colors black or white,
including the dim autosuggestion text, so in Ghostty choose a light theme whose
colors read well.

## Neovim workflow

Selfishell includes a pinned Neovim configuration whose leader key
is `Space`. In Normal mode, press `Space` and pause to open which-key. The popup
shows actions available in the current context; continue typing to narrow the
list. Every Selfishell mapping has a description, so which-key remains aligned
with the installed configuration without a separate shortcut list.

Press `Space g g` to open Lazygit for repository-wide Git operations, including
staging, commits, and branch management. You can also run `lazygit` directly
from a terminal. Gitsigns remains available for changes in the current buffer.

Lua, Python, Bash, sh, JSON, YAML, TOML, and Markdown LSP support appears
when a configured server attaches. Neovim's standard LSP mappings remain
available as well.

Selfishell pins seven default LSP servers and synchronizes them through Mason;
see [LSP updates](UPDATES.md#neovim-lsp-servers).

Additional LSP servers are installed with `:LspInstall <server>`, the standard
mason-lspconfig command; installed servers auto-enable on the next matching
buffer. For a server Selfishell does not manage by default, customize its
settings by adding `~/.config/nvim/after/lsp/<server>.lua` (for example
`after/lsp/rust_analyzer.lua`), returning a config table Neovim's built-in
LSP client merges in (see `:help lsp-config`).

### Debugging

Selfishell includes nvim-dap, nvim-dap-ui, nvim-nio and mason-nvim-dap at
approved commits. They load on the first debug key or DAP command. Install
only the adapters you need, then open Neovim from the project root:

```vim
:DapInstall python
:DapInstall delve
```

These install Python's debugpy and Go's Delve respectively. The language
runtime/toolchain must also be installed. Mason installs supported adapters
and the bridge registers their available default launch configurations,
including installations completed during the current Neovim session.
`:MasonInstall debugpy` and `:MasonInstall delve` also work. No debug adapter
is installed merely by opening Neovim or running Selfishell setup.

Set a breakpoint with `F9`, then press `F5` and select a launch configuration.
Debug panels open once the session initializes and close on termination or exit.
Use Normal mode for the following keys:

| Action | Function key | Space shortcut |
| --- | --- | --- |
| Start / continue | `F5` | `Space D c` |
| Toggle breakpoint | `F9` | `Space D b` |
| Step over | `F10` | `Space D o` |
| Step into | `F11` | `Space D i` |
| Step out | `Shift+F11` | `Space D O` |
| Terminate | `Shift+F5` | `Space D t` |
| Toggle debug panels | | `Space D u` |
| Evaluate expression | | `Space D e` (also Visual mode) |

The Space shortcuts also work when a keyboard or terminal intercepts function
keys. Which-key shows the actions and their function-key equivalents. `Space d`
still shows LSP diagnostics; `Ctrl+h/j/k/l` moves between debug windows.

Python's default launch runs the current file. On each launch, Selfishell
selects an executable from `VIRTUAL_ENV`, then `CONDA_PREFIX`, then the current
project's `.venv/bin/python`, then `python3` or `python` on PATH. The adapter's
own Python environment is separate from the interpreter running your program.
Go uses the bridge's package and test launch configurations; run Neovim from
the appropriate project directory or specify the package explicitly.

For project-specific entrypoints, arguments, environment variables or attach
settings, use `.vscode/launch.json`. nvim-dap reads it when starting a session;
its `type` must match a registered adapter (for these defaults, `python` or
`delve`). Only a subset of VS Code's format is supported; use standard JSON
without trailing commas. See `:help dap-launch.json`.

Adapter installation does not guarantee a default launch configuration for
every language. Java, for example, needs JDTLS plus `java-debug-adapter` and
additional JDTLS/DAP integration, commonly through
[nvim-jdtls](https://github.com/mfussenegger/nvim-jdtls#debugger-via-nvim-dap).
`:DapInstall javadbg` installs that package; it does not complete the integration.
Selfishell does not configure Java or framework-specific launch workflows.

For personal adapter settings, add `~/.config/nvim/lua/plugins/dap_user.lua`
(under `$XDG_CONFIG_HOME` when set). Existing lazy.nvim imports load it;
Selfishell does not create, replace or remove this file. For example:

```lua
return {
  {
    "jay-babu/mason-nvim-dap.nvim",
    opts = function(_, opts)
      opts.handlers.python = function(config)
        config.configurations[1].pythonPath = "/absolute/path/to/venv/bin/python"
        require("mason-nvim-dap").default_setup(config)
      end
    end,
  },
}
```

Use overrides for the included plugins; installing additional Neovim plugins
into the managed plugin directory is subject to
[plugin synchronization](UPDATES.md). Adapter updates and removal remain under
`:Mason` control; see [debug adapter updates](UPDATES.md#neovim-debug-adapters).

### Editor behavior

New splits open to the right and below, four lines of context remain above and
below the cursor when possible, commands that would discard unsaved changes ask
for confirmation, and `:substitute` results preview in a split before they are
applied. Bufferline shows open buffers across the top; use `[b` and `]b` to move
between them, and `Space b d` to close the current buffer without closing its
editor window.

When Neovim is available, `vim` resolves to Neovim while `vi` remains the
system editor. `EDITOR` defaults to `nvim` and `VISUAL` defaults to `EDITOR`,
so external programs such as Lazygit also use Neovim. Existing nonempty values
are preserved; an application's explicit editor setting takes precedence.
