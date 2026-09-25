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

Set `SELFISHELL_TEST_CLI` to an absolute executable path to run the ordinary
CLI integration calls in `cli_test.bash`, `managed_install_test.bash`,
`package_manifest_test.bash`, and `updates_test.bash` against another candidate.
Without it, they run `bin/selfishell`. The override is executed directly, so
its shebang selects the interpreter. Some tests still invoke the Bash CLI or a
copied release payload explicitly; this does not make the full suite candidate
compatible.

See [AGENTS.md](AGENTS.md) for repository implementation constraints and
[docs/RELEASING.md](docs/RELEASING.md) for maintainer release procedures.
