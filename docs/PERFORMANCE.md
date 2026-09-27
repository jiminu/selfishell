# Shell Performance

Selfishell can measure shell startup and small CLI commands with
`scripts/benchmark.sh`, run manually when needed. The Go runner builds in the
current checkout before sampling and requires an already-built `.build/selfishell`
CLI (run `bash scripts/build-cli.sh`). The benchmark uses an isolated temporary
`HOME`; it never reads or changes the developer's shell configuration.

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

`--prompt` uses the native Go PTY probe. It measures
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
also causes startup to audit again. A completion directory modified after the
dump, such as one that gained a newly installed tool's completion, also takes
the audited rebuild; comparing directory mtimes keeps that check under 1 ms.

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

## Matched cutover measurement

These are wall-clock milliseconds on one Mac15,6, Darwin arm64, macOS 27.0
(build 26A428), 11 logical CPUs, Go 1.27.1. The fixed Bash CLI/config root was
`3bbbfa0346ee74eb47f31a81ec666340a5ef6018`; the native CLI was built from
`fed3823a28cae906ccd40c4a0a38701c77cbfc10` plus the final benchmark
PATH and PTY session fixes. The actual v1.3.1 commit
`d025710338036f1f54b948f1f3e5c17a0b3f7e38` is a separate interoperability
reference, not a performance input. The old and current package, dependency
and mise pin manifests were byte-identical. Their Zsh configuration differs
only by a comment in `update-notice.zsh`; cached Zsh variation should not be
attributed to the CLI language.

The same current Go benchmark driver selected either immutable Bash export or
native executable and matching configuration root. Both executables and the
driver were built/prepared before sampling; no compilation occurred within a
sample. Each profile used a fresh private HOME, XDG and mise tree. Base excluded
ambient integrations; full provisioned pinned private mise, Starship, fzf,
zoxide and Zinit, all confirmed active before samples. The benchmark's shell
and CLI process boundaries, warmups, diagnostic setup, 160-column PTY and
three command-to-prompt cycles were identical for each engine. Status and
doctor used a configured but partially provisioned private HOME and exited 1
for both engines. Full diagnostic homes link to the same private pinned mise
binary used by the shell fixture; both engines then report 8 tools present
and 12 missing. An initial full diagnostic run omitted that link: Bash counted
mise on PATH, while Go found no direct-install target in the diagnostic HOME.
Those unmatched diagnostic rows were superseded by the full diagnostic rerun
shown here. Other full shell/CLI rows use the original run, and prompt rows
use final direct-runner sessions. The first `common-first` sample is the separate cold cache
cost, not part of the 30 warm samples; it is sensitive to cache generation and
varied materially between runs. Prompt first has 30 measured shells after one
warmup; prompt command has 90 cycles.

The workstation was not dedicated or idle. Aggregate CPU idle around this work
was roughly 63–73%, load average about 3.6–4.4; there were no interval swap-ins
or swap-outs in the recorded preparation sample. Ordinary background work
continued. Runs were matched on the same host and close in time but were not
fully interleaved, so load drift and cache effects remain possible. One native
base `cli-version` sample reached 447.541 ms; its mean is therefore 18.096 ms
while its p50 is 3.115 ms. Preserve that outlier in interpretation. The full
repository prompt-command mean was 25.329 ms for Bash and 37.195 ms for Go;
that difference reflects the measured shell/workstation runs and is not a
claim that changing the CLI language accelerated shell startup.

### Base profile

| Metric | n | Bash mean / p50 / p95 / max | Go mean / p50 / p95 / max |
| --- | ---: | --- | --- |
| baseline-zsh | 30 | 3.738 / 3.518 / 4.694 / 4.773 | 3.961 / 3.591 / 5.712 / 8.838 |
| common-first | 1 | 163.927 / 163.927 / 163.927 / 163.927 | 165.520 / 165.520 / 165.520 / 165.520 |
| common-cached | 30 | 13.285 / 13.232 / 13.899 / 13.940 | 13.072 / 12.967 / 13.602 / 14.712 |
| interactive-cached | 30 | 13.500 / 13.448 / 14.010 / 14.030 | 13.406 / 13.344 / 13.874 / 14.145 |
| cli-version | 30 | 4.002 / 3.990 / 4.254 / 4.452 | 18.096 / 3.115 / 5.070 / 447.541 |
| cli-help | 30 | 5.934 / 5.919 / 6.250 / 6.270 | 2.790 / 2.778 / 2.890 / 3.126 |
| cli-status | 30 | 132.341 / 131.413 / 141.933 / 142.397 | 46.830 / 46.580 / 48.961 / 49.068 |
| cli-doctor | 30 | 64.945 / 64.739 / 68.995 / 69.120 | 19.810 / 19.718 / 20.643 / 20.753 |
| prompt-first-empty | 30 | 38.283 / 43.937 / 50.931 / 51.266 | 39.463 / 43.778 / 49.999 / 50.430 |
| prompt-command-empty | 90 | 1.442 / 1.644 / 2.465 / 2.854 | 1.511 / 1.640 / 2.457 / 2.759 |
| prompt-first-repository | 30 | 37.145 / 39.710 / 50.272 / 51.360 | 40.937 / 43.465 / 51.312 / 54.292 |
| prompt-command-repository | 90 | 1.349 / 1.594 / 2.400 / 2.521 | 1.622 / 1.706 / 2.406 / 2.563 |

### Full profile

| Metric | n | Bash mean / p50 / p95 / max | Go mean / p50 / p95 / max |
| --- | ---: | --- | --- |
| baseline-zsh | 30 | 3.423 / 3.204 / 4.617 / 6.247 | 3.015 / 2.929 / 3.839 / 3.876 |
| common-first | 1 | 1534.561 / 1534.561 / 1534.561 / 1534.561 | 695.149 / 695.149 / 695.149 / 695.149 |
| common-cached | 30 | 69.509 / 66.789 / 88.407 / 106.085 | 67.323 / 66.807 / 70.618 / 74.923 |
| interactive-cached | 30 | 82.068 / 81.002 / 86.131 / 87.194 | 80.982 / 80.713 / 84.361 / 86.094 |
| cli-version | 30 | 4.008 / 3.984 / 4.329 / 4.363 | 2.677 / 2.699 / 2.820 / 3.058 |
| cli-help | 30 | 5.975 / 5.965 / 6.275 / 6.288 | 2.620 / 2.626 / 2.731 / 2.805 |
| cli-status | 30 | 147.034 / 146.453 / 150.535 / 151.157 | 59.771 / 59.291 / 62.945 / 68.602 |
| cli-doctor | 30 | 124.383 / 123.840 / 128.078 / 134.418 | 106.163 / 105.774 / 111.704 / 112.132 |
| prompt-first-empty | 30 | 111.262 / 109.654 / 114.826 / 153.458 | 113.369 / 112.139 / 119.937 / 125.946 |
| prompt-command-empty | 90 | 25.368 / 21.777 / 35.280 / 36.373 | 25.992 / 22.735 / 35.108 / 35.908 |
| prompt-first-repository | 30 | 110.098 / 109.180 / 118.265 / 120.434 | 123.578 / 124.096 / 128.805 / 130.981 |
| prompt-command-repository | 90 | 25.329 / 21.876 / 35.380 / 36.464 | 37.195 / 34.130 / 47.456 / 47.958 |

### Configuration-only lifecycle

A separate precompiled Go fixture driver used 30 fresh private homes per engine
and operation. `install --skip-packages --yes` was timed on an empty home;
`update --tools-only --skip-packages --yes` and `uninstall --restore --yes`
were each timed after an untimed config-only install into a fresh home. Every
sample exited 0. Its controlled PATH placed failing Apt, Homebrew, sudo, chsh,
curl and wget guards first; none was called. No real package manager, login
shell mutation, or developer HOME was involved. Setup, fixture reset and
cleanup are excluded from the operation times.

| Operation | n/engine | Bash mean / p50 / p95 / max | Go mean / p50 / p95 / max |
| --- | ---: | --- | --- |
| install | 30 | 637.327 / 636.998 / 646.405 / 662.025 | 300.292 / 300.506 / 320.028 / 327.640 |
| update | 30 | 283.399 / 281.978 / 290.454 / 294.225 | 189.986 / 191.151 / 197.801 / 198.882 |
| uninstall | 30 | 323.681 / 322.975 / 334.952 / 340.743 | 125.573 / 127.182 / 132.468 / 133.494 |

### Size and scope

The historical CLI launcher is 3,168 bytes and the fixed Bash archive contains
229,741 bytes of regular payload per target. The native source launcher is 747
bytes; the measured Darwin/ARM64 development executable is 5,581,666 bytes.
The native release archive's executable may differ from that development file,
so archive payload bytes are reported separately. Exact `0.0.0-measure`
release builds supplied these sizes; archive labels do not establish runtime
execution on all targets.

| Artifact | Bash reference | Native |
| --- | ---: | ---: |
| linux-amd64 compressed / regular payload bytes | 56,408 / 229,741 | 3,132,406 / 5,234,694 |
| linux-arm64 compressed / regular payload bytes | 56,408 / 229,741 | 2,911,704 / 5,043,273 |
| macos-amd64 compressed / regular payload bytes | 56,408 / 229,741 | 2,986,688 / 5,058,318 |
| macos-arm64 compressed / regular payload bytes | 56,408 / 229,741 | 2,818,156 / 4,827,680 |

The direct runner initially could not fork its PTY child when its process led
a terminal-less session: opening the PTY slave acquired it before the child
called `setsid`. The Darwin and Linux PTY open paths now use `O_NOCTTY`.
A session-leader regression failed with the original code and passed with the
fix; all four direct-runner prompt profiles above then completed. An earlier
Go-test-hosted prompt dataset is retained with the local raw evidence, but the
table uses the final direct-runner samples. Full-profile results prove private
provisioning on this macOS host only. Actual WSL and Ubuntu 26.04 measurements
were unavailable; no timings are extrapolated to them.
