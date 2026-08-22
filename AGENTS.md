# Selfishell Agent Guide

This file contains repository-wide rules for coding agents. Keep it focused on
constraints that affect implementation. Use the linked documents for user and
maintainer procedures instead of duplicating them here.

## Project and Sources of Truth

Selfishell provides a consistent Zsh development environment for macOS, Ubuntu,
and Ubuntu on WSL. Installation must be simple, maintenance predictable, and the
user experience consistent across supported platforms.

- The immutable `v<version>` Git tag is the release version source of truth.
- `profiles/*.conf` defines built-in package profiles.
- `dependencies.conf` pins direct downloads and Git dependencies.
- `common/mise.toml` pins mise-managed developer tools.
- `docs/RELEASING.md` is the release procedure.

## Product Contract

The canonical command is `selfishell`; `sfs` is an optional convenience
symlink. Do not introduce `sf`, and use `selfishell` in documentation,
automation, and errors.

Supported commands are `help`, `version`, `doctor`, `install`, `status`,
`update`, `rollback`, and `uninstall`. Keep their responsibilities narrow:

- the bootstrap installs only the CLI unless `--setup` is explicit;
- `selfishell install` explicitly installs a profile and configuration;
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

Selfishell-declared dependency installation paths (e.g. a Zinit-managed Zsh
plugin checkout declared in `dependencies.conf`) are managed product state,
not user data, and may be replaced to restore their approved pinned version.

Managed defaults are copied under `~/.config/selfishell`. User-facing
integration uses either managed links or bounded blocks in user-owned
configuration files.

`~/.zshrc` and `~/.zprofile` remain user-owned, each holding one bounded
Selfishell block. `~/.zshrc`'s block sources the managed platform entrypoint;
personal aliases, exports, PATH entries, and functions belong outside it. On
Ubuntu/WSL, `~/.zshenv` is also user-owned and contains only Selfishell's
bounded zshenv block (`skip_global_compinit=1`); macOS `~/.zshenv` is not
managed.

Preserve these lifecycle invariants:

- write pending state before moving user data or creating a managed path;
- write state through a temporary file and atomic rename;
- retain the original backup path across idempotent reinstalls;
- checksum managed regular files;
- treat a replaced link, changed file, or changed path type as user data;
- preflight the full uninstall resource set before removing any resource;
- run required user-owned loader and block-target preflights before package or
  configuration changes during install and update;
- remove only an intact installer-managed loader;
- make dry-run create no directories, state, backups, links, or files;
- increment the fixed-line state format version before changing field order or
  meaning.

## Implementation Boundaries

- Keep `install.sh`, the CLI entrypoint, and shared libraries compatible with
  macOS Bash 3.2 unless the product explicitly installs another interpreter.
- Keep Homebrew and Apt operations in `lib/package_managers/`; do not scatter
  platform branches through command implementations.
- Keep profile files declarative: only supported `include` and `package`
  records, never executable shell code.
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

## Profiles and Dependencies

`developer` is the default profile and includes `minimal` plus the larger
interactive tools, jq, build tools, and language/editor tooling. `minimal` is
the explicit lightweight choice. Ghostty is a separate saved macOS installation
choice.

The developer profile's mise-managed tool membership is declared in
`profiles/developer.conf`; exact versions for those tools are pinned in
`common/mise.toml`, the source of truth for mise-managed tool versions.

When the tools/configuration phase runs, `--skip-packages` must skip package
and tool installation and apply managed configuration only. A default update
whose target release is already active exits before that phase; use
`update --tools-only --skip-packages` to reapply the current release's managed
configuration. Only that form is also network-free, since `update`'s
CLI-release phase can otherwise still reach the network. `optional` packages
are attempted automatically but remain non-fatal.

Automated dependency discovery may open a review PR but must never auto-merge
or auto-publish a release. A maintainer reviews and merges
`automation/dependency-updates`, then runs the normal manual release process
described below.

## Release Rules

`install.sh` selects an exact platform archive, verifies it against
`SHA256SUMS`, installs it into a versioned release directory, and atomically
switches links. An explicit version must use only its own
`releases/download/v<version>` path and never fall back to latest.

Semantic version tags are immutable release sources. The source checkout does
not track a release `VERSION` file: `scripts/build-release.sh` receives an
explicit version and generates `VERSION` only inside the release payload and
published asset set. A source checkout therefore reports `selfishell
development`, while an installed release reads its generated `VERSION` file.

Treat release readiness, audit, or preparation requests as non-publishing.
Only an explicit request to release, publish, or tag authorizes creating and
pushing a release tag. Before publishing, read `docs/RELEASING.md` and
`.github/workflows/release.yml` for the current procedure and automation
contract.

Publish from the documented release branch, currently `main`, only after its
changes are already pushed and verified. Create an annotated `v<version>` tag
on the intended commit and push that tag; the Release workflow validates the
tag, reruns the release checks on Linux and macOS, builds the archives, smoke
tests the exact install, generates attestations, and publishes the immutable
GitHub Release. Do not create a release-only version commit or maintain a
second version source.

Never replace assets on an existing GitHub Release; publish a new patch version
instead. After publication, verify all four archives, `SHA256SUMS`, generated
`VERSION`, the exact version URL, and `releases/latest/download/VERSION`. See
`docs/RELEASING.md` for the complete procedure.

## Verification

Run the smallest relevant tests while iterating, then run the repository gate
for any shell, lifecycle, profile, dependency, or release change:

```sh
bash scripts/check.sh
```

The gate performs Bash/Zsh syntax checks, ShellCheck, formatting checks, and the
test suite. Tests must use a temporary `HOME` and must never install against or
modify the developer's real home directory. Behavioral changes require tests,
especially for empty/existing paths, repeated operations, interruptions,
unsupported platforms, --skip-packages behavior, uninstall, restore, update,
and rollback.

Keep verification proportional to the change. For incidental presentation
changes such as spacing, prose, or glyph choices, do not add a regression test
unless the exact presentation is a product contract or a repeated source of
bugs; use the relevant parser, formatter, or focused smoke check instead. For a
small option change, prefer extending an existing focused test over creating a
new test or suite. Do not run the full repository gate for an unrelated
cosmetic or configuration-only change when a smaller relevant validation covers
it; the gate remains required for the change categories listed above.

## Repository Map

| Path | Responsibility |
| --- | --- |
| `bin/`, `lib/` | CLI commands, lifecycle, platform and package adapters |
| `common/`, `mac/`, `ubuntu/` | Managed shell, editor, and platform configuration |
| `profiles/`, `dependencies.conf` | Declarative profiles and approved dependencies |
| `tests/` | Isolated unit and lifecycle coverage |
| `scripts/` | Validation, benchmarks, dependency discovery, release builds |
| `.github/` | CI, dependency automation, and release publication |
| `docs/` | User, maintainer, and security documentation |

## Working Process

1. Follow the user's requested scope.
2. Check the worktree before editing and preserve unrelated user changes.
3. Keep changes to one reviewable feature, fix, or documentation slice.
4. Add or update tests for behavioral changes.
5. When changing a shared shell function's signature, search the entire
   repository for every call site, including scripts and tests, before
   finishing the change.
6. Record durable architecture decisions in this file or a focused document
   under `docs/`; keep transient status and dated run logs out of agent rules.
7. Keep planning artifacts out of the repository. Implementation plans, design
   drafts, task checklists, and roadmaps are working notes for one change: they
   go stale the moment the change merges, and the merged diff plus its tests are
   the durable record. Write them outside the checkout.
8. Report only checks actually run. Separate local results, GitHub Actions
   results, and checks that were unavailable; never imply an unrun check passed.
9. After merging a branch into `main`, delete the remote branch (e.g.
   `gh pr merge --delete-branch`); don't leave merged branches behind.
