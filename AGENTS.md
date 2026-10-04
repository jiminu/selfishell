# Selfishell Agent Guide

This file contains repository-wide rules for coding agents: constraints that
affect implementation. User and maintainer procedures live in the linked
documents; link to them instead of repeating them here.

## Project and Sources of Truth

Selfishell provides a consistent Zsh development environment for macOS, Ubuntu,
and Ubuntu on WSL. Installation must be simple, maintenance predictable, and the
user experience consistent across supported platforms.

- The immutable `v<version>` Git tag is the release version source of truth.
- `packages.conf` defines the environment's package membership.
- `dependencies.conf` pins direct downloads and Git dependencies, including
  Zinit, Zsh plugin, and Neovim plugin commits. Zsh plugin commits are repeated
  in the `zinit ice ver'…'` lines of `config/shared/zsh/completion.zsh` and
  `interactive.zsh`; change both together, as dependency automation does.
- `config/shared/mise.toml` pins mise-managed tool versions.
- `config/shared/nvim/lua/config/languages.lua` pins the default LSP servers as
  `server@version`; the update tools phase applies them through Mason, replacing
  a different installed version. User-added Mason packages are not managed.
- `go.mod` and the root `mise.toml` pin the development Go toolchain together.
- `docs/UPDATES.md` defines user-facing update behavior; `docs/RELEASING.md`
  defines the release and dependency-update procedures.

## Product Contract

The canonical command is `selfishell`; `sfs` is an optional convenience
symlink. Do not introduce `sf`, and use `selfishell` in documentation,
automation, and errors.

Supported commands are `help`, `version`, `install`, `status`, `update`,
`rollback`, and `uninstall`. Keep their responsibilities narrow:

- the bootstrap installs only the CLI unless `--setup` is explicit;
- `status` is the single read-only diagnosis (system, configuration, tools,
  Zsh plugins); `doctor` stays a hidden alias, not a separate check;
- `selfishell install` explicitly installs the development environment and configuration;
- `update --cli-only` and `update --tools-only` keep release and environment
  updates separable;
- rollback uses a retained release without downloading it again;
- purge removes the CLI only when explicitly requested.

Install Selfishell without root privileges under XDG-compatible user paths:

```text
~/.local/bin/selfishell
~/.local/bin/sfs -> selfishell
~/.local/share/selfishell/releases/<version>/
~/.local/share/selfishell/current
~/.config/selfishell/
~/.local/state/selfishell/
~/.cache/selfishell/
```

The installed product must work after the source checkout is removed.

## User Data and Managed State

Treat every existing path as user data. Back it up safely, never overwrite a
backup, and never restore over an occupied target.

Selfishell-owned dependency paths are managed product state and may be replaced
to restore their approved pins: Zsh plugin checkouts, and Zinit or direct
downloads recorded as Selfishell-installed. A usable unrecorded installation is
preserved as external. Neovim plugin checkouts move to approved commits only
while clean: a modified or non-Git checkout stops install and update until the
user removes it.

Managed defaults are copied under `~/.config/selfishell`. User-facing
integration uses only:

- bounded blocks at the top of user-owned files: `~/.zshrc` (sources the
  platform entrypoint), `~/.zprofile` (mise shims), `~/.vimrc` (sources the Vim
  entrypoint), Ubuntu/WSL `~/.zshenv` (`skip_global_compinit=1`; macOS
  `~/.zshenv` is not managed), and, when Ghostty is chosen on macOS,
  `~/.config/ghostty/config.ghostty` (`user.ghostty` is never touched);
- when chosen on WSL, only the current distribution profile's `font.face` and
  `colorScheme` in Windows Terminal `settings.json`, backed up and journaled
  separately, plus a checksummed color-scheme fragment under
  `%LOCALAPPDATA%/Microsoft/Windows Terminal/Fragments/Selfishell`. Never create
  a profile or change its name, launch command, or the default profile. Identify
  existing profiles by distro identity, preserving renamed display names; skip
  missing or ambiguous targets. Preserve unrelated JSON, comments, and user edits.
  Uninstall restores only the two values that still match the applied values;
- managed links `~/.config/nvim`, `~/.config/starship.toml`, and
  `~/.config/mise/conf.d/selfishell.toml`, honoring `XDG_CONFIG_HOME`.

Personal aliases, exports, PATH entries, and functions belong outside blocks.
Keep the user-facing list in `docs/INSTALLATION.md` in sync.

Preserve these lifecycle invariants:

- write pending state before moving user data or creating a managed path;
- write state through a temporary file and atomic rename;
- retain the original backup path across idempotent reinstalls;
- checksum managed regular files;
- synchronize update tools and configuration without a blanket confirmation;
  ask only before overwriting locally modified managed files or blocks, retaining
  conflict backups and the `--yes`/non-interactive preservation behavior;
- repair wholly absent blocks without replacing surrounding user content;
  retain malformed-marker and changed-path-type protections;
- treat a replaced link, changed file, or changed path type as user data;
- preflight the full uninstall resource set before removing any resource;
- run required user-owned loader and block-target preflights before package or
  configuration changes during install and update;
- remove only an intact installer-managed loader;
- make dry-run create no directories, state, backups, links, or files;
- increment the fixed-line state format version before changing field order or
  meaning.

## Implementation Boundaries

- Keep `install.sh`, the source CLI launcher, and maintenance shell scripts
  compatible with macOS Bash 3.2. The installed CLI is a prebuilt native Go
  executable; source development requires the pinned Go toolchain explicitly.
- Keep Homebrew and Apt operations in the Go platform adapters; do not scatter
  platform branches through command implementations.
- Keep `packages.conf` declarative: only supported `package` records, never
  executable shell code.
- Make repeated setup safe and idempotent.
- Download to a temporary location, verify it, and activate it atomically.
- Never execute an unversioned remote release payload as the installer.
- Avoid `sudo` for Selfishell files; use it only for system package operations
  that require it.
- Respect `HOME`, XDG variables, proxy variables, and non-interactive
  execution.
- Never store credentials, internal URLs, kubeconfigs, or user-specific secrets
  in the public repository.
- Do not claim platform support without automated or documented verification.
- Ordinary shell startup must never install updates or block on the network. A
  cached release notice may refresh metadata in a non-blocking background job.

## Packages and Dependencies

Selfishell provides one development environment, without selectable profiles.
Ghostty is a separate saved macOS installation choice; an existing app on PATH
or in system/user Applications is preserved. Windows Terminal integration is a
separate saved WSL choice. Its optional fonts are `ubuntu-wsl` direct package
records with `font` download markers, installed and registered per Windows user.
Existing font families are preserved; fonts use versioned paths so Windows-loaded
files are never replaced during pin updates. Change only an ownership-checked
Selfishell font registration. Existing distro profiles use Dark+, matching
Ghostty; their launch commands remain user-owned. Installer-owned fonts follow approved pins
and remain installed after configuration uninstall. The `configured` marker
in the state directory records completed setup, not a package selection.
Behavior below is described for users in `docs/UPDATES.md`.

- Installer-owned mise operations run from the release's `config/shared`
  directory so a caller's project cannot override approved tool versions.
- Status queries installed versions, not configured requests; an
  orphaned mise shim is not an external installation. After completed setup,
  status reports missing records for the current platform's required resources
  (respecting the Ghostty choice) and still inspects every tracked record,
  including other platforms'. Missing optional tools stay informational in
  detail and summary.
- `optional` packages are attempted automatically but remain non-fatal.
- `--skip-packages` skips package and tool installation and applies managed
  configuration only. An already-current default update exits before the tools
  phase; only `update --tools-only --skip-packages` reapplies it network-free.
- After a successful tools sync, prune unused versions of the platform's
  declared mise tools, never with an empty tool list. Protect current pins via
  mise's tracked configuration, exclude the previous release's configuration
  even if tracked, and keep the CLI rollback release and project tracking.
  Skip on incomplete setup (including optional failures), `--skip-packages`,
  and CLI-only or no-op updates; cleanup failures only warn.
- Dry-run must not invoke mise; even its read-only commands write metadata.

Dependency automation may open a review PR but must never auto-merge or
auto-publish a release; a maintainer merges `automation/dependency-updates` and
then runs the normal manual release. Go patch discovery stays within the
current release line; selecting a new line is a maintainer choice.
Vulnerability scans run in the dedicated scheduled workflow, outside the
ordinary local gate.

## Release Rules

- `install.sh` verifies an exact platform archive against `SHA256SUMS`,
  installs it into a versioned release directory, and switches links
  atomically. An explicit version uses only its own
  `releases/download/v<version>` path and never falls back to latest.
- No `VERSION` file is tracked: `scripts/build-release.sh` generates it inside
  the release payload, so a checkout reports `selfishell development`. Never
  add a release-only version commit or a second version source.
- Readiness, audit, or preparation requests are non-publishing. Only an
  explicit request to release, publish, or tag authorizes pushing a tag; first
  read `docs/RELEASING.md` and `.github/workflows/release.yml`.
- Tag a pushed, verified `main` commit with an annotated `v<version>` tag; the
  Release workflow reruns full CI for it before building and publishing.
- Tags and releases are immutable, enforced by a `v*` tag ruleset and GitHub
  immutable releases. Never move a tag or replace assets; publish a new patch
  version, and verify each publication as `docs/RELEASING.md` describes.

## Verification

Run the smallest relevant tests while iterating, then the repository gate
(`bash scripts/check.sh`; contents in `CONTRIBUTING.md`) for any shell,
lifecycle, package, dependency, or release change.

- Maintained test setup, process control, assertions, and cleanup belong in Go;
  native Zsh/Lua runtime probes and small external-process fixtures may remain.
- Tests must use a temporary `HOME` and never install against or modify the
  developer's real home directory. Clear inherited `WSL_DISTRO_NAME` in Linux
  home fixtures; Windows integration fixtures must explicitly provide a fake
  Windows environment or a private native scratch directory.
- Integration tests call `t.Parallel()` unless they need `t.Setenv`. They write
  files only through `testutil.WriteFile` or `testutil.AppendFile`: a concurrent
  fork would inherit a plain write descriptor and make exec fail with ETXTBSY.
- Behavioral changes require tests, especially for empty/existing paths,
  repeated operations, interruptions, unsupported platforms, `--skip-packages`,
  uninstall, restore, update, and rollback.
- Keep verification proportional: presentation-only changes (spacing, prose,
  glyphs) need no regression test unless the exact output is a product contract
  or a repeated bug source; use a parser, formatter, or focused smoke check
  instead. A small option change extends an existing focused test. Outside the
  gate categories above, a cosmetic or configuration-only change may skip the
  gate when a smaller check covers it.

## Repository Map

| Path | Responsibility |
| --- | --- |
| `bin/`, `cmd/`, `internal/` | Explicit source launcher, native CLI, lifecycle, platform and package adapters |
| `config/` | Managed shared, macOS, and Ubuntu shell/editor configuration |
| `packages.conf`, `dependencies.conf` | Declarative packages and approved dependencies |
| `tests/` | Go-owned isolated unit and lifecycle coverage; native runtime and external-process fixtures |
| `scripts/` | Validation, benchmarks, dependency discovery, release builds |
| `.github/` | CI, dependency automation, and release publication |
| `docs/` | User, maintainer, and security documentation |

## Working Process

1. Follow the requested scope; keep each change to one reviewable feature, fix,
   or documentation slice, and preserve unrelated worktree changes.
2. When changing a shared function's signature, update every call site,
   including shell scripts and tests.
3. Record durable decisions here or in a focused `docs/` file. Keep planning
   artifacts, transient status, and dated run logs out of the repository.
4. Report only checks actually run, separating local results, GitHub Actions
   results, and unavailable checks; never imply an unrun check passed.
5. After merging into `main`, delete the merged branch (e.g.
   `gh pr merge --delete-branch`).
