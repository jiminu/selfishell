# Installation

Selfishell targets macOS 13 or newer, Ubuntu 24.04 LTS, and Ubuntu on WSL 2
on AMD64 or ARM64. The installed CLI is prebuilt and does not require Go.
The public bootstrap installs the CLI in the current user's home directory and
does not require root access.

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install
```

`selfishell install` sets up the complete development environment. It asks for
confirmation; non-interactive runs need `--yes`.

Installation shows the current phase and finishes with a grouped change summary.
See [progress output](UPDATES.md#tools-and-configuration) for details,
including plain output in CI and how tool failures appear.

## Files Selfishell manages

Managed defaults live under `~/.config/selfishell`. `selfishell install`
connects them to these locations; paths under `~/.config` follow
`XDG_CONFIG_HOME`:

| Path | Change | Existing content |
| --- | --- | --- |
| `~/.zshrc`, `~/.zprofile`, `~/.vimrc` | Marked block added at the top | Kept below the block |
| `~/.zshenv` (Ubuntu/WSL only) | Marked block added at the top | Kept below the block |
| `~/.config/ghostty/config.ghostty` (macOS, if Ghostty is chosen) | Marked block added at the top | Kept below the block |
| `~/.config/nvim` | Link to `~/.config/selfishell/nvim` | Moved to a backup |
| `~/.config/starship.toml` | Link to `~/.config/selfishell/starship.toml` | Moved to a backup |
| `~/.config/mise/conf.d/selfishell.toml` | Link to `~/.config/selfishell/mise/selfishell.toml` | Moved to a backup |
| `~/.config/mise/config.toml` | Created empty if absent | Never changed |
| Files under `~/.config/selfishell` | Managed copies, checksummed | Moved to a backup |

A backup sits beside its original path as `<path>.backup.<timestamp>`, with a
numeric suffix when that name is taken, and reinstalling keeps the original
backup. Adding a block makes no backup because the rest of the file stays in
place. A block target that is a symbolic link, such as a `~/.zshrc` managed by
a dotfiles tool, stops installation before any change. See
[Uninstallation](#uninstallation) to remove these changes and restore backups.

Tools install separately: the pinned mise binary at `~/.local/bin/mise`, and
mise tools, Zinit, and Neovim plugins under `~/.local/share`.

## Reinstallation

Before removing an old installation, keep a separate copy of personal
configuration and any backups you need. Use the
[uninstall procedure](#uninstallation) to preview and restore managed paths
before explicitly purging the CLI. Resolve any reported conflicts first; do
not delete configuration or state directories by hand. Then follow the normal
installation steps.

Reinstalling Selfishell does not require removing Homebrew, Apt or mise tools.

## Verification coverage

CI executes Selfishell on macOS ARM64 and Ubuntu 24.04 AMD64. The Linux ARM64
and macOS AMD64 archives are built but not executed, and WSL 2 is covered only
by isolated tests; see [verification coverage](../CONTRIBUTING.md#verification-coverage)
for the exact CI jobs.

## Bootstrap options

Pass options to the bootstrap after `bash -s --`:

| Option | Effect |
| --- | --- |
| `--version VERSION` | Install an exact release; never falls back to the latest one. |
| `--prefix PATH` | Absolute installation prefix; default `~/.local`. |
| `--setup` | Run `selfishell install` after installing the CLI. |
| `--yes` | With `--setup`, pass `--yes` to `selfishell install`. |
| `--skip-packages` | With `--setup`, pass `--skip-packages` to `selfishell install`. |

The bootstrap installs only the CLI unless `--setup` is supplied; without it,
`--yes` and `--skip-packages` are ignored with a warning.
Version discovery prefers the latest stable release and otherwise uses the
newest version tag only after its exact `VERSION` release asset is published.

### Configure the CLI directory in PATH

If `~/.local/bin` is missing from `PATH`, the installer prints commands for the
current shell and an absolute command that works immediately. The installer
never modifies shell startup files. To make the CLI available in future
sessions, add the following line to your shell startup file:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

### Install CLI and environment together

Install the CLI and development environment non-interactively:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --setup --yes
```

When the login shell is not already Zsh, the managed install step offers to
change it to the Zsh listed in `/etc/shells`; `--yes` accepts the offer.
`chsh` then asks for your password on the terminal. Without a terminal, the
install prints the `chsh -s` command to run yourself.

### Install an exact release

Use an exact release in controlled environments:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --version <version>
selfishell install --yes
```

The archive is downloaded to a temporary directory, checked against the
release's `SHA256SUMS`, and then installed under
`~/.local/share/selfishell/releases/<version>`. Existing non-symbolic CLI paths
are never replaced. A later bootstrap installation retains the former active
release for offline rollback and removes older inactive releases.

### Install under a custom prefix

`--prefix PATH` puts the CLI links in `PATH/bin` and releases in
`PATH/share/selfishell`; `update`, `rollback`, and `uninstall --purge` follow
the running CLI's location. The prefix moves only the CLI: configuration,
state, and tools keep their XDG and `~/.local` locations, and the managed Zsh
configuration prepends `~/.local/bin`, not the custom bin directory. Add that
directory to `PATH` in `~/.zshrc`, outside the Selfishell block, so new shells
and update notices find `selfishell`:

```sh
export PATH="/opt/selfishell/bin:$PATH"
```

### Install configuration without network access

For configuration-only installation after the CLI is provisioned:

```sh
selfishell install --skip-packages --yes
```

`--skip-packages` applies managed configuration without installing packages or
tools. See [update modes](UPDATES.md#update-modes) for its effect on updates.

## Zsh integration

`~/.zshrc` is user-owned. Its Selfishell block sources the managed platform
entrypoint; personal aliases, exports, PATH entries, and functions belong
outside that block.

The `~/.zprofile` block runs `mise activate zsh --shims` when mise is
available, allowing login environments and IDEs such as VS Code to resolve
mise-managed tools. Interactive Zsh keeps using normal mise activation from the
managed `.zshrc` configuration. Untouched blocks are upgraded automatically
with new releases; see
[Modified Managed File](TROUBLESHOOTING.md#modified-managed-file) for what
happens if you edit inside one.

On Ubuntu and Ubuntu on WSL, the `~/.zshenv` block contains only
`skip_global_compinit=1` so Ubuntu's system-wide Zsh configuration does not
initialize `compinit` before Selfishell's own. Selfishell does not manage
`~/.zshenv` on macOS.

## Ghostty customization

On macOS, Selfishell manages two Ghostty paths and recognizes an optional third
path for personal overrides:

```text
Selfishell-managed:
  ${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/ghostty/config.ghostty

User-owned entrypoint with a Selfishell-managed block:
  ${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty

Optional, fully user-owned override:
  ${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/user.ghostty
```

The entrypoint contains a marked Selfishell block that includes the managed
defaults and then an optional `user.ghostty`; do not edit inside that block
directly. Write personal settings to `user.ghostty` instead:

```sh
${EDITOR:-vim} "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/user.ghostty"
```

`user.ghostty` loads after the Selfishell defaults, so any key you set there
wins over the corresponding default. Ghostty applies included files after the
file that includes them, so a setting already in `config.ghostty` loses to the
Selfishell default for the same key or keybind trigger; `font-family` values add
fallbacks instead. `selfishell install` lists such settings; move them to
`user.ghostty` to keep them. `user.ghostty` is entirely yours:
Selfishell never creates, modifies, checksums, or deletes it, and its absence
is normal — Ghostty simply has no overrides applied.

See [Environment](ENVIRONMENT.md) for how the Ghostty choice is saved.

## Other terminals

Selfishell configures only Ghostty. The shell and Neovim also work in other
terminals, such as iTerm2, Terminal.app, or Windows Terminal, once the terminal
provides what Ghostty's managed configuration sets up:

- Font: Neovim's pickers and Markdown preview use Nerd Font icons. Select
  MesloLGS Nerd Font Mono, which Selfishell installs on macOS, as the terminal
  font. Ghostty includes these symbols without it.
- Option as Alt: FZF's Alt-C directory jump and other Alt shortcuts need the
  Option key to send Alt. In iTerm2, set Settings → Profiles → Keys → General
  → Left Option key to Esc+; in Terminal.app, enable Settings → Profiles →
  Keyboard → Use Option as Meta key.
- 24-bit color: Neovim draws its theme in 24-bit color. Terminal.app supports
  it from macOS 26; on earlier macOS versions, use iTerm2 or Ghostty.

For light color schemes, see the minimum contrast note in
[Environment](ENVIRONMENT.md); for copying from Neovim over SSH, see
[Clipboard over SSH and on WSL](TROUBLESHOOTING.md#clipboard-over-ssh-and-on-wsl).

## Uninstallation

### Restore configuration

Preview removal before changing files:

```sh
selfishell uninstall --restore --dry-run
```

Remove managed configuration and restore backups with:

```sh
selfishell uninstall --restore
```

If a managed file has been modified since installation, Selfishell stops before
removing anything so that it does not overwrite your changes.
If a recorded backup is missing, `--restore` also stops before removal. Return
the backup to the reported path and retry, or omit `--restore` to remove the
managed configuration without restoring backups.

If restoration is interrupted, rerun `selfishell uninstall --restore` with the
same or a newer CLI. A path already restored is left untouched, including any
subsequent personal edits. Finish this recovery before installing again. A
backup whose destination is occupied is preserved rather than overwritten.

Without `--restore`, uninstall keeps the
[original backups](#files-selfishell-manages) and prints their paths.

### Restore configuration and purge Selfishell

Add `--purge` to also remove the installed CLI, retained releases, cache, and
state. Backups of managed files you had modified stay in
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/backups`:

```sh
selfishell uninstall --restore --purge
```

Personal content in `~/.zshrc`, `~/.zprofile`, and `~/.vimrc`, and in
`~/.zshenv` on Ubuntu/WSL, is preserved; uninstall removes only the intact
marked Selfishell blocks. Packages installed through Apt, Homebrew, or direct
tool installers are also preserved. Purge does not automatically remove Zinit
or Neovim plugin checkouts.

## Platform notes

- On WSL, install and select a Nerd Font in Windows Terminal or VS Code so
  Starship icons render correctly.
- On macOS, restart Ghostty after installation to apply its configuration.
- Optional packages that are unavailable or fail to install are reported
  without stopping required setup; missing required packages stop installation.
