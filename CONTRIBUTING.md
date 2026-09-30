# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes need Go-owned tests with private `HOME`, XDG, mise and temporary paths.
Go owns maintained setup, process execution, assertions and cleanup. Keep
native Zsh/Lua probes for their runtime APIs and small child-process fixtures
for external fault injection.

## Local development

The exact Go version is pinned in `go.mod` for development and CI. The installed
product does not need Go. The build uses `GOTOOLCHAIN=local` and fails if that
version is unavailable. Zsh, ShellCheck and shfmt are needed for the repository
gate. Neovim at the version pinned in `config/shared/mise.toml` is optional; it
enables the Neovim configuration tests, which skip without it.
The root `mise.toml` selects the same Go version for local development; keep it
in sync with `go.mod`. With mise installed, run `mise trust` and `mise install go`
from the checkout before building. Dependency automation updates both pins for
Go patch releases; see
[approved dependency updates](docs/RELEASING.md#approved-dependency-updates).

Source build scripts share `scripts/go-env.sh`: they require that exact Go
version on `PATH`, disable automatic toolchain downloads and ignore caller
Go settings, workspaces, build flags and cross-compilation targets. They build
in module mode without CGO, module downloads or VCS stamping and preserve
explicit compiler/module cache paths. The release builder selects its four
targets explicitly. `scripts/update-dependencies.sh` builds its Go maintenance
tool the same way.

```bash
bash scripts/build-cli.sh       # Build the host CLI at .build/selfishell
bin/selfishell help            # Source launcher executes that existing binary
.build/selfishell install --dry-run
bash scripts/build-cli.sh --all # Cross-build macOS/Linux × AMD64/ARM64
bash scripts/check.sh           # Syntax, format, vet, tests and four builds
```

The source launcher never compiles on invocation. The production artifact
builder is `bash scripts/build-release.sh --version VERSION --output DIR`; it writes
four native archives, `VERSION` and `SHA256SUMS` without modifying the source
checkout. The eight public commands run Go logic. The standalone Bash 3.2
bootstrap remains the installer transport, and maintenance scripts retain
narrow shell glue where needed.

Tests use the current checkout. Integration tests live in `tests/integration`
and cover updates, rollback, backup, restore and interrupted-operation recovery.
`SELFISHELL_TEST_CLI` may select an existing native executable for focused Go test
runs, including signal and benchmark tests. Fixtures that need their own release
root copy the executable so configuration discovery stays local to the fixture.

Keep command parsing and shared failure cases focused; do not repeat basic
help/version or successful reinstall checks in every lifecycle scenario. WSL
shares Ubuntu's configuration implementation, so its configuration matrix covers
install/restore with empty, existing and XDG homes. Release reproducibility checks
combine a relocated source tree, changed mtimes, hostile build variables and a
fresh cache; the shell builder is compared with those verified Go-built assets.

Test fixture builds use temporary Go caches by default. `SELFISHELL_TEST_GO_CACHE`
selects an absolute compiler-cache path instead: the repository gate uses
`.build/test-go-cache` in the checkout so repeated runs reuse fixture builds, and
CI passes its restored cache. The test process leaves that caller-owned cache in
place. Test homes and mise state remain private, and the reproducibility check
still uses a fresh cache.

Integration tests call `t.Parallel()` unless they must change the test process
environment with `t.Setenv`. Give each test its own temporary HOME and pass
child environments explicitly. Write fixture files with `testutil.WriteFile` or
`testutil.AppendFile`, which hold `syscall.ForkLock` so a parallel test's fork
cannot make an executable fixture fail with ETXTBSY; a source check enforces it.

`scripts/check-go.sh` runs Go format, vet, tests and target builds through the
repository gate. See [architecture](docs/ARCHITECTURE.md) for supported behavior
and [release procedure](docs/RELEASING.md) for manual publication.

## Verification coverage

CI runs on GitHub-hosted `ubuntu-latest` (AMD64) and `macos-latest` (ARM64)
runners. For runtime changes and release tags it runs the repository gate and
an exact prebuilt-archive install on both hosts, a full installation from the
Linux/AMD64 archive in an `ubuntu:24.04` container, the macOS/ARM64 archive's
configuration lifecycle, and the pinned Neovim developer lifecycle on Ubuntu.
The release builder produces four archive formats; only the host's native
archive executes in each job.

Linux/ARM64, macOS/AMD64, WSL 2, and OS releases other than those runner
images and the container are not executed in CI. Platform selectors in tests
simulate them; they are not evidence of runtime execution on another OS or CPU.
