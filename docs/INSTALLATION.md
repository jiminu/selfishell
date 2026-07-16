# Installation

Selfishell supports macOS, native Ubuntu, and Ubuntu on WSL on AMD64 or ARM64.
The public bootstrap installs the CLI in the current user's home directory and
does not require root access.

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install --profile developer
```

The default prefix is `~/.local`. Add `~/.local/bin` to `PATH` if the installer
reports that it is missing. The bootstrap installs only the CLI unless `--setup`
is explicitly supplied. Version discovery prefers the latest stable release and
falls back to the most recently published pre-release when no stable release is
available.

Use an exact release in controlled environments:

```sh
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh |
  bash -s -- --version 1.0.0
selfishell install --profile minimal --yes
```

The archive is downloaded to a temporary directory, checked against the
release's `SHA256SUMS`, and then installed under
`~/.local/share/selfishell/releases/<version>`. Existing non-symbolic CLI paths
are never replaced.

For offline configuration after the CLI is provisioned:

```sh
SELFISHELL_OFFLINE=1 selfishell install --profile developer --yes
```

This skips all package and direct dependency network operations.
