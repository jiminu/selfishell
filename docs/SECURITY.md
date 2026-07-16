# Security Model

Selfishell modifies user shell configuration and installs development tools, so
release provenance and preservation of existing files are security boundaries.

- Release and direct-download archives are SHA-256 verified before activation.
- GitHub Release assets have signed Sigstore build-provenance attestations bound
  to the release workflow and artifact digests.
- Direct dependency versions are approved in `dependencies.conf`.
- Git dependencies use an approved tag or commit.
- Existing configuration is backed up and tracked before managed replacement.
- Interactive shell startup performs no network update.
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
