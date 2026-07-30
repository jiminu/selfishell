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

## Bootstrap options

### Add the CLI directory to PATH

The default prefix is `~/.local`. If `~/.local/bin` is missing from `PATH`, the
installer prints commands for the current shell and an absolute command that
works immediately. It does not modify shell startup files by default. Pass
`--add-to-path` to add an idempotent, tracked entry only to the current shell's
regular startup file, `~/.bashrc` or `~/.zshrc`, based on the current default
shell. The selected path must be absent or a regular file; symbolic links and
other path types are preserved and rejected:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --add-to-path
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

For offline configuration after the CLI is provisioned:

```sh
SELFISHELL_OFFLINE=1 selfishell install --profile developer --yes
# or
selfishell install --profile developer --skip-packages --yes
```

`SELFISHELL_OFFLINE=1` and `--skip-packages` perform configuration-only
installation without package or network commands.

## Legacy Zsh transition

Selfishell no longer replaces `~/.zshrc` with a managed symbolic link or loads
`~/.config/selfishell/local.zsh`. Existing installations using that legacy model
are intentionally not migrated automatically. Before reinstalling the new
configuration:

```sh
selfishell uninstall --restore --yes
```

Review the preserved `~/.config/selfishell/local.zsh`, copy any settings you
still want directly into the restored `~/.zshrc`, then run `selfishell install`
again. Selfishell will add one marked loader block and leave the rest of
`.zshrc` user-owned. It does not delete `local.zsh`.

Selfishell also adds a marked block to the user-owned `~/.zprofile`. The block
runs `mise activate zsh --shims` when mise is available, allowing login
environments and IDEs such as VS Code to resolve mise-managed tools. Interactive
Zsh keeps using normal mise activation from the managed `.zshrc` configuration.
Untouched blocks are upgraded automatically with new releases. If you edit
inside a block, an interactive install or update offers to back up the full file
and replace only that block, or to skip it. `--yes` preserves the edit and stops.

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

Personal content in `~/.zshrc` and `~/.zprofile` is preserved; uninstall removes
only the intact marked Selfishell blocks.
Packages installed through Apt, Homebrew, or direct tool installers are also
preserved. If `--add-to-path` was used, purge removes the installer's unchanged
PATH entry; a modified entry is preserved and stops the purge for review.
Purge does not automatically remove Zinit or Neovim plugin checkouts.

## Platform notes

- On WSL, install and select a Nerd Font in Windows Terminal or VS Code so
  Starship icons render correctly.
- On macOS, restart Ghostty after installation to apply its configuration.
- Optional packages unavailable on a distribution are reported without
  stopping required setup; missing required packages stop installation.
