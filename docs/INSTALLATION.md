# Installation

Selfishell targets macOS 13 or newer, Ubuntu 24.04 LTS, and Ubuntu on WSL 2
on AMD64 or ARM64. The installed CLI is prebuilt and does not require Go.
The public bootstrap installs the CLI in the current user's home directory and
does not require root access.

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install
```

`selfishell install` sets up the complete development environment.

Installation shows the current phase and finishes with a grouped change summary.
See [progress output and logs](UPDATES.md#tools-and-configuration) for details,
including plain output in CI and how to inspect tool failures.

## Reinstallation

Before removing an old installation, keep a separate copy of personal
configuration and any backups you need. Use the
[uninstall procedure](#uninstallation) to preview and restore managed paths
before explicitly purging the CLI. Resolve any reported conflicts first; do
not delete configuration or state directories by hand. Then follow the normal
installation steps.

Reinstalling Selfishell does not require removing Homebrew, Apt or mise tools.

## Verification coverage

Automated verification has exercised a native Linux/AMD64 archive and full
installation in an Ubuntu 24.04 container, a native macOS/ARM64 archive and
configuration lifecycle on macOS 26.6.2, and the pinned Neovim developer
lifecycle on Ubuntu. The release builder produces four archive formats; only
the host's native archive executes in each job. CI smoke tests install an exact
prebuilt archive on Linux and macOS after the repository gate.

WSL 2, Ubuntu 26.04 and every advertised CPU/OS combination have not been
executed as separate CI runners. Platform selection and configuration behavior
have isolated tests, which are not substitutes for execution on those hosts.

## Bootstrap options

### Configure the CLI directory in PATH

The default prefix is `~/.local`. If `~/.local/bin` is missing from `PATH`, the
installer prints commands for the current shell and an absolute command that
works immediately. The installer never modifies shell startup files. To make
the CLI available in future sessions, add the following line to your shell
startup file:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

The bootstrap installs only the CLI unless `--setup` is explicitly supplied.
Version discovery prefers the latest stable release and otherwise uses the
newest version tag only after its exact `VERSION` release asset is published.

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

### Install configuration without network access

For configuration-only installation after the CLI is provisioned:

```sh
selfishell install --skip-packages --yes
```

`--skip-packages` performs configuration-only installation without package or
network commands.

## Zsh integration

`~/.zshrc` is user-owned. Selfishell manages one bounded loader block that
sources the managed platform entrypoint; personal aliases, exports, PATH
entries, and functions belong outside that block.

Selfishell also adds a marked block to the user-owned `~/.zprofile`. The block
runs `mise activate zsh --shims` when mise is available, allowing login
environments and IDEs such as VS Code to resolve mise-managed tools. Interactive
Zsh keeps using normal mise activation from the managed `.zshrc` configuration.
Untouched blocks are upgraded automatically with new releases; see
[Modified Managed File](TROUBLESHOOTING.md#modified-managed-file) for what
happens if you edit inside one.

On Ubuntu and Ubuntu on WSL, `~/.zshenv` also remains user-owned. Selfishell
manages only a bounded block containing `skip_global_compinit=1` so Ubuntu's
system-wide Zsh configuration does not initialize `compinit` before
Selfishell's own. Selfishell does not manage `~/.zshenv` on macOS.

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
wins over the corresponding default. `user.ghostty` is entirely yours:
Selfishell never creates, modifies, checksums, or deletes it, and its absence
is normal — Ghostty simply has no overrides applied.

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

### Restore configuration and purge Selfishell

Add `--purge` to also remove the installed CLI, retained releases, cache, and
state. Backups of managed files you had modified stay in
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/backups`:

```sh
selfishell uninstall --restore --purge
```

Personal content in `~/.zshrc` and `~/.zprofile`, and in `~/.zshenv` on
Ubuntu/WSL, is preserved; uninstall removes only the intact marked Selfishell
blocks. Packages installed through Apt, Homebrew, or direct tool installers are
also preserved. Purge does not automatically remove Zinit or Neovim plugin
checkouts.

## Platform notes

- On WSL, install and select a Nerd Font in Windows Terminal or VS Code so
  Starship icons render correctly.
- On macOS, restart Ghostty after installation to apply its configuration.
- Optional packages unavailable on a distribution are reported without
  stopping required setup; missing required packages stop installation.
