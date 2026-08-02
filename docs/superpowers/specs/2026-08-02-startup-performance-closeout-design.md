# Startup Performance Closeout Design

## Goal

Remove network work from ordinary Zsh startup and make the existing performance
evidence trustworthy without sacrificing mise's project switching or Zinit's
plugin-management convenience.

## Decisions and Scope

This slice has three runtime and measurement deliverables:

1. keep Zinit, but provision Selfishell's four pinned Zsh plugins during
   `selfishell install` and tool updates instead of on first shell startup;
2. finish the deterministic benchmark and profiling work already started on
   `agent/quality-closeout`, including the remaining macOS isolation gaps;
3. use Zsh's native executable lookup on macOS and native Ubuntu while retaining
   the filtered manual lookup required by WSL.

Mise activation remains unchanged. Its startup cost is an intentional trade for
automatic project-local tool and environment switching. Zinit remains the
runtime plugin loader. This work does not replace Zinit with a custom plugin
manager, introduce a hard performance gate, or change the selected plugins.

The existing `agent/quality-closeout` work will be rebased onto the latest local
`main`; it must preserve the merged Neovim tree changes and must not restore old
screenshots. The already approved mise-version consistency and platform
verification changes remain part of that branch.

## Zinit Plugin Provisioning

`dependencies.conf` remains the source of truth for the four `zsh-plugin`
repository/commit pairs. After the required Zinit dependency is installed or
confirmed, the package-install path invokes Zinit in a non-interactive Zsh and
uses its `cloneonly` modifier with each reviewed commit to prepare every missing
plugin without sourcing plugin code during installation. This reuses Zinit's
repository naming and storage instead of duplicating a plugin manager inside
Selfishell.

Provisioning is idempotent. Existing Zinit plugin directories are left to
Zinit's existing behavior; the installer does not manually delete, reset, or
replace an occupied plugin path. Pin changes for an existing checkout remain a
separate update concern rather than making this performance fix destructive. A
missing required plugin that cannot be prepared makes the package-install
operation fail rather than leaving a nominally complete profile.
`--skip-packages` and `SELFISHELL_OFFLINE=1` continue to perform
configuration-only installation and do not invoke Zinit or Git.

At shell startup, each `zinit light` call is guarded by the corresponding local
checkout under Zinit's own `ZINIT[PLUGINS_DIR]`. Missing plugins are skipped, not
downloaded. A later explicit `selfishell install` or tools update is the repair
path. This preserves a quiet, network-free shell even after a configuration-only
install or an interrupted plugin provision.

The current pins and loading order remain unchanged:

- `zsh-users/zsh-completions` before `compinit`;
- `Aloxaf/fzf-tab` synchronously when `fzf` is available;
- `zsh-users/zsh-autosuggestions` with `wait'0'`;
- `zdharma-continuum/fast-syntax-highlighting` with `wait'0'`.

## Benchmark Isolation and Diagnostics

Base mode must measure Selfishell rather than tools found on the caller's
machine. It uses one fixed executable path for both integration reporting and
interactive startup. On macOS the benchmark supplies a harmless test-home
`brew` shim so the platform `.zshrc` does not discover the host Homebrew and
reintroduce its ambient path. Benchmark shells start from the isolated test
home, use a normal non-dumb terminal value, and clear ambient mise activation
state.

Full mode continues to provision pinned mise, Starship, and Zinit in the
temporary home and uses explicitly available fzf/zoxide. It may use the network
during benchmark setup, but its measured warm shell startup must use only the
prepared local integrations. The opt-in `zsh/zprof` report is collected after
timed samples and uploaded beside the full-profile TSV in CI.

The full-profile job remains advisory until repeated isolated runs establish a
stable platform baseline. No p95 threshold is added in this slice.

## Executable Lookup

`_selfishell_command_path` uses Zsh's built-in `whence -p` outside WSL. This
avoids repeatedly scanning a long development `PATH` in shell code and returns
only executable path entries rather than aliases or functions.

WSL retains the existing manual directory loop so inherited Windows entries
under `/mnt/<drive>/...` are never probed. The function's output and success or
failure status remain unchanged for callers.

## Failure Handling and Compatibility

- All installer changes remain compatible with macOS Bash 3.2.
- Plugin provisioning uses the already required Zsh, Git, and Zinit; no new
  dependency is added.
- Missing local plugins never trigger a network request during shell startup.
- Configuration-only and dry-run installation create no plugin state.
- Existing user or Zinit-managed plugin directories are never deleted merely
  because Selfishell did not create them.
- Plugin provisioning honors the existing proxy environment through Git/Zinit.

## Verification

- Add a failing installer test proving all four manifest pins are passed to
  non-interactive Zinit provisioning after `zinit` installation.
- Add tests proving dry-run, `--skip-packages`, and offline configuration do not
  provision plugins.
- Add a Zsh startup test where Zinit exists but plugin checkouts do not; it must
  execute no plugin installation or network-capable `zinit light` command.
- Add focused native and WSL lookup tests before changing
  `_selfishell_command_path`.
- Extend benchmark tests so ambient Homebrew integrations cannot re-enter base
  mode on macOS and the integration summary matches what the shell actually
  executes.
- Run focused installer, common-Zsh, benchmark, workflow, and documentation
  suites while iterating, then run `bash scripts/check.sh` because installer,
  shell startup, dependency, and CI behavior change.
- Run isolated base and full benchmarks after the tests pass. Record local
  results separately from GitHub Actions; do not claim the changed workflow has
  run until its PR checks finish.
