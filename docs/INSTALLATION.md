# Installation

Selfishell supports macOS, native Ubuntu, and Ubuntu on WSL on AMD64 or ARM64.
The public bootstrap installs the CLI in the current user's home directory and
does not require root access.

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install
```

The default prefix is `~/.local`. If `~/.local/bin` is missing from `PATH`, the
installer prints commands for the current shell and an absolute command that
works immediately. It does not modify shell startup files by default. Pass
`--add-to-path` to add an idempotent, tracked entry to `~/.bashrc` or `~/.zshrc`
based on the current default shell:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --add-to-path
```

The bootstrap installs only the CLI unless `--setup` is explicitly supplied.
Version discovery prefers the latest stable release and otherwise uses the
newest version tag only after its exact `VERSION` release asset is published.

Use an exact release in controlled environments:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --version 1.0.0
selfishell install --profile minimal --yes
```

The archive is downloaded to a temporary directory, checked against the
release's `SHA256SUMS`, and then installed under
`~/.local/share/selfishell/releases/<version>`. Existing non-symbolic CLI paths
are never replaced. A later bootstrap installation retains the former active
release for offline rollback and removes older inactive releases.

For offline configuration after the CLI is provisioned:

```sh
SELFISHELL_OFFLINE=1 selfishell install --profile developer --yes
```

This skips all package and direct dependency network operations.

Remove managed configuration and restore backups with:

```sh
selfishell uninstall --restore
```

Add `--purge` to also remove the installed CLI, retained releases, cache, and
state. Personal `local.zsh` configuration and packages installed through Apt,
Homebrew, or direct tool installers are preserved. If `--add-to-path` was used,
purge removes the installer's unchanged PATH entry; a modified entry is
preserved and stops the purge for review.

```sh
selfishell uninstall --restore --purge
```
