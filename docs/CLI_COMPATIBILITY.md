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

The baseline proves the comparator and fixtures. It does not by itself prove
Go compatibility. Candidate phases must compare the candidate against these
references, and the existing integration suites still cover Bash-specific
failure injection until equivalent candidate coverage exists.

## Verification reporting

Report local checks, GitHub Actions checks and unavailable environments
separately. Archive-format inspection or cross-compilation is not execution on
that OS/CPU. Release tests select the real host archive; simulated platform
detection remains a separate behavior test. Preserve this distinction in
performance comparisons as well: measure matched fixtures on the same host,
and do not present Linux timings as macOS results.
