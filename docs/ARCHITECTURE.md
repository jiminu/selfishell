# Architecture

The installed `selfishell` command is a native Go executable with all seven
public commands: `help`, `version`, `install`, `status`, `update`, `rollback`,
and `uninstall`; `doctor` is a hidden alias of `status`. `sfs` is an optional
link. The standalone Bash 3.2 bootstrap transports a verified archive; native
Zsh and editor configuration continue to run in their own languages. Users do
not need Go. Package membership and pins live in
`packages.conf`, `dependencies.conf`, and `config/shared/mise.toml`.

## Development and platform boundary

The source launcher `bin/selfishell` executes an existing `.build/selfishell`
and gives a build instruction if it is absent; ordinary invocation never compiles.
See [local development](../CONTRIBUTING.md#local-development) for prerequisites
and build commands.

Source build scripts share `scripts/go-env.sh`: they require the exact Go
version pinned in `go.mod` on `PATH`, disable automatic toolchain downloads and
ignore caller Go settings, workspaces, build flags and cross-compilation targets.
They build in module mode without CGO, module downloads or VCS stamping and
preserve explicit compiler/module cache paths. Full benchmark provisioning
applies the same policy inside its private child process.

`scripts/build-release.sh` is the single production builder. It selects its
four targets explicitly, using `CGO_ENABLED=0`, `GOAMD64=v1` and `GOARM64=v8.0`,
and generates release-only version metadata without modifying the checkout.
See [release artifacts](RELEASING.md#optional-local-artifact-check) for the
output contract. Other shell scripts provide narrow maintenance/CI glue or
benchmark launchers.

Dependency discovery and pin rewriting live in the `selfishell-dev` Go command;
`scripts/update-dependencies.sh` only builds it with the same Go policy and
invokes it. See
[approved dependency updates](RELEASING.md#approved-dependency-updates).

See [supported platforms](INSTALLATION.md) and
[verification coverage](../CONTRIBUTING.md#verification-coverage) for the target
baseline, environments exercised in CI and limits of simulated platform tests.

## Tests

See [tests and repository gate](../CONTRIBUTING.md#tests-and-repository-gate)
for test ownership, isolated fixtures and local verification, and
[verification coverage](../CONTRIBUTING.md#verification-coverage) for CI and
native WSL checks.

## Product behavior

Install and tools updates share configuration preparation, including Ghostty
selection on macOS and Windows Terminal selection and fragment preflight on WSL.
Preparation does not write configuration or saved choices and finishes before
package operations begin. Installation and diagnostics also share package
selection: Windows Terminal opt-in gates only WSL direct downloads marked
`font`; other WSL packages are selected independently.

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
Optional package failures do not invalidate an otherwise completed setup.
Release activation and state commit are separate operations, and concurrent
installers have no lock guarantee. See
[CLI releases and rollback](UPDATES.md#cli-releases-and-rollback) for failure
recovery and concurrency limits.

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
the platform Zsh entrypoint and the saved macOS Ghostty choice. Uninstall must use the complete declaration set, including resources
left from another platform. State collection returns no partial list on an error;
lifecycle consumers must still validate every managed target before removing any.
The primitives perform no target removal, backup replacement, or restoration.

Go tests cover pending file, link and block records, interrupted-operation
recovery, original-backup retention across reinstalls, and restoration of user
bytes. Changes to field order or meaning require a new format version.
