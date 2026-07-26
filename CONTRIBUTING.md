# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes require isolated tests using a temporary `HOME`.

## Local development

```bash
./bin/selfishell help
./bin/selfishell install --dry-run
bash scripts/check.sh
```

See [AGENTS.md](AGENTS.md) for repository implementation constraints and
[docs/RELEASING.md](docs/RELEASING.md) for maintainer release procedures.
