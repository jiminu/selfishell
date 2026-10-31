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

See [Installation](INSTALLATION.md) for setup and
[Updates and rollback](UPDATES.md#tools-and-configuration) for synchronization,
required/optional package behavior and cleanup. Copies of tools left from older
package managers are preserved; after mise activation, its pinned versions take
precedence.

Eza and Bat are optional. Terminal setup has separate saved choices; see
[Ghostty on macOS](INSTALLATION.md#ghostty-on-macos) and
[Windows Terminal on WSL](INSTALLATION.md#windows-terminal-on-wsl).

## Prompt and colors

Directories that are not writable show a red Nerd Font lock icon immediately
after the path.

The right prompt keeps short-lived command results before the more stable
environment context. Node and Java show their major version (`node:26`,
`java:21`). Python uses Starship's built-in module to show its major/minor
version (`py:3.14`) and, when active, the virtualenv (`py:3.14(myenv)`). Generic
`.venv` and `venv` names use their parent directory name. Python appears only
in detected Python directories or with an active virtualenv.
Kubernetes is disabled by default. If enabled, it uses file/folder detection
and includes the namespace when configured (`k8s:dev/payments`).

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

The statusline shows outgoing (green `↑2`) and incoming (red `↓4`) commit counts
beside the branch, joined as `↑2↓4` to match Starship and omitting zero counts.
NvimTree adds incoming file badges such as
`↓M` (modified), `↓D` (deleted), and `↓R` (renamed); directories with incoming
changes show `↓`, including additions that do not exist locally yet. Incoming
modifications are yellow, additions/renames green, and deletions red, following
Starship's Git status colors. Local Git
badges remain separate. Both displays compare against the configured upstream
using the last fetched refs; Neovim never fetches automatically. They refresh
when returning to Neovim or its buffers, and within about five seconds while
Neovim is focused but idle. No counts appear without an upstream or on a detached HEAD.
In partial clones, a rename may appear as deletion/addition when the blobs
needed to detect it are not available locally.

Lua, Python, Bash, sh, JSON, YAML, TOML, and Markdown LSP support appears
when a configured server attaches. Neovim's standard LSP mappings remain
available as well. Completion offers LSP, path, and buffer items: `Tab` and
`Shift-Tab` move through them and `Enter` accepts. Accepting a function adds
its parentheses, a signature window follows the arguments, and `Tab`
completes `:` commands. `Space c a` opens code actions and `Space c f` formats the
buffer when an attached server supports formatting. Diagnostics are marked in
the sign column and underlined; the message appears at the end of the cursor
line. `]d` and `[d` move between diagnostics, and `Ctrl-w d` opens the one under
the cursor in a float. NvimTree underlines files
with errors or warnings, and their parent directories, in the diagnostic color
while keeping the Git name color. Only files a server has analyzed, usually
open ones, are marked.

Files over 1.5 MB, or minified files with very long lines, open with the
`bigfile` filetype shown in the statusline. LSP, Tree-sitter, and the scope
guide stay off for them, while regular syntax highlighting remains.

Selfishell pins eight default LSP servers and synchronizes them through Mason;
see [LSP updates](UPDATES.md#neovim-lsp-servers).

Additional LSP servers are installed with `:LspInstall <server>`, the standard
mason-lspconfig command; installed servers auto-enable on the next matching
buffer. For a server Selfishell does not manage by default, customize its
settings by adding `~/.config/nvim/after/lsp/<server>.lua` (for example
`after/lsp/rust_analyzer.lua`), returning a config table Neovim's built-in
LSP client merges in (see `:help lsp-config`).

### Debugging

Install the adapter for your language, save your source, then use `F9` to set
a breakpoint and `F5` to start. See [Debugging in Neovim](DEBUGGING.md) for
adapter installation, shortcuts, language-specific setup and personal settings.

### Editor behavior

New splits open to the right and below, four lines of context remain above and
below the cursor when possible, commands that would discard unsaved changes ask
for confirmation, and `:substitute` results preview in a split before they are
applied. Bufferline shows open buffers across the top; use `[b` and `]b` to move
between them, and `Space b d` to close the current buffer without closing its
editor window.

To resize a split, focus it with `Ctrl+h/j/k/l`, then press `Space w r` in Normal
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

On macOS, entering Normal mode, starting Neovim, or returning to it selects the
last ASCII keyboard layout (such as ABC), so a Korean, Japanese, or Chinese
input method does not swallow Normal mode commands. Insert mode keeps that
layout; switch back with your usual input source key. Over SSH, the input
source is left alone.

When Neovim is available, `vim` resolves to Neovim while `vi` remains the
system editor. `EDITOR` defaults to `nvim` and `VISUAL` defaults to `EDITOR`,
so external programs such as Lazygit also use Neovim. Existing nonempty values
are preserved; an application's explicit editor setting takes precedence.
