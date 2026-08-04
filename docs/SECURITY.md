# Security Model

Selfishell modifies user shell configuration and installs development tools, so
release provenance and preservation of existing files are security boundaries.

- Release and direct-download archives are SHA-256 verified before activation.
- GitHub Release assets have signed Sigstore build-provenance attestations bound
  to the release workflow and artifact digests.
- Direct dependency versions are approved in `dependencies.conf`.
- mise-managed tool selectors are reviewed in `common/mise.toml`; mise verifies
  checksums or stronger provenance when supported by the selected backend.
- Git dependencies use an approved tag or commit.
- Existing configuration is backed up and tracked before managed replacement.
- Interactive shell startup performs no network update.
- The managed Neovim configuration reads and executes
  `${XDG_CONFIG_HOME:-~/.config}/selfishell/nvim.user.lua` at every startup if
  present. This file is entirely user-owned (Selfishell only creates a
  starter copy once and never edits it afterward), so it carries the same
  trust as any other file the user places in their own `$XDG_CONFIG_HOME`.
  LSP servers it declares are not version-approved the way the default
  servers are: they install from the Mason registry, unpinned, the first
  time a matching buffer is opened, the same lazy-install path the default
  servers already use.
- Selfishell files are installed without root privileges. Apt may request `sudo`
  for system packages, and Homebrew follows its own privilege model.

SHA-256 detects corruption and asset substitution relative to the published
checksum, while the build-provenance attestation verifies which GitHub workflow
produced an asset. Apt and Homebrew packages follow their configured repository
trust and version policies.

Verify a downloaded release archive with GitHub CLI:

```sh
gh attestation verify selfishell-<version>-<platform>-<architecture>.tar.gz \
  --repo jiminu/selfishell
```

Review `install.sh`, use an exact release, and mirror verified artifacts for
high-control environments.
