# Selfishell Agent Guide

Repository-wide implementation constraints for coding agents. Keep user and
maintainer procedures in the linked documents rather than repeating them here.

## Project and Sources of Truth

Selfishell provides a consistent Zsh development environment for macOS, Ubuntu,
and Ubuntu on WSL. Keep installation simple and maintenance predictable.

- The immutable `v<version>` Git tag is the release version source of truth.
  `scripts/build-release.sh` generates `VERSION` only in release payloads;
  checkouts report `selfishell development`. Never track `VERSION`, add a
  release-only version commit, or introduce a second version source.
- `packages.conf` defines the environment's package membership.
- `dependencies.conf` pins direct downloads and Git dependencies, including
  Zsh plugin and Neovim plugin commits.
- `config/shared/mise.toml` pins mise-managed tool versions.
- `config/shared/nvim/lua/config/languages.lua` pins the default LSP servers as
  `server@version`; the update tools phase applies them through Mason, replacing
  a different installed version. User-added Mason packages are not managed.
- `go.mod` and the root `mise.toml` pin the development Go toolchain together.

Before changing an area, read its governing document and keep it in sync:

- Managed paths and integration: [Installation](docs/INSTALLATION.md#files-selfishell-manages).
- Update behavior and dependency synchronization: [Updates](docs/UPDATES.md).
- State format and recovery: [Architecture](docs/ARCHITECTURE.md#managed-state-format).
- Tests and verification: [Contributing](CONTRIBUTING.md#tests-and-repository-gate).
- Releases and dependency automation: [Releasing](docs/RELEASING.md).

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

Install Selfishell under these XDG-compatible user paths:

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
to restore their approved pins: Zsh plugin checkouts and other Git or direct
downloads recorded as Selfishell-installed. A usable unrecorded installation is
preserved as external. Neovim plugin checkouts move to approved commits only
while clean: a modified or non-Git checkout stops install and update until the
user removes it. Before removing unused plugin checkouts, stop synchronization
and preserve any directory that is non-Git, unreadable, locally modified, or
contains untracked or ignored files (excluding generated `doc/tags`). Only
clean unused Git checkouts may be removed.

Managed defaults are copied under `~/.config/selfishell`. User-facing
integration uses only:

- bounded blocks at the top of user-owned files: `~/.zshrc` (sources the
  platform entrypoint), `~/.zprofile` (mise shims), `~/.vimrc` (sources the Vim
  entrypoint), `~/.zshenv` (`skip_global_compinit=1` and the emacs keymap
  pin), and, when Ghostty is chosen on macOS,
  `~/.config/ghostty/config.ghostty` (`user.ghostty` is never touched);
- when chosen on WSL, one recorded Windows Terminal JSON fragment under
  `Fragments/Selfishell` that `updates` the current distribution profile's
  `font.face` and `colorScheme` (built-in `Dark+`, no custom scheme). Never
  write `settings.json`. Never create a profile or change its name, launch
  command, or the default profile. Identify existing profiles by distro
  identity, preserving renamed display names; skip missing or ambiguous targets.
  Move the fragment when the saved profile disappears; never touch a fragment
  recorded by another distribution whose state was copied into a clone.
  Font or color scheme values in the user's profile or Defaults take precedence;
  report them instead of editing them;
- managed links `~/.config/nvim`, `~/.config/starship.toml`, and
  `~/.config/mise/conf.d/selfishell.toml`, honoring `XDG_CONFIG_HOME`.

Personal aliases, exports, PATH entries, and functions belong outside blocks.
Keep the user-facing list in `docs/INSTALLATION.md` in sync.

Preserve these lifecycle invariants:

- write pending state before moving user data or creating a managed path;
- write state through a temporary file and atomic rename;
- make repeated setup safe and idempotent; retain the original backup path;
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
- retain direct-dependency ownership and font recovery records across purge,
  leaving installed tools and fonts in place;
- make dry-run create no directories, state, backups, links, or files, and never
  invoke mise: even its read-only commands write metadata;
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
- Download to a temporary location, verify it, and activate it atomically.
- Never execute an unversioned remote release payload as the installer.
- Never use `sudo` for Selfishell files; use it only for system package operations
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
The prompt and Neovim UI require a Nerd Font; do not add plain-Unicode
fallbacks. Ghostty is a separate saved macOS installation choice; an existing
app on PATH or in system/user Applications is preserved. Windows Terminal
integration is a separate saved WSL choice. Its optional fonts are `ubuntu-wsl`
direct package records with `font` download markers, installed and registered
per Windows user. Existing font families are preserved; fonts use versioned
paths so Windows-loaded files are never replaced during pin updates. Retain each
outgoing version's ownership evidence and reuse a retained file only when its
recorded path and checksum match the approved pin and file contents. Change only
an ownership-checked Selfishell font registration. Installer-owned fonts follow
approved pins and remain installed after configuration uninstall. The
`configured` marker in the state directory records completed setup, not a
package selection.

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
- Prune unused mise versions only after a successful update tools sync, using
  the platform's declared tools, never an empty tool list. Protect current pins via
  mise's tracked configuration, exclude the previous release's configuration
  even if tracked, and keep the CLI rollback release and project tracking.
  Skip during install, rollback, incomplete sync (including optional failures),
  `--skip-packages`, CLI-only or no-op updates, or when mise `ignored_config_paths`
  is configured. Cleanup and retention-preflight failures only warn; see
  [unused mise versions](docs/UPDATES.md#unused-mise-versions).

Dependency automation may open a review PR but must never auto-merge or
auto-publish a release; a maintainer merges `automation/dependency-updates` and
then runs the normal manual release. The build Go toolchain is excluded from
dependency automation. Maintainers update `go.mod` and the root `mise.toml`
together in a separate change when needed.
Vulnerability scans run in the dedicated scheduled workflow, outside the
ordinary local gate.

## Release Rules

- `install.sh` verifies the exact platform archive against `SHA256SUMS` before
  atomic installation and link activation in the versioned release layout above.
  Explicit versions use only `releases/download/v<version>`, never latest.
- Readiness, audit, or preparation requests are non-publishing. Only an
  explicit request to release, publish, or tag authorizes pushing a tag; first
  read `docs/RELEASING.md` and `.github/workflows/release.yml`.
- Tag only a pushed, verified `main` commit with an annotated `v<version>` tag.
- Never move a tag or replace release assets; publish a new patch version.
  Follow [release verification](docs/RELEASING.md#verify-the-published-release)
  after every publication.

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
- Product code reads the environment only through the injected
  `process.Environment` (a source check rejects `os.Getenv` and friends), so
  tests pass a private environment and call `t.Parallel()` instead of using
  `t.Setenv`. Tests write files only through `testutil.WriteFile` or
  `testutil.AppendFile` to prevent ETXTBSY from concurrent forks; see
  [test rules](CONTRIBUTING.md#tests-and-repository-gate).
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

`bin/`, `cmd/`, and `internal/` hold the launcher, CLI and lifecycle/platform
adapters; `config/` holds managed shell/editor defaults; `tests/` holds isolated
coverage and runtime fixtures; `scripts/` holds validation, benchmarks and
maintenance tools; `.github/` holds automation; `docs/` holds project guides.

`internal/` is layered and imports point only downward: the foundation
packages (`process`, `managed`, `report`) import no project package, the
subsystem packages (`terminal`, `release`, `neovim`, `lifecycle`) import only
the foundation, and `internal/selfishell` holds the CLI and imports the rest.
Put new code in the lowest layer whose dependencies it needs; a package that
would need the core belongs in the core or should receive an interface or a
function instead. See [Go packages](docs/ARCHITECTURE.md#go-packages).

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
