# Shell Performance

Selfishell measures shell startup and small CLI commands on every Ubuntu and
macOS CI run. The benchmark uses an isolated temporary `HOME`; it never reads or
changes the developer's shell configuration.

Run it locally with:

```sh
bash scripts/benchmark.sh
```

Each metric reports the mean, median (`p50`), 95th percentile (`p95`), and
maximum duration in milliseconds. `interactive-cached` starts a complete
interactive Zsh through the platform `.zshrc`. External integrations found in
`PATH`, such as Starship, fzf, and zoxide, are included and listed in the output.
Zinit is included only when it exists inside the isolated benchmark home, so a
normal local run does not execute or modify the developer's plugin checkout.

`common-first` is the once-per-day completion cache generation cost.
`common-cached` and `interactive-cached` represent ordinary warm startup. The
first-run metric is informational and does not have a performance budget.

## CI budgets

CI currently records budget overruns as warnings. Shared GitHub runners vary in
speed, so the initial budgets are deliberately broad:

| Metric p95 | Ubuntu | macOS |
| --- | ---: | ---: |
| Common configuration | 50 ms | 50 ms |
| Interactive startup | 150 ms | 200 ms |
| CLI version/help | 50 ms | 50 ms |

Every run uploads `shell-performance-<platform>` as a TSV artifact. These
results establish platform-specific baselines before budgets become blocking.
Set `SELFISHELL_BENCHMARK_ENFORCE=1` to make an overrun fail locally or in CI.

The budget variables are:

- `SELFISHELL_BENCHMARK_COMMON_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_INTERACTIVE_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_VERSION_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_HELP_P95_MAX_MS`

## Initial local reference

On 2026-07-16, Linux AMD64 with Starship, fzf, and zoxide available produced an
interactive cached p95 of approximately 77 ms. This is a development-machine
reference, not the Ubuntu CI baseline. CI artifacts are the source for comparing
future results on equivalent hosted platforms.
