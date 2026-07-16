# Selfishell Agent Guide

This file is the durable project context for coding agents. Read it before
changing the repository, then read `docs/MILESTONES.md` to find the current
delivery stage.

## Project Intent

Selfishell provides a consistent, fast Zsh development environment for personal
machines, other developers, and company-managed new computers.

The product goals, in priority order, are:

1. Installation must be simple.
2. Maintenance and releases must be predictable.
3. The same user experience should work across supported platforms.

The intended user experience is:

```sh
curl -fsSL https://selfishell.dev/install.sh | bash
selfishell install
selfishell doctor
```

The domain is provisional until the project has a real distribution endpoint.
Do not add a dead production URL to executable code.

## Current State

The repository currently contains Bash bootstrap scripts, shared Zsh settings,
Starship configuration, aliases, Vim configuration, and Ghostty configuration.

- `bootstrap.sh` is the temporary legacy full-setup entrypoint.
- `legacy/macos.sh` installs packages with Homebrew.
- `legacy/ubuntu.sh` installs packages with apt and direct downloads.
- `legacy/common.sh` contains shared legacy installation helpers.
- `common/common.zsh` contains shared interactive shell initialization.
- Configuration files are currently linked directly from the checkout.

The `selfishell` CLI, managed configuration lifecycle, declarative profiles, and
managed apt/Homebrew/direct package installation now exist. The versioned release
bootstrap, reproducible dependency manifest, explicit updates, rollback, and
artifact builder also exist. Tagged release publication is automated; public
beta verification remains a manual release gate.

Implemented CLI commands are `help`, `version`, `doctor`, `install`, `status`,
`update`, `rollback`, and `uninstall`. `bootstrap.sh` intentionally
remains a legacy full-bootstrap wrapper while the managed package/profile layer
is developed.

## Product Decisions

- Use one installation method for macOS and Ubuntu: a small curl-delivered
  bootstrap script backed by versioned GitHub Release archives.
- The bootstrap installs only the Selfishell CLI. It must not silently replace
  user configuration or install the full development environment.
- The canonical command is `selfishell`. Also provide `sfs` as an optional
  convenience symlink for interactive use.
- Do not use `sf` as a command name because it is too generic and has a higher
  collision risk with existing developer tools.
- Official documentation, automation, and error messages should use
  `selfishell`; `sfs` is only a shorthand.
- `selfishell install` performs user environment setup explicitly.
- Install Selfishell without root privileges under XDG-compatible user paths.
- Keep package-manager distribution, such as Homebrew Tap or APT, optional and
  defer it until the release process is stable.
- Start with macOS, Ubuntu, and Ubuntu on WSL. Other distributions are not
  officially supported until they have platform adapters and CI coverage.
- Keep a minimal cross-platform core. Optional tools belong in profiles.
- Treat existing user files as user data: back up safely, never overwrite a
  backup, and provide restoration and uninstall paths.
- Separate Selfishell CLI updates from managed tool/configuration updates.
- Prefer reproducible releases. Pin and checksum direct downloads; use normal
  package-manager resolution for system packages unless there is a documented
  compatibility requirement.

## Target Filesystem Layout

```text
~/.local/bin/selfishell
~/.local/bin/sfs -> selfishell
~/.local/share/selfishell/releases/<version>/
~/.local/share/selfishell/current
~/.config/selfishell/
~/.local/state/selfishell/
~/.cache/selfishell/
```

The installed product must continue working after the source checkout or build
directory is moved or removed.

## Target CLI Contract

Keep command names and responsibilities narrow:

- `selfishell install`: install a selected profile and user configuration.
- `selfishell update`: update the CLI release, managed tools, and configuration;
  `--cli-only` and `--tools-only` restrict the scope.
- `selfishell doctor`: diagnose platform, dependencies, and configuration.
- `selfishell status`: report installed versions and managed files.
- `selfishell rollback`: switch to a retained Selfishell release.
- `selfishell uninstall`: remove managed files and optionally restore backups.
- `selfishell version`: print the CLI version.

`sfs <command>` must resolve to the same implementation and behavior as
`selfishell <command>`. Help and documentation should show `selfishell` as the
primary form.

Do not implement an implicit update during ordinary shell startup.

## Managed Resource State

Managed configuration is copied under `~/.config/selfishell`; user-facing paths
link to those copies, never to a source checkout. Each managed file and link has
an individual versioned state record under
`~/.local/state/selfishell/resources`. State is written through a temporary file
and atomic rename.

Preserve these invariants when extending the lifecycle:

- Write pending state before moving user data or creating a managed path.
- Retain the original backup path across idempotent reinstalls.
- Record and verify checksums for managed regular files.
- Treat a replaced link, changed file, or changed path type as user data.
- Preflight every uninstall resource before removing any of them.
- Never restore a backup over an occupied target.
- Dry-run must not create XDG directories, state files, backups, or links.
- The fixed-line state format is internal. Increment its version before changing
  field order or meaning.

## Engineering Rules

- Preserve macOS compatibility when writing bootstrap and CLI entrypoint code.
  macOS may provide Bash 3.2 unless the project explicitly installs another
  interpreter first.
- Keep platform-specific package operations in platform adapters. Do not scatter
  `brew` and `apt` branches throughout command implementations.
- Make setup idempotent. Running an operation twice must not destroy data or
  create duplicate configuration.
- Download into a temporary location, verify it, then move it atomically into
  place.
- Never execute an unversioned remote payload as the actual installer. A remote
  bootstrap may select a release, but release archives must be versioned and
  checksum-verified.
- Avoid `sudo` for Selfishell files. Request it only for system package actions
  that genuinely require it.
- Respect `HOME`, XDG variables, proxy variables, and non-interactive execution.
- Do not store company URLs, credentials, tokens, kubeconfigs, or user-specific
  secrets in the public repository.
- Keep company customization injectable through a local configuration file or a
  separate private repository.
- Do not claim support for a platform without automated or documented manual
  verification.

## Profiles and Local Extensions

Built-in profiles live in `profiles/*.conf` and contain declarative `include` and
`package` records. Keep profile files free of executable shell code. The profile
order is `minimal`, `developer`, `kubernetes`, then `full`; each larger profile
includes the preceding one. `minimal` is the default and includes the everyday
interactive shell tools; language runtimes and build dependencies begin in
`developer`.

Private package additions use `--local-profile FILE` or
`SELFISHELL_LOCAL_PROFILE`. Local files may contain only package records and may
not include another profile. Private shell customization belongs in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh`; it is sourced but never
tracked, replaced, or deleted by managed resource state.

Package adapters must inherit proxy environment variables. `--skip-packages` and
`SELFISHELL_OFFLINE=1` must perform configuration-only installation without any
package or network command.

## Release Contract

`install.sh` is the public, curl-delivered bootstrap. Keep it small and compatible
with macOS Bash 3.2. It selects an exact platform/architecture archive, verifies
that archive against `SHA256SUMS`, extracts into a versioned release directory,
then atomically switches `current` and CLI links.

Release assets and naming are defined in `docs/RELEASING.md`. A requested version
must use only its `releases/download/v<version>` path and must never fall back to
latest. The bootstrap installs the CLI only unless `--setup` is explicit.
Semantic version tags are the release version source. Do not replace assets on an
existing GitHub Release.

## Verification Expectations

Every shell change should receive the checks applicable to it:

```sh
bash -n bootstrap.sh legacy/common.sh legacy/macos.sh legacy/ubuntu.sh
zsh -n mac/.zshrc ubuntu/.zshrc common/*.zsh
```

As the test harness is introduced, also run ShellCheck, formatting checks, unit
tests with a temporary `HOME`, and idempotency tests. Never run installation
tests against the developer's real home directory.

Tests should cover at least:

- an empty home directory;
- existing configuration files and symbolic links;
- two consecutive installations;
- interrupted or incomplete downloads and clones;
- unsupported platforms and missing optional packages;
- uninstall, restore, CLI update, and rollback behavior.

## Known Risks in the Current Implementation

- Homebrew bootstrap still executes Homebrew's upstream installer when Homebrew
  is absent; company deployments should provision Homebrew separately when this
  trust model is not acceptable.
- Apt and Homebrew packages follow their package-manager repositories rather than
  the direct dependency manifest, so their exact transitive versions are not
  reproducible across repository snapshots.
- Release archives are checksum-verified but are not yet cryptographically
  signed. Do not describe checksums as publisher authentication.

Address these through the milestones instead of hiding them with documentation.

## Working Process

1. Read this file and `docs/MILESTONES.md`.
2. Check the worktree before editing and preserve unrelated user changes.
3. Select the earliest incomplete milestone whose prerequisites are complete.
4. Keep changes scoped to one reviewable milestone or a clearly identified slice.
5. Add or update tests with behavioral changes.
6. Update milestone checkboxes only after the acceptance criteria actually pass.
7. Record material architecture decisions in this file or a focused ADR under
   `docs/` so work can continue on another machine without conversation history.
