# Selfishell Milestones

This document is the implementation roadmap. `AGENTS.md` contains the durable
architecture and engineering constraints. Checkboxes describe repository state,
not intent; mark them complete only when their acceptance criteria pass.

## M0 - Baseline and Safety Fixes

Goal: make the current bootstrap safe enough to evolve without losing existing
behavior or user data.

- [x] Add a test harness that always uses a temporary `HOME`.
- [x] Add syntax checks, ShellCheck, and formatting checks.
- [x] Fix backup filename collisions without overwriting previous backups.
- [x] Test `link_file` with files, directories, valid links, and dangling links.
- [x] Distinguish required and optional package failures in the final result.
- [x] Document the current supported platforms and destructive operations.

Acceptance criteria:

- CI runs shell syntax and static analysis checks.
- Two backups created in the same second are both preserved.
- Tests do not read or modify the runner's real shell configuration.
- A missing required package produces a nonzero result; optional omissions are
  summarized clearly.

## M1 - CLI Foundation

Goal: introduce the `selfishell` command without changing the installation model
all at once.

- [ ] Add `bin/selfishell` with help, version, doctor, and explicit exit codes.
- [ ] Provide `sfs` as an optional convenience symlink; do not claim `sf`.
- [ ] Split commands, shared logic, and platform adapters into separate modules.
- [ ] Detect macOS, Ubuntu, Ubuntu on WSL, architecture, and package manager.
- [ ] Keep the entrypoint compatible with the supported macOS shell environment.
- [ ] Preserve `main.sh` as a temporary compatibility wrapper.

Acceptance criteria:

- `selfishell version`, `selfishell help`, and `selfishell doctor` work on every
  supported platform.
- `sfs version` invokes the same implementation as an optional shorthand.
- Unsupported platforms fail with an actionable message.
- Platform detection is covered by tests using mocked system data.

## M2 - Managed Installation and Recovery

Goal: stop depending on the source checkout and make user changes reversible.

- [ ] Install configuration into XDG-compatible managed directories.
- [ ] Track every created, replaced, and backed-up path in state metadata.
- [ ] Implement `selfishell install`, `status`, and `uninstall`.
- [ ] Implement restoration without overwriting files modified after installation.
- [ ] Make every operation idempotent and safe after interruption.
- [ ] Provide `--dry-run`, `--yes`, and non-interactive behavior.

Acceptance criteria:

- The source checkout can be deleted after installation without breaking Zsh.
- A second installation creates no duplicate PATH entries or unnecessary backups.
- Uninstall removes only managed files and can restore the original configuration.
- Dry-run performs no filesystem or package changes.

## M3 - Profiles and Platform Adapters

Goal: make Selfishell useful to different users without forcing every tool on
everyone.

- [ ] Define `minimal`, `developer`, `kubernetes`, and `full` profiles.
- [ ] Move package lists out of command flow and into declarative profile data.
- [ ] Implement consistent Homebrew and apt adapters.
- [ ] Add local/private configuration injection without secrets in this repository.
- [ ] Respect proxy variables and restricted-network environments.

Proposed profile boundaries:

- `minimal`: Zsh, Git, Starship, and core configuration.
- `developer`: minimal plus fzf, zoxide, pyenv, and NVM.
- `kubernetes`: developer plus kubectl and context tools.
- `full`: all supported CLI and desktop integrations.

Acceptance criteria:

- Installing one profile does not install tools exclusive to another profile.
- Required and optional dependencies are reported consistently on both platforms.
- Company-specific extensions can be supplied without modifying tracked files.

## M4 - Versioned Release Bootstrap

Goal: provide one safe installation command for macOS and Ubuntu.

- [ ] Create a small, auditable `install.sh` bootstrap.
- [ ] Build versioned release archives for `amd64` and `arm64` where applicable.
- [ ] Publish SHA-256 checksums with every release.
- [ ] Install releases under `~/.local/share/selfishell/releases/<version>`.
- [ ] Atomically switch the `current` link and expose `~/.local/bin/selfishell`.
- [ ] Link `~/.local/bin/sfs` to `selfishell` as an optional shorthand.
- [ ] Support `--version`, `--prefix`, `--yes`, and optional `--setup`.
- [ ] Handle a missing `~/.local/bin` PATH entry with an actionable message.

Acceptance criteria:

- The same bootstrap command works on all supported platforms.
- A checksum mismatch aborts without modifying the active installation.
- Installing a specific version never silently selects another version.
- The bootstrap installs the CLI only unless setup was explicitly requested.

## M5 - Updates and Rollback

Goal: make updates deliberate, observable, and recoverable.

- [ ] Add a single version manifest for directly managed dependencies.
- [ ] Pin and checksum direct release downloads.
- [ ] Pin Git-based plugins by tag or commit where reproducibility requires it.
- [ ] Implement `selfishell update` for managed tools and configuration.
- [ ] Implement `selfishell self-update` for the CLI release.
- [ ] Retain prior releases and implement `selfishell rollback`.
- [ ] Prevent automatic network updates during interactive shell startup.

Acceptance criteria:

- Failed updates leave the previous CLI and configuration usable.
- Status reports current, approved, and available versions separately.
- Rollback switches to a retained release without downloading it again.
- Tool updates and CLI updates have separate commands and state records.

## M6 - Release Automation and Public Beta

Goal: make releases routine for maintainers and understandable to new users.

- [ ] Run Ubuntu and macOS CI with supported shell versions.
- [ ] Automate tagged GitHub Release archives and checksums.
- [ ] Add end-to-end tests for clean install, upgrade, rollback, and uninstall.
- [ ] Publish installation, profile, security, company, and troubleshooting docs.
- [ ] Add a security policy and vulnerability reporting channel.
- [ ] Perform a beta on at least one clean machine per supported platform.

Acceptance criteria:

- Creating a semantic-version tag produces a tested, installable release without
  manual archive editing.
- The README's primary installation path succeeds on clean supported machines.
- A new contributor can reproduce tests and create a local release candidate from
  repository documentation alone.

## M7 - Optional Package Manager Distribution

Goal: add package-manager convenience only after the core release channel is
stable.

- [ ] Evaluate usage and demand for a Homebrew Tap.
- [ ] Reuse the same signed/checksummed release artifacts in any formula.
- [ ] Evaluate `.deb`, PPA, or an APT repository only if maintenance demand exists.
- [ ] Keep package installation separate from modifying user configuration.

Acceptance criteria:

- Package-manager installation exposes the same CLI as the curl bootstrap.
- Installing or removing the package never silently rewrites user dotfiles.
- Additional distribution channels do not introduce a separate version source.

## Release Readiness Checklist

A public stable release should not be declared until all of the following hold:

- [ ] M0 through M6 are complete.
- [ ] No known path can overwrite an existing backup.
- [ ] Direct downloads are versioned and checksum-verified.
- [ ] Installation, update, rollback, and uninstall have end-to-end coverage.
- [ ] Supported platforms and limitations are documented accurately.
- [ ] No secrets or company-specific infrastructure are present in release files.
