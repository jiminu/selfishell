# Shell Performance

Selfishell measures shell startup and small CLI commands on every Ubuntu and
macOS CI run. The benchmark uses an isolated temporary `HOME`; it never reads or
changes the developer's shell configuration.

Run it locally with:

```sh
bash scripts/benchmark.sh --mode base
bash scripts/benchmark.sh --mode full
```

`SELFISHELL_BENCHMARK_PROFILE=base|full` is equivalent to `--mode`:

```sh
SELFISHELL_BENCHMARK_PROFILE=full bash scripts/benchmark.sh
```

Each metric reports the mean, median (`p50`), 95th percentile (`p95`), and
maximum duration in milliseconds. `interactive-cached` starts a complete
interactive Zsh through the platform `.zshrc`.

### Base mode

Base mode is the default, and what CI runs on every push/PR. It measures
Selfishell's own startup cost independent of external integrations: Starship,
fzf, zoxide, and Zinit are included only if they already happen to be on
`PATH` (Zinit only if it exists inside the isolated benchmark home), so a
normal local run does not execute or modify the developer's plugin checkout.

### Full-profile mode

Full mode additionally provisions the pinned mise, Starship, and Zinit -- with
its pinned Zsh plugins -- into the benchmark's own isolated `HOME`, via the
same code path the real installer uses, so `interactive-cached` reflects a
real developer-profile startup rather than whatever happens to already be on
the runner's `PATH`. It:

- uses an isolated, temporary `HOME`; the real user `HOME` is never read or
  changed;
- installs the pinned mise, Starship, and Zinit (with its pinned plugins)
  into that isolated `HOME`;
- measures fzf and zoxide only if they are already on `PATH` -- installing
  packages is out of scope for this script, so provision them via the
  platform package manager first;
- needs network access to provision those tools, so it is not part of the
  regular (network-free) unit test suite;
- runs in CI as its own `shell-full-profile-benchmark` job (Ubuntu only,
  after installing fzf/zoxide via `apt`), separate from the base benchmark
  that runs on every push/PR.

`common-first` is the once-per-day completion cache generation cost.
`common-cached` and `interactive-cached` represent ordinary warm startup. The
first-run metric is informational and does not have a performance budget.

## CI budgets

CI currently records budget overruns as warnings rather than failures:
`SELFISHELL_BENCHMARK_ENFORCE` defaults to `0`, so a miss is reported as an
observation, never a failing check. The base benchmark is the only one with
budget thresholds currently configured; shared GitHub runners vary in speed,
so those initial budgets are deliberately broad:

| Metric p95 | Ubuntu | macOS |
| --- | ---: | ---: |
| Common configuration | 50 ms | 50 ms |
| Interactive startup | 150 ms | 200 ms |
| CLI version/help | 50 ms | 50 ms |

The full-profile benchmark runs under the same warn-only policy
(`SELFISHELL_BENCHMARK_ENFORCE=0`) but does not currently set any
`*_P95_MAX_MS` variable, so it reports metrics for comparison without
budget-overrun warnings.

Every run uploads a TSV artifact: `shell-performance-<platform>` for the base
benchmark (one per OS in the CI matrix) and `shell-performance-full-profile`
for the full-profile benchmark. These results establish platform-specific
baselines before budgets become blocking. Set `SELFISHELL_BENCHMARK_ENFORCE=1`
to make an overrun fail locally or in CI.

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
