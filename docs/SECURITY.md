# Security Model

Selfishell modifies user shell configuration and installs development tools, so
release provenance and preservation of existing files are security boundaries.

- Release and direct-download archives are SHA-256 verified before activation.
- Direct dependency versions are approved in `dependencies.conf`.
- Git dependencies use an approved tag or commit.
- Existing configuration is backed up and tracked before managed replacement.
- Interactive shell startup performs no network update.
- Selfishell files are installed without root privileges. Apt may request `sudo`
  for system packages, and Homebrew follows its own privilege model.

SHA-256 detects corruption and asset substitution relative to the published
checksum, but it is not publisher authentication. Release signing is not yet
implemented. Apt and Homebrew packages also follow their configured repository
trust and version policies.

Review `install.sh`, use an exact release, and mirror verified artifacts for
high-control environments.
