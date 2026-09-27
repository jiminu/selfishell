# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes need Go-owned tests with private `HOME`, XDG, mise and temporary paths.
Go owns maintained setup, process execution, assertions and cleanup. Keep
native Zsh/Lua probes for their runtime APIs and small child-process fixtures
for external fault injection.

## Local development

Go **1.27.1** is pinned in `go.mod` for development and CI. The installed
product does not need Go. The build uses `GOTOOLCHAIN=local` and fails if that
version is unavailable. Zsh, ShellCheck, shfmt and Neovim are needed for the
repository gate.

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

Tests use the current checkout and do not require old Bash commits or full Git
history. The Bash-to-Go transition uses a fresh installation; in-place upgrades
and rollback between the Bash and Go implementations are outside the supported
contract. Go-to-Go updates, rollback, backup, restore and interrupted-operation
recovery remain covered. `SELFISHELL_TEST_CLI` may select an existing native
executable for focused Go test runs.

`scripts/check-go.sh` runs Go format, vet, tests and target builds through the
repository gate. Four archive formats are checked; host-native execution occurs
on each CI host. Platform selectors in tests are simulations, not evidence of
runtime execution on another OS or CPU. See [CLI compatibility](docs/CLI_COMPATIBILITY.md)
for supported behavior and [release procedure](docs/RELEASING.md) for manual
publication.
