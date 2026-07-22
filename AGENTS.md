# Selfishell Agent Guide

This file contains repository-wide rules for coding agents. Keep it focused on
constraints that affect implementation. Use the linked documents for user and
maintainer procedures instead of duplicating them here.

## Project and Sources of Truth

Selfishell provides a consistent Zsh development environment for macOS, Ubuntu,
and Ubuntu on WSL. Installation must be simple, maintenance predictable, and the
user experience consistent across supported platforms.

- `VERSION` is the product version.
- `profiles/*.conf` defines built-in package profiles.
- `dependencies.conf` pins direct downloads and Git dependencies.
- `common/mise.toml` pins mise-managed developer tools.
- `docs/MILESTONES.md` is the roadmap, not a default task queue.
- `docs/RELEASING.md` is the release procedure.

The public installer currently lives in this GitHub repository. The
`selfishell.dev` domain is aspirational; do not add it to executable code until
it has a real distribution endpoint.

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

Managed configuration is copied under `~/.config/selfishell`; user-facing paths
link to those copies. `~/.zshrc` remains user-owned and contains one bounded
loader block that sources the managed platform entrypoint. Personal aliases,
exports, PATH entries, and functions belong outside that block.

Preserve these lifecycle invariants:

- write pending state before moving user data or creating a managed path;
- write state through a temporary file and atomic rename;
- retain the original backup path across idempotent reinstalls;
- checksum managed regular files;
- treat a replaced link, changed file, or changed path type as user data;
- preflight every uninstall or update resource before changing any of them;
- remove only an intact installer-managed loader or PATH block;
- make dry-run create no directories, state, backups, links, or files;
- increment the fixed-line state format version before changing field order or
  meaning.

The former managed `.zshrc` symlink and `local.zsh` extension are retired. Do
not migrate or delete them automatically. Detect legacy state and stop with
manual uninstall/reinstall instructions.

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
- Respect `HOME`, XDG variables, proxy variables, offline mode, and
  non-interactive execution.
- Never store credentials, internal URLs, kubeconfigs, or user-specific secrets
  in the public repository.
- Do not claim platform support without automated or documented verification.
- Ordinary shell startup must never install updates or block on the network. A
  cached release notice may refresh metadata in a non-blocking background job.

## Profiles and Dependencies

`minimal` is the default profile. `developer` includes `minimal` and adds the
larger interactive tools, jq, build tools, and language/editor tooling. Ghostty
is a separate saved macOS installation choice.

The developer profile uses a pinned mise binary for Neovim, Tree-sitter CLI,
Node.js, Python, and uv. Do not delete legacy NVM or pyenv data during migration.

Local additions use `--local-profile FILE` or `SELFISHELL_LOCAL_PROFILE`. Local
profiles may contain package records only and may not include another profile.
`--skip-packages` and `SELFISHELL_OFFLINE=1` must perform configuration-only
installation without package or network commands. `optional` packages are
attempted automatically but remain non-fatal.

Automated dependency discovery may open a review PR but must never auto-merge.
A maintainer merge of `automation/dependency-updates` may dispatch a stable
patch release only when `dependencies.conf` changes and the remaining diff is
limited to matching commit-pin replacements in `common/completion.zsh` and
`common/interactive.zsh`. Any other change must block automatic publication.

## Release Rules

`install.sh` selects an exact platform archive, verifies it against
`SHA256SUMS`, installs it into a versioned release directory, and atomically
switches links. An explicit version must use only its own
`releases/download/v<version>` path and never fall back to latest.

Semantic version tags are immutable release sources. Never replace assets on an
existing GitHub Release; publish a new patch version instead. Before tagging,
update `VERSION` and run:

```sh
bash scripts/release-check.sh <version>
```

After publication, verify all four archives, `SHA256SUMS`, `VERSION`, the exact
version URL, and `releases/latest/download/VERSION`. See
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
unsupported platforms, offline behavior, uninstall, restore, update, and
rollback.

## Repository Map

| Path | Responsibility |
| --- | --- |
| `bin/`, `lib/` | CLI commands, lifecycle, platform and package adapters |
| `common/`, `mac/`, `ubuntu/` | Managed shell, editor, and platform configuration |
| `profiles/`, `dependencies.conf` | Declarative profiles and approved dependencies |
| `tests/` | Isolated unit and lifecycle coverage |
| `scripts/` | Validation, benchmarks, dependency discovery, release builds |
| `.github/` | CI, dependency automation, and release publication |
| `docs/` | User, maintainer, security, and roadmap documentation |

## Working Process

1. Follow the user's requested scope. Consult the roadmap only for roadmap work
   or when no task has been selected.
2. Check the worktree before editing and preserve unrelated user changes.
3. Keep changes to one reviewable feature, fix, or documentation slice.
4. Add or update tests for behavioral changes.
5. Update milestone checkboxes only after their acceptance criteria pass.
6. Record durable architecture decisions in this file or a focused document
   under `docs/`; keep transient status and dated run logs out of agent rules.
