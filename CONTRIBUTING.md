# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes require isolated tests using a temporary `HOME`.

Keep multiline data and embedded-language programs under `tests/fixtures/`
instead of compressing them into shell command strings. Reuse the isolated
sequential or parallel runners in `tests/test_helper.bash` rather than adding a
suite-specific runner.

## Local development

```bash
./bin/selfishell help
./bin/selfishell install --dry-run
bash scripts/check.sh
```

The developer gate requires Go **1.27.1** on `PATH` (pinned in `go.mod`),
Python 3, Zsh, ShellCheck and shfmt. Go belongs to development and CI only;
it is not added to the installed environment. The build uses `GOTOOLCHAIN=local`
and fails on a different version instead of downloading another toolchain.

```bash
bash scripts/build-cli.sh       # Native candidate: .build/selfishell
bash scripts/build-cli.sh --all # macOS/Linux × AMD64/ARM64
bash scripts/check-go.sh        # fmt, vet, tests, builds and native comparisons
.build/selfishell help
```

The Go candidate implements help/local version, configuration-only
`install --skip-packages`, and `uninstall` (including restore and purge).
Commands such as update, status, doctor, rollback, and `version --available`
remain incomplete. Use `bin/selfishell` for production behavior during the
migration; the complete integration suite is not yet supported by the candidate.
The Go gate compares the native binary against the fixed reference at identical
paths, including generated VERSION files, symlinks, stderr terminals and
`NO_COLOR`, and runs help/version with no tools on PATH. Cross-built binaries
are not reported as executed on platforms other than the actual test host.

Set `SELFISHELL_TEST_CLI` to an absolute executable path to run the ordinary
CLI integration calls in `cli_test.bash`, `managed_install_test.bash`,
`package_manifest_test.bash`, and `updates_test.bash` against another candidate.
Without it, they run `bin/selfishell`. The override is executed directly, so
its shebang selects the interpreter. Some tests still invoke the Bash CLI or a
copied release payload explicitly; this does not make the full suite candidate
compatible.

The compatibility tests require the repository's full Git history. They never
fetch during a test run. For a shallow
checkout, run `git fetch --unshallow` before testing. CI checkouts that run the
gate use `fetch-depth: 0`.

```bash
go test ./tests/migration -run '^TestBaseline$' -count=1
go test ./tests/migration -run '^TestFoundation$' -count=1
go test ./tests/migration -run '^TestConfig' -count=1
```

The baseline compares the fixed Bash reference with itself and tests the actual
legacy release. Config tests compare the fixed reference with the Go candidate
at the same temporary paths. `SELFISHELL_TEST_CLI` may name an absolute native
candidate for Go migration tests; it does not replace fixed reference runs.
The remaining Bash feature suites still use `tests/run.bash`,
`tests/test_helper.bash`, and `tests/cli_runner.bash`. Go compatibility tests
retain the `cksum.bash` and `state_bridge.bash` fixtures, while migration tests
retain `date.bash` only as a fixed external backup clock.
See [CLI compatibility](docs/CLI_COMPATIBILITY.md) for the pinned references,
comparison boundaries and native-platform verification requirements.

See [AGENTS.md](AGENTS.md) for repository implementation constraints and
[docs/RELEASING.md](docs/RELEASING.md) for maintainer release procedures.
