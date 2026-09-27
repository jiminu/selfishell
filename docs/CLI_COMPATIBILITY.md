# CLI compatibility

The installed `selfishell` command is a native Go executable with all eight
public commands: `help`, `version`, `doctor`, `install`, `status`, `update`,
`rollback`, and `uninstall`. `sfs` is an optional link. The standalone Bash 3.2
bootstrap transports a verified archive; native Zsh and editor configuration
continue to run in their own languages. There is no selectable Bash/Go mode
and users do not need Go. Package membership and pins remain in
`packages.conf`, `dependencies.conf`, and `config/shared/mise.toml`.

## Development and platform boundary

`go.mod` pins Go 1.27.1 for source builds and CI. Build explicitly with
`bash scripts/build-cli.sh`; `bin/selfishell` then executes the existing
`.build/selfishell` and gives a build instruction if it is absent. Ordinary
invocation never compiles. `scripts/build-release.sh --version VERSION` is the
single production builder and writes native archives without a source `VERSION`
file. Release executables use `CGO_ENABLED=0`, `GOAMD64=v1` and `GOARM64=v8.0`.

The target floor is macOS 13, Ubuntu 24.04 LTS, and Ubuntu on WSL 2, for AMD64
and ARM64 archive formats. Completed native CI execution has covered a Linux
AMD64 Ubuntu 24.04 container and macOS ARM64 26.6.2. Four formats are built and
inspected; format inspection and simulated platform selectors do not prove
runtime behavior on all four targets. Actual WSL, Ubuntu 26.04, macOS 13,
and the other CPU/OS pairings remain unexecuted in this migration evidence.
The new ordinary CI exact-prebuilt smoke step and Release workflow's artifact
transfer need completed runs before their results can be claimed.

## Transition and test boundary

The Bash-to-Go transition uses a fresh installation. In-place upgrades from
Bash releases and rollback across the Bash/Go boundary are not supported.
See [reinstalling from a Bash release](INSTALLATION.md#reinstalling-from-a-bash-release).
Go-to-Go updates and offline rollback remain supported.

Tests exercise the current Go CLI directly, including exit statuses, diagnostic
output, personal-file bytes and modes, managed links, state and backups. They
use the current checkout without exporting historical commits. No old Bash
engine, state bridge or full-history test requirement remains.

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
occupied target. Dry-run writes nothing. `update --tools-only --skip-packages`
reapplies current configuration without network access; ordinary default
update is a no-op when already current. Rollback uses retained files offline.

An existing user file with the same bytes as a managed default is backed up
before adoption and preserved on restore.
Three operational limits remain intentional. Optional Apt failures warn
without invalidating a setup that otherwise completes; a fresh download may
activate before a later state commit failure is reported; and concurrent
installers have no lock guarantee. Run one bootstrap/update/rollback at a time.
A failure after activation requires checking `selfishell version` before retry.

## Managed-state format

State format v2 has seven newline-terminated fields in order:
version, kind, status, target, reference, backup, checksum. Reads preserve field
bytes and empty optional fields. Trailing lines after the seventh field are
ignored; a write emits exactly seven lines. Invalid version/kind/status, an
empty target, a missing field terminator, or NUL bytes
are rejected. Writers reject embedded line breaks and NUL rather than producing
a record whose fields cannot be represented faithfully.

A missing record is distinct from a malformed record and from an I/O failure.
State symlinks, directories and special files are rejected. Writes validate
before creating directories, create a private temporary file beside the state,
finish writing, syncing and closing it, then rename it atomically. A failed
pre-commit step preserves the previous record and removes its temporary file;
an interrupted process may leave a temporary file, which subsequent reads ignore.
Identical writes retain the existing state inode and timestamps. This does not
introduce a concurrent-installer or power-loss durability guarantee.

Checksums retain POSIX `cksum`'s `CRC:SIZE` representation, without text
normalization. The Go helper uses the existing `cksum` executable on a regular
file and propagates input, process and output errors. It does not follow a
managed-path symlink as though it were an unchanged file.

Resource declarations have a fixed order. Installation selection chooses
the platform Zsh entrypoint, the saved macOS Ghostty choice, and Ubuntu/WSL's
zshenv block. Uninstall must use the complete declaration set, including resources
left from another platform. State collection returns no partial list on an error;
lifecycle consumers must still validate every managed target before removing any.
The primitives perform no target removal, backup replacement, or restoration.

Go tests cover pending file, link and block records, interrupted-operation
recovery, original-backup retention across reinstalls, and restoration of user
bytes. State format v2 remains unchanged; changes to field order or meaning
still require a new format version.
