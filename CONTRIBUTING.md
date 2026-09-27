# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes need Go-owned tests with private `HOME`, XDG, mise and temporary paths.
Go owns maintained setup, process execution, assertions and cleanup. Keep
native Zsh/Lua probes for their runtime APIs and fixed Bash fixtures only for
old state/protocol comparisons and external fault injection.

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

Compatibility tests require full local Git history. They export immutable
Bash reference `3bbbfa0346ee74eb47f31a81ec666340a5ef6018` and actual
v1.3.1 `d025710338036f1f54b948f1f3e5c17a0b3f7e38`; missing history fails
without fetching or substituting another revision. CI uses `fetch-depth: 0`.
The fixed Bash executable is a comparison fixture, never a selectable product
engine. `SELFISHELL_TEST_CLI` may select an existing native executable for
focused Go test runs.

`scripts/check-go.sh` runs Go format, vet, tests and target builds through the
repository gate. Four archive formats are checked; host-native execution occurs
on each CI host. Platform selectors in tests are simulations, not evidence of
runtime execution on another OS or CPU. See [CLI compatibility](docs/CLI_COMPATIBILITY.md)
for reference boundaries and [release procedure](docs/RELEASING.md) for manual
publication.
