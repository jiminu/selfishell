# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes need Go-owned tests with private `HOME`, XDG, mise and temporary paths.

## Local development

The exact Go version is pinned in `go.mod` for development and CI. The installed
product does not need Go. The build uses `GOTOOLCHAIN=local` and fails if that
version is unavailable. Zsh, ShellCheck and shfmt are needed for the repository
gate. Neovim at the version pinned in `config/shared/mise.toml` is optional; it
enables the Neovim configuration tests, which skip without it.
The root `mise.toml` selects the same Go version for local development; keep it
in sync with `go.mod`. With mise installed, run `mise trust` and `mise install go`
from the checkout before building. Dependency automation leaves both pins
unchanged. Update them together in a separate reviewed change when needed; see
[approved dependency updates](docs/RELEASING.md#approved-dependency-updates).

```bash
bash scripts/build-cli.sh       # Build the host CLI at .build/selfishell
bin/selfishell help            # Source launcher executes that existing binary
.build/selfishell install --dry-run
bash scripts/build-cli.sh --all # Cross-build macOS/Linux × AMD64/ARM64
bash scripts/check.sh           # Syntax, format, vet, tests and four builds
```

The source launcher never compiles on invocation. See
[build isolation](docs/ARCHITECTURE.md#development-and-platform-boundary) for
compiler settings and
[local release artifacts](docs/RELEASING.md#optional-local-artifact-check) for
archive builds. Publication follows the [release procedure](docs/RELEASING.md#publish).

`.git-blame-ignore-revs` lists commits that only moved code between packages.
Run `git config blame.ignoreRevsFile .git-blame-ignore-revs` once so `git blame`
skips them; GitHub's blame view reads the file automatically. Package layering
is described in [Go packages](docs/ARCHITECTURE.md#go-packages).

## Tests and repository gate

`bash scripts/check.sh` checks Bash/Zsh syntax, ShellCheck and shfmt, parses the
Neovim Lua through `scripts/check-lua.sh`, then runs Go format, vet, tests and
four target builds through `scripts/check-go.sh`. The Lua parse uses `nvim` (or
`luac`) from PATH and prints a skip line without either; CI runs it in the Neovim
job with the pinned `nvim`.
Go owns maintained setup, process execution, assertions and cleanup. Native
Zsh/Lua probes cover their runtime APIs; small child-process fixtures inject
external failures.

Tests use the current checkout. Integration tests live in `tests/integration`
and cover updates, rollback, backup, restore and interrupted-operation recovery.
`SELFISHELL_TEST_CLI` may select an existing native executable for focused Go test
runs, including signal and benchmark tests. Fixtures that need their own release
root copy the executable so configuration discovery stays local to the fixture.

Integration tests call `t.Parallel()` unless they must change the test process
environment with `t.Setenv`. Give each test its own temporary HOME and pass
child environments explicitly. Write fixture files with `testutil.WriteFile` or
`testutil.AppendFile`, which hold `syscall.ForkLock` so a parallel test's fork
cannot make an executable fixture fail with ETXTBSY; a source check enforces it.
Product code in `internal/selfishell`, `internal/lifecycle`, `internal/release`,
`internal/neovim`, `internal/terminal`, `internal/process`, `internal/managed`
and `internal/report` reads the environment only through
the injected `process.Environment` (another source check), so its tests pass a
private environment instead of calling `t.Setenv`.

The subsystem packages (`internal/terminal`, `internal/release` and
`internal/neovim`) carry only their unit tests; the core's CLI-driving tests in
`internal/selfishell` verify their behaviour, so a change there needs the whole
gate, not `go test ./internal/<package>` alone.

Keep command parsing and shared failure cases focused; do not repeat basic
help/version or successful reinstall checks in every lifecycle scenario.

### Lifecycle and release fixtures

Tests check exit statuses, diagnostic output, personal-file bytes and modes,
managed links, state and backups. WSL shares Ubuntu's configuration implementation,
so its configuration matrix covers
install/restore with empty, existing and XDG homes. Release reproducibility checks
combine a relocated source tree, changed mtimes, hostile build variables and a
fresh cache; the shell builder is compared with those verified Go-built assets.
Lifecycle fixtures reuse the production-built payload with different version
metadata and checksums, preparing only the host archive. Published-release
fixtures still include all four platforms.

### Fixture build caches

Test fixture builds use temporary Go caches by default. `SELFISHELL_TEST_GO_CACHE`
selects an absolute compiler-cache path instead: the repository gate uses
`.build/test-go-cache` in the checkout so repeated runs reuse fixture builds, and
CI passes its restored cache to the gate and native archive E2E fixtures. CI's
pinned `setup-go` action keys its cache from `go.mod`; no `go.sum` is required.
The race job also hashes `scripts/check-go.sh`, so its race builds use a
separate cache key from the gate's.
The Neovim E2E setup also retains that compiler cache when switching to its
temporary home. The test process leaves the caller-owned cache in place. Test
homes, XDG paths and mise state remain private, and the reproducibility check
still uses a fresh cache. Package and tool downloads are not shared between
test homes.

`SELFISHELL_GO_RACE=1 bash scripts/check-go.sh` runs the whole test step with
`-race` and `CGO_ENABLED=1`, so it needs a C compiler; builds, vet and gofmt keep
the normal `CGO_ENABLED=0` policy. CI runs it in a separate Ubuntu job.
CI's gate also sets `SELFISHELL_GO_COVERPROFILE` to an absolute file path, logs
per-package and total coverage, and uploads the profile as a workflow artifact.

## Verification coverage

CI runs on GitHub-hosted `ubuntu-latest` (AMD64) and `macos-latest` (ARM64)
runners. For runtime changes and release tags it runs the repository gate and
an exact prebuilt-archive install on both hosts, a full installation from the
Linux/AMD64 archive in an `ubuntu:24.04` container, the macOS/ARM64 archive's
configuration lifecycle, the pinned Neovim developer lifecycle, and the Go
tests under the race detector on Ubuntu.
The release builder produces four archive formats; only the host's native
archive executes in each job.

Linux/ARM64, macOS/AMD64, WSL 2, and OS releases other than those runner
images and the container are not executed in CI. Platform selectors in tests
simulate them; they are not evidence of runtime execution on another OS or CPU.

### Native WSL terminal checks

On WSL 2 with PowerShell interoperability, run these focused checks:

```sh
SELFISHELL_TEST_WSL_INTEROP=1 go test ./internal/terminal -run '^TestWindowsInteropReadOnly$' -v
SELFISHELL_TEST_WINDOWS_TEMP=/mnt/c/Windows/Temp go test ./internal/selfishell -run '^TestWindowsTerminalNativeFragment$' -v
SELFISHELL_TEST_WSL_FONTS=1 SELFISHELL_TEST_WINDOWS_TEMP=/mnt/c/Windows/Temp go test ./internal/selfishell -run '^TestWindowsFontLoadedNativeUpgrade$' -v
```

Use the actual mapped Windows temporary directory if the mount root differs.
The filesystem check creates and removes its own private directory and writes
and removes a fragment there, leaving a scratch settings file unchanged.
The font check downloads the approved regular TTF, loads it privately in a
short-lived Windows process, and verifies a new version can activate while the
old file is held open with Windows sharing restrictions, then reapplies the old
pin without downloading or replacing its locked file. Persistent registration
is stubbed. None of these checks changes personal Windows Terminal settings or
writes real font registry entries. Font registration and the fragment still need
verification in a disposable Windows user account: accept the initial WSL setup,
check all four styles and Dark+ in Windows Terminal after restarting it and
after Windows logout/login, for both a legacy `Windows.Terminal.Wsl` and a
`Microsoft.WSL` profile; confirm a font under the profile or Defaults takes
precedence and that Windows Terminal writes nothing to `settings.json`; repeat
setup, upgrade the font pin while Windows has loaded the old font, reapply the
previous pin, and confirm uninstall removes the fragment, reverts the
appearance, and leaves fonts available. Also verify an existing font family is
preserved and that `--skip-packages` never registers fonts.
