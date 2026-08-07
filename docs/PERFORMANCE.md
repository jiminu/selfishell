# Shell Performance

Selfishell can measure shell startup and small CLI commands with
`scripts/benchmark.sh`, run manually when needed. The benchmark uses an
isolated temporary `HOME`; it never reads or changes the developer's shell
configuration.

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

Base mode is the default. It measures
Selfishell's own startup cost independent of external integrations: Starship,
fzf, zoxide, and Zinit are excluded even if they are installed on the caller's
`PATH`. The fixed benchmark path makes results independent of tools installed
on the developer machine. Every measured shell starts from the isolated home
directory with an empty isolated mise config. On macOS, base mode also places a
benchmark-only no-op `brew` ahead of Homebrew discovery so `brew shellenv`
cannot reintroduce host tools. A normal local run therefore does not read the
developer's mise configuration or execute or modify the developer's plugin
checkout.

### Full-profile mode

Full mode additionally provisions the pinned mise, Starship, and Zinit -- with
its pinned Zsh plugins -- into the benchmark's own isolated `HOME`, via the
same code path the real installer uses, so `interactive-cached` reflects a
real developer-profile startup rather than whatever happens to already be on
the runner's `PATH`. It:

- uses an isolated, temporary `HOME`; the real user `HOME` is never read or
  changed;
- uses that home as its working directory and gives mise an isolated global
  config;
- installs the pinned mise, Starship, and Zinit (with its pinned plugins)
  into that isolated `HOME`;
- measures fzf and zoxide only if they are already on `PATH` -- installing
  packages is out of scope for this script, so provision them via the
  platform package manager first;
- needs network access to provision those tools, so it is not part of the
  regular (network-free) unit test suite, or run in CI -- run it locally
  when needed.

`common-first` is the once-per-day completion cache generation cost.
`common-cached` and `interactive-cached` represent ordinary warm startup. The
first-run metric is informational and does not have a performance budget.

## Startup profiling

Set `SELFISHELL_BENCHMARK_ZPROF_FILE` to collect one Zsh profiler report after
the normal timed measurements:

```sh
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-startup.zprof \
  bash scripts/benchmark.sh --mode full
```

The report ranks initialization functions by time and is intended for finding
expensive startup paths. The benchmark loads Zsh's built-in `zsh/zprof` module
only for this additional diagnostic startup, after every reported metric and
budget check has completed. It adds no code or dependency to ordinary shell
startup and does not enforce a performance threshold.

## Budgets

Budgets are opt-in and unset by default. `SELFISHELL_BENCHMARK_ENFORCE`
defaults to `0`, so a budget miss is reported as an observation rather than a
failing check; set it to `1` to make an overrun fail. Set any of the
`*_P95_MAX_MS` variables below to check a metric against a threshold you
choose.

The budget variables are:

- `SELFISHELL_BENCHMARK_COMMON_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_INTERACTIVE_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_VERSION_P95_MAX_MS`
- `SELFISHELL_BENCHMARK_HELP_P95_MAX_MS`
