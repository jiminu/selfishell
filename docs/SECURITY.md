# Security Model

Selfishell modifies user shell configuration and installs development tools, so
release provenance and preservation of existing files are security boundaries.

- Release and direct-download archives are SHA-256 verified before activation.
- GitHub Release assets have signed Sigstore build-provenance attestations bound
  to the release workflow and artifact digests.
- Direct dependency versions are approved in `dependencies.conf`.
- Managed raw executables are checked against their approved download checksum
  again during installation and tool synchronization.
- mise-managed tool selectors are reviewed in `config/shared/mise.toml`; mise verifies
  checksums or stronger provenance when supported by the selected backend.
- Git dependencies use an approved tag or commit.
- Existing configuration is backed up and tracked before managed replacement.
- Shell startup never installs updates; release metadata can refresh in the
  background. See [update notices](UPDATES.md#status-and-update-notices).
- Default LSP server versions are approved in
  `config/shared/nvim/lua/config/languages.lua`. Mason installs these pins on
  first use, and Selfishell's update tools phase reconciles installed versions
  with them. These pins select server versions, not immutable registry metadata
  or transitive dependencies. Additional servers and other Mason packages remain
  user-managed through `:Mason`.
- Selfishell files are installed without root privileges. Apt may request `sudo`
  for system packages, and Homebrew follows its own privilege model.

SHA-256 detects corruption and asset substitution relative to the published
checksum, while the build-provenance attestation verifies which GitHub workflow
produced an asset. Apt and Homebrew packages follow their configured repository
trust and version policies.

Verify a downloaded release archive with GitHub CLI:

```sh
gh attestation verify selfishell-<version>-<platform>-<architecture>.tar.gz \
  --repo jiminu/selfishell \
  --signer-workflow jiminu/selfishell/.github/workflows/release.yml \
  --source-ref refs/tags/v<version> --deny-self-hosted-runners
```

Without the signer and ref options, any workflow in the repository could have
produced a valid attestation.

Review `install.sh`, use an exact release, and mirror verified artifacts for
high-control environments.

The Go security workflow runs weekly and on manual dispatch. It uses the pinned
compiler and `govulncheck` to scan reachable package and standard-library
vulnerabilities for all four release targets. It runs independently of pull
request CI. Check its latest result before releasing; a clean scan covers known
vulnerabilities and is not a substitute for reviewing lifecycle changes.

Child-command discovery skips relative `PATH` entries. Installer Git commands
discard inherited repository-selection variables such as `GIT_DIR` and
`GIT_WORK_TREE`, while retaining authentication and proxy settings. They set
`GIT_TERMINAL_PROMPT=0` for direct and indirect Git children so failed access to
public dependencies cannot wait for terminal credentials behind progress output.
Apt requests that sudo preserve only the configured proxy variables; proxy values
remain in the environment rather than command-line arguments. Sudo policy still
controls whether preservation is permitted. Dependency
discovery sends its GitHub token through curl's standard input rather than its
command-line arguments.
