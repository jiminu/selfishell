# Startup Performance Closeout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep mise and Zinit usability while removing plugin downloads from shell startup, hardening performance evidence, and reducing optional-command lookup cost.

**Architecture:** Rebase the approved `agent/quality-closeout` measurement work onto current `main`, then reuse Zinit's `cloneonly` installation mode to prepare manifest-pinned plugins during explicit package operations. Runtime Zinit declarations load only existing local checkouts, and native Zsh executable discovery replaces manual scanning except on WSL.

**Tech Stack:** macOS Bash 3.2-compatible shell, Zsh, Zinit 3.15.0, Git, GitHub Actions YAML, existing Bash test runner, Zsh `zsh/zprof`.

## Global Constraints

- Keep mise activation unchanged.
- Keep Zinit as the runtime plugin manager and preserve the four selected plugins and their loading order.
- Ordinary shell startup must never install plugins or block on the network.
- `dependencies.conf` remains the Zsh plugin repository and commit source of truth.
- Existing plugin directories are never manually deleted, reset, or replaced by this change.
- `--skip-packages`, `SELFISHELL_OFFLINE=1`, and dry-run remain non-mutating and network-free.
- Keep installer and shared Bash compatible with macOS Bash 3.2.
- Keep full-profile performance budgets advisory; add no hard threshold.
- Preserve PR #94's Neovim tree configuration and the user's current screenshots.

---

### Task 1: Rebase and verify the existing quality-closeout work

**Files:**
- Existing worktree: `.worktrees/quality-closeout`
- Preserve: `common/nvim/lua/plugins/ui.lua`
- Preserve: `img/nvim.png`
- Preserve: `img/selfishell.png`

**Interfaces:**
- Consumes: local `main` containing PR #94 and the approved quality/startup specs.
- Produces: `agent/quality-closeout` rebased onto current `main`, retaining its benchmark, CI mise-selector, and platform-verification commits.

- [ ] **Step 1: Verify both worktrees are clean**

Run:

```sh
git status --short
git -C .worktrees/quality-closeout status --short
```

Expected: both outputs are empty.

- [ ] **Step 2: Rebase the existing branch onto local main**

Run:

```sh
git -C .worktrees/quality-closeout rebase main
```

Expected: the three implementation commits are replayed after the current design commits without reintroducing old Neovim assets.

- [ ] **Step 3: Verify the rebase did not change PR #94 assets**

Run:

```sh
git -C .worktrees/quality-closeout diff --name-status main...HEAD
git -C .worktrees/quality-closeout diff main...HEAD -- common/nvim/lua/plugins/ui.lua img/nvim.png img/selfishell.png
```

Expected: the first command lists only quality-closeout files; the second command is empty.

- [ ] **Step 4: Run the existing focused quality tests**

Run from the linked worktree:

```sh
bash tests/benchmark_test.bash
bash tests/github_actions_pins_test.bash
bash tests/docs_test.bash
```

Expected: all three suites pass before adding the new runtime work.

### Task 2: Provision pinned Zinit plugins during explicit package work

**Files:**
- Modify: `tests/installers_test.bash`
- Modify: `lib/installers.sh`

**Interfaces:**
- Consumes: `zsh-plugin` records from `dependencies_manifest_path`; installed `${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git/zinit.zsh`.
- Produces: `install_zinit_plugins`, returning success only after every missing manifest plugin is accepted by Zinit's `cloneonly` path.

- [ ] **Step 1: Write the failing provisioning test**

Add a test fixture that writes a fake `zinit.zsh` defining a `zinit` function which appends its arguments to `$SELFISHELL_TEST_ZINIT_LOG`. Write a temporary manifest containing the four real `zsh-plugin` record shapes, call `install_zinit_plugins`, and assert each repository appears in one `light` call and each commit appears in its preceding `ice cloneonly ver<commit>` call.

The core assertions are:

```bash
grep -Fqx "ice cloneonly ver$completion_commit" "$SELFISHELL_TEST_ZINIT_LOG" ||
  fail "zsh-completions was not provisioned at its approved commit"
grep -Fqx 'light zsh-users/zsh-completions' "$SELFISHELL_TEST_ZINIT_LOG" ||
  fail "zsh-completions was not passed to Zinit"
[[ "$(grep -c '^light ' "$SELFISHELL_TEST_ZINIT_LOG")" -eq 4 ]] ||
  fail "Installer did not provision exactly four Zsh plugins"
```

- [ ] **Step 2: Run the installer suite and verify RED**

Run `bash tests/installers_test.bash`.

Expected: FAIL because `install_zinit_plugins` does not exist.

- [ ] **Step 3: Implement minimal non-interactive Zinit provisioning**

Add this Bash 3.2-compatible shape to `lib/installers.sh`:

```bash
install_zinit_plugins() {
  local data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  local manifest
  local zinit_script="$data_home/zinit/zinit.git/zinit.zsh"

  [[ "${SELFISHELL_OFFLINE:-0}" != 1 ]] || return 0
  manifest="$(dependencies_manifest_path)"
  [[ -r "$zinit_script" && -r "$manifest" ]] || return 1

  command zsh -f -c '
    source "$1" || exit 1
    while read -r type repository revision _ _ _ _ _ _; do
      [[ "$type" == zsh-plugin ]] || continue
      zinit ice cloneonly "ver${revision}" || exit 1
      zinit light "$repository" || exit 1
    done <"$2"
  ' zsh "$zinit_script" "$manifest"
}
```

Split the `install_direct_package` case so `zinit` runs `dependency_install` and then `install_zinit_plugins`; `starship` and `mise` keep their current path. Preserve the existing early dry-run return.

- [ ] **Step 4: Verify GREEN and failure propagation**

Add one test whose fake Zinit returns nonzero for a selected plugin and assert `install_zinit_plugins` fails. Run:

```sh
bash tests/installers_test.bash
bash tests/proxy_test.bash
```

Expected: both pass; the child Zsh inherits the existing proxy environment without new filtering.

- [ ] **Step 5: Commit installer provisioning**

```sh
git add tests/installers_test.bash lib/installers.sh
git commit -m "feat: provision Zinit plugins during install"
```

### Task 3: Prevent runtime Zinit from installing missing plugins

**Files:**
- Modify: `tests/common_zsh_test.bash`
- Modify: `common/completion.zsh`
- Modify: `common/interactive.zsh`

**Interfaces:**
- Consumes: `ZINIT[PLUGINS_DIR]` from sourced Zinit and repository identifiers already present in configuration.
- Produces: `_selfishell_zinit_plugin_ready REPOSITORY`, true only when the corresponding Zinit checkout directory exists.

- [ ] **Step 1: Write the failing no-download startup test**

Create a fake benchmark-home Zinit script:

```zsh
typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="$HOME/.local/share/zinit/plugins"
zinit() {
  print -r -- "$*" >>"$HOME/zinit-calls"
}
```

Source `common/common.zsh` with an empty plugins directory, `PATH=/usr/bin:/bin`, and `SELFISHELL_UPDATE_NOTICE=0`. Assert the call log contains no line beginning with `light `.

- [ ] **Step 2: Run the common-Zsh suite and verify RED**

Run `bash tests/common_zsh_test.bash`.

Expected: FAIL because current `zinit light` calls run even when their checkout directories are absent.

- [ ] **Step 3: Add one local-checkout guard and apply it to all four plugins**

After sourcing Zinit in `common/completion.zsh`, define:

```zsh
_selfishell_zinit_plugin_ready() {
  local repository="$1"
  local directory="${repository//\//---}"
  [[ -n "${ZINIT[PLUGINS_DIR]:-}" && -d "$ZINIT[PLUGINS_DIR]/$directory" ]]
}
```

Require the helper before each `zinit light` call. Keep the existing ice modifiers, pins, conditions, and ordering byte-for-byte otherwise. Leave the helper available to `interactive.zsh`, then unset it at the end of that module with `unfunction _selfishell_zinit_plugin_ready 2>/dev/null`.

- [ ] **Step 4: Verify missing plugins are skipped and installed plugins still load**

Add a second test creating all four normalized checkout directories and assert the fake log contains all four existing `light` calls. Run `bash tests/common_zsh_test.bash`.

Expected: both tests and the existing pin-consistency test pass.

- [ ] **Step 5: Commit the network-free runtime guard**

```sh
git add tests/common_zsh_test.bash common/completion.zsh common/interactive.zsh
git commit -m "fix: keep Zinit plugin loading network-free"
```

### Task 4: Use native command lookup outside WSL

**Files:**
- Modify: `tests/common_zsh_test.bash`
- Modify: `common/common.zsh`

**Interfaces:**
- Consumes: command name and Zsh `path`; optional `WSL_DISTRO_NAME`.
- Produces: unchanged `_selfishell_command_path COMMAND` output/status contract.

- [ ] **Step 1: Write a failing native lookup edge-case test**

Create executable `probe` in the test working directory, set Zsh `path` to an empty first element followed by system directories, clear `WSL_DISTRO_NAME`, and call `_selfishell_command_path probe`. Assert it succeeds and returns `probe`. The existing loop incorrectly probes `/probe`, so this test is RED before the native lookup change.

- [ ] **Step 2: Verify RED**

Run `bash tests/common_zsh_test.bash`.

Expected: FAIL because the current manual loop does not implement an empty PATH entry as the current directory.

- [ ] **Step 3: Add the native fast path**

At the top of `_selfishell_command_path`, add:

```zsh
if [[ -z "${WSL_DISTRO_NAME:-}" ]]; then
  builtin whence -p -- "$command_name"
  return
fi
```

Keep the existing filtered loop unchanged for WSL.

- [ ] **Step 4: Verify native and WSL behavior**

Add a WSL case with an inherited `/mnt/c/...` entry before a temporary Linux executable and assert the Linux path is returned. Run `bash tests/common_zsh_test.bash`.

Expected: PASS for both the native empty-entry case and WSL filtered lookup.

- [ ] **Step 5: Commit command lookup optimization**

```sh
git add tests/common_zsh_test.bash common/common.zsh
git commit -m "perf: use native Zsh command lookup"
```

### Task 5: Close macOS benchmark isolation gaps

**Files:**
- Modify: `tests/benchmark_test.bash`
- Modify: `scripts/benchmark.sh`
- Modify: `docs/PERFORMANCE.md`

**Interfaces:**
- Consumes: Task 1's `INTERACTIVE_PATH` and `SELFISHELL_BENCHMARK_ZPROF_FILE`.
- Produces: base runs where reported-absent integrations are not executed through macOS Homebrew discovery.

- [ ] **Step 1: Strengthen the existing base isolation test**

Run base mode with a zprof output file and assert both the integration summary reports optional integrations absent and the profile contains neither a Starship invocation/error nor a real-home mise path:

```bash
! grep -qi 'starship' "$profile_file" ||
  fail "Base profiler executed ambient Starship"
! grep -Fq "$REAL_HOME/.config/mise" "$profile_file" ||
  fail "Benchmark profiler read the developer mise configuration"
```

- [ ] **Step 2: Run the benchmark suite and verify RED on macOS**

Run `bash tests/benchmark_test.bash`.

Expected on macOS with Homebrew Starship installed: FAIL because `mac/.zshrc` finds `/opt/homebrew/bin/brew`, evaluates `brew shellenv`, and reintroduces ambient integrations.

- [ ] **Step 3: Add the smallest benchmark-only Homebrew barrier**

For base mode on Darwin, create an executable no-op `brew` shim inside `$TEST_HOME/.local/bin`. The fixed `INTERACTIVE_PATH` already places that directory before system Homebrew, so `mac/.zshrc` sees `brew` and skips host `brew shellenv` without changing product configuration.

Set `TERM=xterm-256color`, `MISE_SHELL=`, and `MISE_GLOBAL_CONFIG_FILE=$TEST_HOME/.config/mise/config.toml` for timed and profiled interactive shells. Create the empty isolated mise config before measurement and run each shell with its working directory set to `$TEST_HOME`.

- [ ] **Step 4: Verify benchmark tests and local base smoke**

Run:

```sh
bash tests/benchmark_test.bash
SELFISHELL_BENCHMARK_ITERATIONS=3 \
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-base.zprof \
bash scripts/benchmark.sh --mode base
```

Expected: base integrations are absent, no Starship/mise leakage appears in the profile, and all existing metric labels remain stable.

- [ ] **Step 5: Document the additional isolation guarantees and commit**

Update `docs/PERFORMANCE.md` to name the fixed PATH, isolated working directory/mise config, and benchmark-only macOS Homebrew barrier.

```sh
git add tests/benchmark_test.bash scripts/benchmark.sh docs/PERFORMANCE.md
git commit -m "test: isolate macOS shell benchmarks"
```

### Task 6: Verify behavior and measure the result

**Files:**
- Verify all changed files.

**Interfaces:**
- Consumes: Tasks 1-5.
- Produces: fresh local test evidence, benchmark results, and a clean reviewable branch.

- [ ] **Step 1: Run focused suites**

```sh
bash tests/installers_test.bash
bash tests/proxy_test.bash
bash tests/common_zsh_test.bash
bash tests/benchmark_test.bash
bash tests/github_actions_pins_test.bash
bash tests/docs_test.bash
```

Expected: all pass.

- [ ] **Step 2: Run syntax, formatting, and diff checks**

```sh
git diff --check main...HEAD
bash -n lib/installers.sh scripts/benchmark.sh
zsh -n common/common.zsh common/completion.zsh common/interactive.zsh
```

Expected: all commands exit 0 with no diagnostics.

- [ ] **Step 3: Run the repository gate**

Run `bash scripts/check.sh`.

Expected: syntax, ShellCheck, formatting, and every test suite pass.

- [ ] **Step 4: Run isolated performance measurements**

```sh
SELFISHELL_BENCHMARK_ITERATIONS=50 bash scripts/benchmark.sh --mode base
SELFISHELL_BENCHMARK_ITERATIONS=30 \
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-full.zprof \
bash scripts/benchmark.sh --mode full
```

Expected: base mode is deterministic; full mode prepares plugins before timed warm startup and produces a profiler report. Record p50/p95 without claiming a hard pass threshold.

- [ ] **Step 5: Review scope and branch cleanliness**

```sh
git status --short
git diff --stat main...HEAD
git diff main...HEAD -- common/nvim/lua/plugins/ui.lua img/nvim.png img/selfishell.png
```

Expected: status is clean; only intended startup/benchmark/CI/docs files differ; the Neovim tree and screenshot diff is empty.
