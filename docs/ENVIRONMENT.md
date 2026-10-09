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
:DapInstall js
```

These install Python's debugpy, Go's Delve and the JavaScript/TypeScript
debugger respectively. The language runtime/toolchain must also be installed.
Mason installs supported adapters
and the bridge registers their available default launch configurations,
including installations completed during the current Neovim session.
`:MasonInstall debugpy`, `:MasonInstall delve` and
`:MasonInstall js-debug-adapter` also work. No debug adapter
is installed merely by opening Neovim or running Selfishell setup.
When F5 or `Space D c` finds no launch configuration, Selfishell shows the
installation command for supported defaults if the adapter is missing.
If an adapter is already installed or registered, it points to launch
configuration help instead. Personal Lua providers and `launch.json` are
checked before showing this guidance.

Save the source files with `:w` (or `:wa` for all edited files) before starting:
debuggers run the files on disk, so unsaved edits can leave breakpoints out of
sync. New launches warn about unsaved changes in the current file and other
loaded files under the working directory. The warning leaves your edits
unsaved and lets the launch proceed; save and restart to debug the new code.
It does not repeat when continuing a paused session or attaching to a process.
In Python, launching an empty file with unsaved code can report
`line 0` / `Invalid cursor line`; terminate the session, save, and launch again.
Set a breakpoint with `F9` (a red circle in the gutter), then press `F5` and
select a launch configuration.
Debug panels open once the session initializes and close when the last session
closes, including disconnects and adapter failures.
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
Stepping follows the execution line in a source window, including after moving
between debug panels. The REPL keeps input focus while the source view follows.
`Space D u` hides all remaining debug panels, even if some were closed with
`:q`; when all panels are hidden, it opens the complete layout.

`:q` closes a window while its source buffer can remain loaded. If all source
windows were closed, terminating the session or hiding panels with `Space D u`
restores an editor for the source buffer. A deleted buffer is not reopened;
an empty editor is created instead.

Terminals that report `Shift+F5` / `Shift+F11` as `F17` / `F23` are supported.
Windows Terminal binds F11 to full screen by default. Remove or change that
binding in Settings > Actions to pass F11 to Neovim, or use `Space D i`.
See [Windows Terminal actions](https://learn.microsoft.com/en-us/windows/terminal/customize-settings/actions#toggle-full-screen).
On a Mac keyboard, use `Fn` (or Globe) with a function key, or enable standard
function keys in Keyboard settings; macOS shortcuts such as Show Desktop can
also intercept F11. See [Apple's function-key guide](https://support.apple.com/102439).

Python's default launch runs the current file. On each launch, Selfishell
selects an executable from `VIRTUAL_ENV`, then `CONDA_PREFIX`, then the current
project's `.venv/bin/python`, then `python3` or `python` on PATH. The adapter's
own Python environment is separate from the interpreter running your program.
Go uses the bridge's package and test launch configurations; run Neovim from
the appropriate project directory or specify the package explicitly.

JavaScript and TypeScript share Microsoft's `js-debug-adapter`. Installing
`:DapInstall js` registers the following defaults for both languages:

| Configuration | Use |
| --- | --- |
| `Node: Launch current file` | Run the open file with Node on PATH. |
| `Node: Launch JavaScript file` | Choose a JavaScript entrypoint, including compiled TypeScript output. |
| `Node: Attach (port)` | Connect to a local Node inspector; the prompt defaults to port 9229. |

Both launch options use the debug UI's Console terminal, so programs using
`readline` or other standard input work. Move to Console and press `i` to
enter input. Press `Ctrl+\` then `Ctrl+n` to return to Normal mode before
using debug shortcuts. Attach keeps the process's existing terminal.

Selfishell's Node 24 can run `.ts`, `.mts` and `.cts` files with erasable type
annotations directly. This does not type-check or apply `tsconfig.json`;
enums, decorators, JSX/TSX and path aliases may need your project's compiler
or runtime. For compiled TypeScript, enable source maps in the project build,
open the original `.ts` file, set a breakpoint and choose
`Node: Launch JavaScript file`. Select the generated `.js`, `.mjs` or `.cjs`
entrypoint. Source maps are enabled, and generated files are searched under
the working directory, excluding `node_modules`. Run the project's build
again after edits; F5 does not build automatically.
For a project that already uses `tsx`, a project launch configuration can
set `"runtimeArgs": ["--import", "tsx"]` to run its TypeScript entrypoint.
Selfishell does not install project runtimes or build dependencies.

For attach, start your program with `node --inspect-brk=127.0.0.1:9229 app.js`
(or the equivalent flags in your project command), then select
`Node: Attach (port)`. Node may pause at entry; F5 continues to your breakpoint.
Framework launches use project-specific configuration. Browser debugging
also needs a browser adapter registration; these defaults target Node.js.
See [Node's TypeScript support](https://nodejs.org/docs/latest-v24.x/api/typescript.html)
and [js-debug options](https://github.com/microsoft/vscode-js-debug/blob/main/OPTIONS.md).

For project-specific entrypoints, arguments, environment variables or attach
settings, use `.vscode/launch.json`. nvim-dap reads it when starting a session;
its `type` must match a registered adapter (for these defaults, `python`,
`delve`, or `pwa-node` / `node` for JS/TS). Only a subset of VS Code's format
is supported; use standard JSON without trailing commas. See `:help dap-launch.json`.

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

To resize a split, focus it with `Ctrl+h/j/k/l`, then press `Space w` in Normal
mode. A hint appears at the bottom: repeat `h` / `l` to move the divider left /
right by five columns, `k` / `j` to move it up / down by two lines, or `=` to
equalize resizable splits. The right / bottom divider is used when available;
at the screen edge, the left / top divider is used instead. For example, `h`
narrows a left sidebar but widens the rightmost editor, and `k` expands a bottom
debug panel upward. `Esc` exits resize mode and restores normal movement keys.
Any other key exits and performs its normal action, so `:` opens the command
line and `Ctrl+h/j/k/l` switches windows immediately.
This also works in nvim-tree and debug panels, using keys that travel through
ordinary terminal and SSH connections. To set an exact width, use
`:vertical resize 40` for 40 columns.

When Neovim is available, `vim` resolves to Neovim while `vi` remains the
system editor. `EDITOR` defaults to `nvim` and `VISUAL` defaults to `EDITOR`,
so external programs such as Lazygit also use Neovim. Existing nonempty values
are preserved; an application's explicit editor setting takes precedence.
