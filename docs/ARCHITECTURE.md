# Architecture

The installed `selfishell` command is a native Go executable with all seven
public commands: `help`, `version`, `install`, `status`, `update`, `rollback`,
and `uninstall`; `doctor` is a hidden alias of `status`. `sfs` is an optional
link. The standalone Bash 3.2 bootstrap transports a verified archive; native Zsh and editor configuration
continue to run in their own languages. Users do not need Go. Package
membership and pins live in
`packages.conf`, `dependencies.conf`, and `config/shared/mise.toml`.

## Development and platform boundary

`go.mod` pins the exact Go version for source builds and CI. Build explicitly with
`bash scripts/build-cli.sh`; `bin/selfishell` then executes the existing
`.build/selfishell` and gives a build instruction if it is absent. Ordinary
invocation never compiles. `scripts/build-release.sh --version VERSION` is the
single production builder and writes native archives without a source `VERSION`
file. Release executables use `CGO_ENABLED=0`, `GOAMD64=v1` and `GOARM64=v8.0`.
Developer build entrypoints use `scripts/go-env.sh` to check the pin before
compilation and isolate Go settings from the caller. Full benchmark provisioning
applies the same policy inside its private child process.

Dependency discovery and pin rewriting live in the `selfishell-dev` Go command;
`scripts/update-dependencies.sh` only builds and invokes it. See
[approved dependency updates](RELEASING.md#approved-dependency-updates).

The target floor is macOS 13, Ubuntu 24.04 LTS, and Ubuntu on WSL 2, for AMD64
and ARM64 archive formats. See [verification coverage](../CONTRIBUTING.md#verification-coverage)
for environments exercised in CI and the limits of simulated platform tests.

## Tests

Tests exercise the current CLI directly, including exit statuses, diagnostic
output, personal-file bytes and modes, managed links, state and backups.
Integration tests in `tests/integration` use the current checkout.

All maintained test orchestration, setup, assertions and cleanup use Go.
Native Zsh probes cover shell startup, completion, widgets and notices; Lua
probes cover Neovim APIs. Small child-process fixtures inject external failures.
Shell scripts under `scripts/` are bootstrap, narrow maintenance/CI glue,
explicit builders or benchmark launchers. `scripts/check.sh` is the repository
gate: shell checks, Go format/vet/tests, four builds and native host checks.
Release publication is a separate manual tag decision.

## Product behavior

Managed paths remain user-safe: pending state is written before mutation,
regular files are checksummed, original backups persist across reinstalls,
preflights cover all uninstall resources, and restore never overwrites an
occupied target. Dry-run writes nothing. Rollback uses retained files offline.
See [update modes](UPDATES.md#update-modes) for `--skip-packages` and no-op
updates.

A full update runs the newly activated CLI for its tools/configuration phase.
Cancellation sends that CLI SIGTERM and gives it up to five seconds to cancel
and reap its direct child. Ordinary child commands receive SIGTERM with a
one-second grace period before forced termination. Children keep the foreground
process group for terminal input.

An existing user file with the same bytes as a managed default is backed up
before adoption and preserved on restore.
Three operational limits remain intentional. Optional package failures warn
without invalidating a setup that otherwise completes; a fresh download may
activate before a later state commit failure is reported; and concurrent
installers have no lock guarantee. Run one bootstrap/update/rollback at a time.
A failure after activation requires checking `selfishell version` before retry.

## Managed-state format

State formats v2 and v3 have seven newline-terminated fields in order:
version, kind, status, target, reference, backup, checksum. Reads preserve field
bytes and empty optional fields. Trailing lines after the seventh field are
ignored; a write emits exactly seven lines. Invalid version/kind/status, an
empty target, a missing field terminator, or NUL bytes
are rejected. Writers reject embedded line breaks and NUL rather than producing
a record whose fields cannot be represented faithfully.

New transient records use v3. A pending file update retains the accepted
pre-write checksum, so failure before replacement can be retried; the source
checksum also recognizes a replacement completed before the active state was
saved. Changes made by the user after the failed write still require conflict
handling. Initial installation retains its backup-before-adoption behavior.

The v3 `restoring` status is written after removing the managed file or link,
before exclusively moving its original backup home. If the backup still exists,
retry requires an empty target. If only the target exists, retry removes the
record without touching that now user-owned path. If both paths are absent,
recovery stops; if both are occupied, neither is overwritten. Finish an
interrupted restore with `selfishell uninstall --restore` before reinstalling.

Completed `active` records retain v2 representation for compatibility with older
CLI rollback releases. Readers accept existing v2 records. An older CLI rejects
v3 transient records safely; complete the interrupted operation with the newer
CLI before rolling back.

Managed block checksums treat a closing marker at EOF without a final newline
as the same line with a newline. Removal still uses the actual byte boundaries
and preserves surrounding user content; marker and block-body edits remain
subject to the normal conflict checks.

A missing record is distinct from a malformed record and from an I/O failure.
State symlinks, directories and special files are rejected. Writes validate
before creating directories, create a private temporary file beside the state,
finish writing, syncing and closing it, then rename it atomically. A failed
pre-commit step preserves the previous record and removes its temporary file;
an interrupted process may leave a temporary file, which subsequent reads ignore.
Identical writes retain the existing state inode and timestamps. This does not
introduce a concurrent-installer or power-loss durability guarantee.

Checksums use POSIX `cksum`'s `CRC:SIZE` representation, without text
normalization. Go computes the same checksum in process using the standard
library's IEEE CRC, preserving existing state values without starting an
external command for each file or block. Reads use bounded buffers, check
cancellation between reads, and propagate read errors. The regular-file checks
and no-follow open prevent treating a managed-path symlink as an unchanged file.

Resource declarations have a fixed order. Installation selection chooses
the platform Zsh entrypoint, the saved macOS Ghostty choice, and Ubuntu/WSL's
zshenv block. Uninstall must use the complete declaration set, including resources
left from another platform. State collection returns no partial list on an error;
lifecycle consumers must still validate every managed target before removing any.
The primitives perform no target removal, backup replacement, or restoration.

Go tests cover pending file, link and block records, interrupted-operation
recovery, original-backup retention across reinstalls, and restoration of user
bytes. Changes to field order or meaning require a new format version.
