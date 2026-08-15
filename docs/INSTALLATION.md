# Installation

Selfishell supports macOS, native Ubuntu, and Ubuntu on WSL on AMD64 or ARM64.
The public bootstrap installs the CLI in the current user's home directory and
does not require root access.

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install
```

`selfishell install` selects the recommended `developer` profile. Use
`selfishell install --profile minimal` for a lightweight shell setup.

## Verification coverage

Automated verification currently exercises the Ubuntu 24.04 container
installation lifecycle, the configuration lifecycle on a GitHub-hosted macOS
runner, and the pinned Neovim developer lifecycle on Ubuntu. Shell checks run
on Ubuntu and macOS.

The supported-platform list is broader than this runner matrix. WSL and every
advertised architecture are not exercised as separate CI runners, although
their detection and platform-specific behavior have isolated test coverage.

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

### Install CLI and default profile together

Install the CLI and default `developer` profile non-interactively:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --setup --yes
```

The managed install step can also offer to set the current user's login shell to
Zsh when Zsh is installed and the session is interactive.

### Install an exact release

Use an exact release in controlled environments:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --version <version>
selfishell install --profile developer --yes
```

The archive is downloaded to a temporary directory, checked against the
release's `SHA256SUMS`, and then installed under
`~/.local/share/selfishell/releases/<version>`. Existing non-symbolic CLI paths
are never replaced. A later bootstrap installation retains the former active
release for offline rollback and removes older inactive releases.

### Install configuration without network access

For configuration-only installation after the CLI is provisioned:

```sh
selfishell install --profile developer --skip-packages --yes
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

### Restore configuration and purge Selfishell

Add `--purge` to also remove the installed CLI, retained releases, cache, and
state:

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
