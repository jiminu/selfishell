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
maximum duration in milliseconds. `interactive-cached` loads the platform
`.zshrc` and exits; it does not measure a visible prompt, command-to-prompt
latency, or deferred plugin readiness. Use `--prompt` for PTY prompt timing.

## Base mode

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

## Full-environment mode

Full mode provisions pinned mise, Starship, fzf, zoxide, Zinit, and its Zsh
plugins through the production installers into the temporary `HOME`. It uses
the release's exact pins and an isolated mise configuration, so measurements
include the managed shell integrations without changing the developer's tools
or plugin checkouts. Provisioning requires network access; run it locally when
needed, outside CI and the network-free test suite.

## Prompt and diagnostic measurements

```sh
bash scripts/benchmark.sh --mode full --prompt --diagnostics
```

Both options are independent and also work in network-free `base` mode.
They use the same iteration count (`SELFISHELL_BENCHMARK_ITERATIONS`, default
30) and append metrics to `SELFISHELL_BENCHMARK_RESULTS_FILE` when set.
They have no enforced timing budgets.

`--prompt` requires Python 3, using only its standard library. It measures
`prompt-first-{empty,repository}` and `prompt-command-{empty,repository}` in
a 160-column PTY with the managed Starship configuration. Each scenario warms
one shell before collecting samples. Every measured shell runs three `:`
commands after a fixed 100 ms settling interval. A numbered prompt marker
distinguishes a new command cycle from an editing redraw. Timings include shell
process creation for the first prompt and run until the marker reaches the PTY;
they exclude GUI terminal rendering. The settling interval does not prove
deferred plugins are ready. Repository timings depend on this checkout's size
and working-tree state; language projects may have different costs.

`--diagnostics` applies configuration with `install --skip-packages --yes` in
a separate temporary HOME, then measures `cli-status` and `cli-doctor` after
one warm-up invocation each. It prints that invocation's output and exit code
to identify the measured state; a changed exit code during sampling fails the
benchmark. Exit 1 is expected when required tools are missing. Both modes query
available system package inventories. Base mode uses `/usr/bin:/bin`; full mode
also shares the temporary pinned shell tools and Zinit plugins and includes
the caller's PATH tools and package managers.
It does not install the remaining developer tools or system packages, so these
results describe a partially provisioned environment, not a complete setup.
HOME, XDG directories, and mise data/cache/state paths point into the temporary tree.
Prompt and diagnostic measurements also bound mise's ancestor configuration
search with `MISE_CEILING_PATHS`, excluding settings above the measured directory
or source root.

## Startup caches

Interactive startup audits completion directories on first use and once daily.
The `.zcompdump.audit` marker records the audit separately from `.zcompdump`,
which can be reused without rewriting it. Insecure entries are excluded without
a prompt. After a clean audit, warm startups reuse the dump without another
audit. Insecure paths are audited on each startup until repaired, so restoring
the completion search path cannot reintroduce an excluded directory. A replaced
audit marker (a symlink, nonempty file, or another path type) is preserved and
also causes startup to audit again.

fzf, zoxide, and Starship initialization caches survive unchanged installs and
configuration reapplication. A changed `zsh/interactive.zsh` generator invalidates
these caches before installation; unrelated managed configuration changes leave
them intact. Startup compares the resolved executable path and file identity
using Zsh's built-in stat module, so replacing a tool or rolling back to an older
binary regenerates its initialization even when its timestamp is preserved.
The identity comment and syntax-checked initialization are activated together by
an atomic rename. Failed generation retains the previous cache and retries on
the next startup. Older caches without an identity comment regenerate once.
Identity checks use whole-second timestamps rather than hashing the executable
on every startup. A same-size edit within the same second that preserves both
the inode and mtime can go undetected; remove that tool's init cache to regenerate it.

`common-first` measures initial completion cache generation.
`common-cached` and `interactive-cached` represent ordinary warm startup. The
first-run metric is informational and does not have a performance budget.

## Startup profiling

Set `SELFISHELL_BENCHMARK_ZPROF_FILE` to collect one Zsh profiler report after
the normal timed measurements:

```sh
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-startup.zprof \
  bash scripts/benchmark.sh --mode full
```

The report ranks initialization functions by time. The profiler runs once after
all timed measurements and budget checks; it adds no overhead to ordinary
startup and enforces no threshold.

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
