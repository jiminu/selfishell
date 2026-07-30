# Test Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every declared test execute in a fail-safe isolated process, clean up its temporary home, report skip correctly, and run Neovim configuration tests under the pinned CI toolchain.

**Architecture:** Add a small shared runner to `tests/test_helper.bash`. Each test runs as a background child so Bash `errexit` remains active; the parent waits and interprets normal, assertion-failure, and skip statuses. Existing suite-level and managed-install parallelism remain unchanged.

**Tech Stack:** macOS Bash 3.2-compatible shell, Zsh, GitHub Actions, existing test harness.

## Global Constraints

- Do not change Selfishell product behavior.
- Do not add dependencies.
- Preserve `SELFISHELL_SUITE_JOBS=4` and `SELFISHELL_TEST_JOBS=4` defaults.
- Preserve the managed-install suite's parallel execution.
- Tests must use temporary `HOME` paths and clean them after success, failure, or skip.
- Use exit status `77` for skip and `99` for an assertion already reported by `fail`.
- Keep the large-file split, embedded-Lua extraction, and runtime optimization out of this slice.

---

### Task 1: Add a fail-safe discovered-test runner

**Files:**
- Create: `tests/test_helper_test.bash`
- Modify: `tests/test_helper.bash`

**Interfaces:**
- Produces: `skip REASON`, `run_test_isolated TEST [SETUP] [TEARDOWN]`, and `run_discovered_tests [SETUP] [TEARDOWN]`.
- `skip` prints `SKIP: REASON` and exits `77`.
- `fail` keeps printing its assertion and exits `99` so the parent does not print a duplicate failure.

- [ ] **Step 1: Write runner regression tests**

Create child-script tests that source `tests/test_helper.bash` and prove:

```bash
test_unexpected_failure_stops_without_pass() {
  false
  printf 'continued\n' >"$MARKER"
}

test_cleanup_runs_after_failure() {
  false
}

test_skip_is_not_reported_as_pass() {
  skip 'fixture unavailable'
}
```

The parent assertions must check the child exit status, absence of `continued`,
presence of the cleanup marker, and output containing `SKIP` without `PASS`.

- [ ] **Step 2: Run the new test and verify RED**

Run: `bash tests/test_helper_test.bash`

Expected: nonzero because `run_discovered_tests` is undefined.

- [ ] **Step 3: Implement the minimal shared runner**

The runner must start each `run_test_isolated` call in the background, then
`wait` for it. Do not call a test function from `if`, `!`, `&&`, or `||`.

`run_test_isolated` must:

```bash
set -Eeuo pipefail
trap "$teardown_name" EXIT   # when provided
"$setup_name"                # when provided
"$test_name"
trap - EXIT
"$teardown_name"             # when provided
printf 'PASS: %s\n' "$test_name"
```

The parent maps `0` to success, `77` to a successful skip, `99` to a reported
failure returned as suite status `1`, and any other nonzero status to
`FAIL: TEST (exit code N)` followed by suite status `1`.

- [ ] **Step 4: Run the helper regression test and verify GREEN**

Run: `bash tests/test_helper_test.bash`

Expected: all helper tests pass; the intentional child failures do not leak
temporary directories or false PASS output.

- [ ] **Step 5: Run static checks**

Run:

```bash
bash -n tests/test_helper.bash tests/test_helper_test.bash
shellcheck -x tests/test_helper.bash tests/test_helper_test.bash
shfmt -d -i 2 -ci tests/test_helper.bash tests/test_helper_test.bash
```

- [ ] **Step 6: Commit**

```bash
git add tests/test_helper.bash tests/test_helper_test.bash
git commit -m "test: add fail-safe discovered runner"
```

### Task 2: Route suites through the shared runner

**Files:**
- Modify: `tests/benchmark_test.bash`
- Modify: `tests/cli_test.bash`
- Modify: `tests/common_zsh_test.bash`
- Modify: `tests/dependency_release_test.bash`
- Modify: `tests/dependency_updates_test.bash`
- Modify: `tests/docs_test.bash`
- Modify: `tests/github_actions_pins_test.bash`
- Modify: `tests/installers_test.bash`
- Modify: `tests/lifecycle_e2e_test.bash`
- Modify: `tests/managed_install_test.bash`
- Modify: `tests/neovim_config_test.bash`
- Modify: `tests/package_adapters_test.bash`
- Modify: `tests/platform_test.bash`
- Modify: `tests/profiles_test.bash`
- Modify: `tests/proxy_test.bash`
- Modify: `tests/release_bootstrap_test.bash`
- Modify: `tests/release_verification_test.bash`
- Modify: `tests/tool_status_test.bash`
- Modify: `tests/updates_test.bash`
- Modify: `tests/version_test.bash`
- Modify: `tests/workflow_notifications_test.bash`

**Interfaces:**
- Consumes: Task 1's three shared helper functions and status conventions.
- Produces: automatic execution of every `test_*` function without manual call lists.

- [ ] **Step 1: Migrate the four false-pass sequential runners**

Replace the local `run_test`/`main` implementations in benchmark, CLI,
dependency-release, and platform suites with direct calls to
`run_discovered_tests`. Pass existing setup and teardown hooks where present.

- [ ] **Step 2: Verify the four focused suites**

Run:

```bash
bash tests/benchmark_test.bash
bash tests/cli_test.bash
bash tests/dependency_release_test.bash
bash tests/platform_test.bash
```

Expected: every declared test passes and no suite continues after an unexpected
failure in the helper regression fixture.

- [ ] **Step 3: Fix the managed-install parallel runner**

Delete its local `run_test`. Keep the existing batching and log ordering, but
launch this shared call in each background slot:

```bash
run_test_isolated "$test_name" setup_managed_home teardown_managed_home
```

- [ ] **Step 4: Verify managed-install behavior**

Run: `bash tests/managed_install_test.bash`

Expected: 78 tests pass with four jobs and no temporary home remains from the
suite.

- [ ] **Step 5: Replace every manual test call list**

Use `run_discovered_tests` in the remaining sequential suites. Convert
suite-wide setup into named hooks where tests need a fresh HOME. Tests that
already create their own isolated fixture may use the runner without hooks.

For Neovim tests, add `setup_neovim_test` and `teardown_neovim_test` hooks that
create and remove the existing XDG directories per test. Replace each manual
Neovim-unavailable branch with:

```bash
command -v nvim >/dev/null 2>&1 || skip 'Neovim unavailable'
```

- [ ] **Step 6: Remove the redundant omitted test**

Delete `test_minimal_profile_installs_vimrc` from `tests/installers_test.bash`.
Do not replace it: `test_install_copies_configuration_and_tracks_resources`
already compares the managed Vimrc and asserts the user Vimrc link.

- [ ] **Step 7: Remove per-test EXIT-trap leaks**

For dependency-release and dependency-update suites, remove each internal
`setup_test_home` and `trap teardown_test_home EXIT`. Invoke:

```bash
run_discovered_tests setup_test_home teardown_test_home
```

at the file end instead.

- [ ] **Step 8: Verify every migrated suite**

Run: `bash tests/run.bash`

Expected: all 23 suites pass, including `test_helper_test.bash`; Neovim tests
either pass or report only SKIP, never both SKIP and PASS.

- [ ] **Step 9: Verify no explicit list can omit a test**

Run:

```bash
rg -n "^printf 'PASS: test_|if ! run_test|\"\$test_name\" \|\| rc=" tests
```

Expected: no matches.

- [ ] **Step 10: Verify dependency suites leak no temporary homes**

Count `selfishell-test.*` under `${TMPDIR:-/tmp}` before and after running the
two dependency suites. Expected delta: `0`.

- [ ] **Step 11: Commit**

```bash
git add tests
git commit -m "test: isolate and discover shell tests"
```

### Task 3: Require Neovim configuration tests in pinned CI

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the existing pinned mise environment containing Neovim 0.12.4,
  Tree-sitter CLI 0.26.11, and Node.js 24.18.0.
- Produces: a dedicated CI step running `tests/neovim_config_test.bash` before
  `scripts/neovim-e2e.sh`.

- [ ] **Step 1: Write a failing workflow assertion**

Add a test to `tests/github_actions_pins_test.bash` requiring the
`neovim-developer-e2e` job to invoke `bash tests/neovim_config_test.bash` through
the existing pinned mise toolchain.

- [ ] **Step 2: Run it and verify RED**

Run: `bash tests/github_actions_pins_test.bash`

Expected: failure because the workflow invokes only `scripts/neovim-e2e.sh`.

- [ ] **Step 3: Add the workflow step**

Add a separate step after toolchain installation:

```yaml
- name: Run Neovim configuration tests
  run: ~/.local/bin/mise exec neovim@0.12.4 tree-sitter@0.26.11 node@24.18.0 -- bash tests/neovim_config_test.bash
```

- [ ] **Step 4: Verify GREEN and syntax-adjacent checks**

Run:

```bash
bash tests/github_actions_pins_test.bash
bash tests/ci_change_classification_test.bash
```

Expected: both suites pass.

- [ ] **Step 5: Run the Neovim test with the local pinned binary**

Run:

```bash
PATH="$HOME/.local/share/mise/installs/neovim/0.12.4/bin:$PATH" bash tests/neovim_config_test.bash
```

Expected: every Neovim configuration test passes with no SKIP.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml tests/github_actions_pins_test.bash
git commit -m "ci: require pinned Neovim configuration tests"
```

### Task 4: Full verification

**Files:**
- Verify only

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: evidence that the test-only refactor preserves repository behavior.

- [ ] **Step 1: Inspect all changes**

Run:

```bash
git diff --check
git status --short
git diff --stat main...HEAD
```

- [ ] **Step 2: Run the repository gate**

Run:

```bash
PATH="$HOME/.local/share/mise/installs/neovim/0.12.4/bin:$PATH" bash scripts/check.sh
```

Expected: Bash/Zsh syntax, ShellCheck, shfmt, version consistency, and all test
suites pass.

- [ ] **Step 3: Confirm worktree cleanliness and commit history**

Run:

```bash
git status --short
git log --oneline main..HEAD
```

Expected: clean worktree and three focused implementation commits after the
design and plan documents.
