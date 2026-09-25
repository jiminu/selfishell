# CLI compatibility

The Go CLI preserves the existing command, user-data and release contracts in
[AGENTS.md](../AGENTS.md). A language change does not change package membership,
managed state meaning, or the boundaries between CLI updates and environment
updates. Bootstrap remains a shell script, and Zsh/editor configuration remains
in its native form. Users receive a compiled executable and do not install Go.

## Toolchain and platform boundary

The selected development toolchain is Go **1.27.1**. Native release candidates
use `CGO_ENABLED=0`, with baseline CPU targets (`GOAMD64=v1`, `GOARM64=v8.0`),
for macOS/Linux on AMD64/ARM64. The toolchain version belongs to development and
CI configuration; it is not an environment package in `packages.conf`.

The Go CLI requires **macOS 13 or newer**. Older macOS versions are outside the
support scope; no compatibility implementation or intermediate migration
release is planned for them. The Linux validation baseline is **Ubuntu 24.04
LTS**, with **WSL 2 running Ubuntu** as the Windows environment. Ubuntu 26.04
LTS, actual WSL execution and additional CPU combinations need their own
installation checks before being reported as verified. WSL 1, native Windows
and other Linux distributions are outside the Go migration's support scope.
These are migration targets, not evidence that the Go candidate has passed
platform verification. See [installation verification coverage](INSTALLATION.md#verification-coverage)
for the production environment's existing coverage.

Manifest validation precedes configuration application, including when
`--skip-packages` is used. Runtime consumers such as benchmarks and E2E scripts
must exercise the candidate as each capability becomes available. Even an early
candidate must be built for all four targets and smoke-tested with `help` and
`version` on its native hosts; complete upgrade/rollback validation follows
when that behavior exists. Production activation waits for compatibility and
release checks, and publication remains a separate maintainer decision.

## Fixed references

The behavior reference is commit
`3bbbfa0346ee74eb47f31a81ec666340a5ef6018`. The legacy release is **v1.3.1** at
`d025710338036f1f54b948f1f3e5c17a0b3f7e38`. Tests export these exact commits
from local Git history. Missing history fails explicitly; tests never fetch
or substitute the current checkout for a legacy release.

The legacy archives are built twice with the legacy commit's own builder.
Tests compare all four archives, `SHA256SUMS` and generated `VERSION`, then
check each checksum and install the exact host archive through the legacy
bootstrap using `file://` URLs. The source export is removed before exercising
the installed CLI, configuration lifecycle and purge.

## Comparison method

`bash tests/go_migration_test.bash --phase baseline` runs the fixed reference
twice, using empty and existing user-data fixtures. The scenario captures
stdout, stderr, exit status and the complete HOME tree after help/version,
invalid arguments, configuration dry-run, install, reinstall, configuration
update and restore. Filesystem captures retain exact bytes, permission bits,
path types and symlink targets, including managed state and backups. Directory
links are not followed and special files are not opened.

Each repetition uses the same HOME and release paths. A fixture fixes only the
backup-name clock so timestamp differences cannot obscure backup identity or
collision suffixes. No captured content, path or checksum is rewritten. The
snapshot helper disables Python bytecode writes so observation does not modify
the fixture. A restricted command PATH and an empty inherited environment keep
caller configuration, package managers and network tools out of the reference
scenario.

The baseline proves the comparator and fixtures. The separate
`bash tests/go_migration_test.bash --phase config` runs the actual native Go
binary and fixed Bash CLI from the same temporary release path. It compares
command output, exit status, and complete HOME bytes, permissions, links, state,
and backups for empty, existing, custom XDG, changed-resource, pending-recovery,
late-preflight, and malformed-package cases on simulated macOS, Ubuntu, and
Ubuntu/WSL. A versioned temporary prefix also exercises dry-run and real purge
after removing the source export. Expected failure statuses and dry-run
invariance are asserted independently of the Bash/Go comparison. The candidate
checks malformed dependency records before mutation; this is an intentional
parser boundary beyond the reference's lazy dependency selection.

`scripts/check-go.sh` runs the config phase on each native CI host. Simulated
platform selection checks resource choice and lifecycle logic; it does not
constitute runtime verification on another OS or CPU. Bash-specific failure
injection remains in its existing integration suites.

## Verification reporting

Report local checks, GitHub Actions checks and unavailable environments
separately. Archive-format inspection or cross-compilation is not execution on
that OS/CPU. Release tests select the real host archive; simulated platform
detection remains a separate behavior test. Preserve this distinction in
performance comparisons as well: measure matched fixtures on the same host,
and do not present Linux timings as macOS results.

## Development execution foundation

`go.mod` pins the development and CI toolchain. `scripts/build-cli.sh` builds
`.build/selfishell` with CGO disabled and baseline CPU settings; `--all` also
builds the four targets under `.build/targets`. `scripts/check-go.sh` runs Go
formatting, vet, tests, builds and native reference comparisons as part of the
repository gate. CI uses the same pin on Linux and macOS. Production entrypoints
and release payloads remain Bash until the migration cutover.

The candidate implements help, local version, configuration-only
`install --skip-packages`, and `uninstall` (including `--restore` and explicit
`--purge`). Install without `--skip-packages` fails before mutation because
package and tool installation remains in Bash. Update, rollback, doctor,
status, and `version --available` remain unavailable in the Go candidate and
return an explicit error. The production CLI and installer remain Bash.
Configuration dry-run makes no filesystem changes, and user-owned targets are
preflighted before install or uninstall changes begin.
The Go candidate deliberately corrects one inherited Bash behavior: when an
existing user file already has the same bytes as a managed default, install
still saves the original before adopting that path. This preserves its original
permissions and lets `uninstall --restore` return it. The immutable Bash
reference skips that backup and can delete the preexisting file on uninstall.
The candidate-only configuration lifecycle test covers this safety correction;
the fixed-reference comparisons and their existing snapshots remain intact.
Release location follows the resolved executable (including chained symlinks),
not the caller's working directory or `SELFISHELL_ROOT`. Generated VERSION files
remain the installed version source; `.git` marks source development builds.

The execution helper passes argument arrays, environment and standard streams
directly to `os/exec`. It retains foreground terminal membership, preserves child
exit/signal statuses, and cancels and reaps its direct child when its context is
cancelled; it does not promise process-tree cancellation. Downloads keep curl's
proxy and `file://` behavior, connection/stall policy and metadata timeout.
Verification and atomic activation of downloaded files belong to their lifecycle
consumers. No shell command is constructed from an argument string.

## Managed-state interoperability

The Go state primitives keep v2's seven newline-terminated fields in order:
version, kind, status, target, reference, backup, checksum. Reads preserve field
bytes and empty optional fields. As with the retained Bash reader, trailing lines
after the seventh field are ignored; a write emits exactly seven lines. Invalid
version/kind/status, an empty target, a missing field terminator, or NUL bytes
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

Resource declarations retain their Bash order. Installation selection chooses
the platform Zsh entrypoint, the saved macOS Ghostty choice, and Ubuntu/WSL's
zshenv block. Uninstall must use the complete declaration set, including resources
left from another platform. State collection returns no partial list on an error;
lifecycle consumers must still validate every managed target before removing any.
The primitives perform no target removal, backup replacement, or restoration.

Interoperability tests use the fixed Bash reference and v1.3.1's own native
release payload after removing its source export. Both generate pending file,
link and block records, consume Go-written records, finish interrupted setup,
retain original backups through reinstalls, and restore user bytes. These tests
prove the state boundary used by the configuration lifecycle comparison above.
