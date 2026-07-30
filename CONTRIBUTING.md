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

See [AGENTS.md](AGENTS.md) for repository implementation constraints and
[docs/RELEASING.md](docs/RELEASING.md) for maintainer release procedures.
