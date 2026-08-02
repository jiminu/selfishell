# Quality Closeout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Selfishell's performance evidence, CI tool pins, public screenshots, and platform-verification claims trustworthy without changing product startup behavior.

**Architecture:** Keep the existing benchmark and CI jobs, adding only an isolated benchmark `PATH`, an opt-in Zsh profiler output, and one derived mise selector shared by the existing Neovim steps. Extend focused tests in place, update the existing performance and installation guides, and replace the two README image assets without adding screenshot automation.

**Tech Stack:** Bash 3.2-compatible shell, Zsh `zsh/zprof`, GitHub Actions YAML, existing Bash test helper, Markdown, PNG screenshots.

## Global Constraints

- Keep `install.sh`, CLI entrypoints, and shared libraries compatible with macOS Bash 3.2.
- Keep base benchmarking network-free and isolated from the real `HOME`.
- Do not change ordinary shell startup, add a runtime dependency, or make performance budgets blocking.
- Keep `common/mise.toml` as the mise-managed developer tool version source of truth and `profiles/developer.conf` as the installation declaration.
- Keep support limited to macOS on Intel and Apple Silicon, native Ubuntu on AMD64 and ARM64, and Ubuntu on WSL.
- Do not add platform runners, screenshot automation, or image pixel-regression tests.
- Use the canonical `selfishell` command and do not introduce `sf`.

---

### Task 1: Deterministic benchmark inputs and opt-in Zsh profile

**Files:**
- Modify: `tests/benchmark_test.bash`
- Modify: `scripts/benchmark.sh`
- Modify: `docs/PERFORMANCE.md`

**Interfaces:**
- Consumes: existing `SELFISHELL_BENCHMARK_PROFILE`, `SELFISHELL_BENCHMARK_RESULTS_FILE`, and benchmark modes.
- Produces: `SELFISHELL_BENCHMARK_ZPROF_FILE`, an optional path receiving one warm interactive-startup `zprof` report; `INTERACTIVE_PATH`, the mode-specific executable search path used by both measurement and reporting.

- [ ] **Step 1: Add a failing ambient-PATH isolation test**

Add a test to `tests/benchmark_test.bash` that creates executable fake
`starship`, `fzf`, and `zoxide` commands in a temporary directory, prepends it
to the caller's `PATH`, runs base mode with one iteration, and requires the
integration summary to report all three as absent:

```bash
test_benchmark_base_mode_ignores_ambient_integrations() {
  local fake_bin output tool

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  for tool in starship fzf zoxide; do
    cat >"$fake_bin/$tool" <<'EOF'
#!/bin/sh
printf ':\n'
EOF
    chmod +x "$fake_bin/$tool"
  done

  output="$(PATH="$fake_bin:$PATH" SELFISHELL_BENCHMARK_ITERATIONS=1 \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base)"

  [[ "$output" == *'starship=absent fzf=absent zoxide=absent zinit=absent'* ]] ||
    fail "Base mode inherited optional integrations from PATH: $output"
  teardown_test_home
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```sh
bash tests/benchmark_test.bash
```

Expected: the new test fails because current base-mode interactive startup and
the integration summary inherit the caller's `PATH`.

- [ ] **Step 3: Add a failing profiler-output test**

Add a network-free base-mode test using the same profiling mechanism that CI
will activate for full mode:

```bash
test_benchmark_writes_opt_in_zprof_report() {
  local profile_file

  setup_test_home
  profile_file="$TEST_ROOT/startup.zprof"

  SELFISHELL_BENCHMARK_ITERATIONS=1 \
    SELFISHELL_BENCHMARK_ZPROF_FILE="$profile_file" \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base >/dev/null

  [[ -s "$profile_file" ]] || fail "Benchmark did not write the requested zprof report"
  grep -Fq 'num  calls' "$profile_file" || fail "Benchmark output is not a zprof report"
  teardown_test_home
}
```

- [ ] **Step 4: Run the focused test and verify RED again**

Run `bash tests/benchmark_test.bash`.

Expected: the new profiling test fails because
`SELFISHELL_BENCHMARK_ZPROF_FILE` is not implemented.

- [ ] **Step 5: Use one mode-specific PATH everywhere inside the interactive benchmark**

In `scripts/benchmark.sh`, derive the path after full-mode provisioning:

```bash
case "$PROFILE_MODE" in
  base) INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin:/usr/bin:/bin" ;;
  full) INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin:$PATH" ;;
esac
```

Use `PATH="$INTERACTIVE_PATH"` in `run_interactive_zsh` and
`describe_integrations`, and export `INTERACTIVE_PATH` for benchmark child Bash
processes. Keep common-mode measurement on its existing minimal path. Base mode
must now report optional integrations as absent even if the caller installed
them; full mode must retain the explicitly provisioned mise/Starship and CI's
apt-provided fzf/zoxide.

- [ ] **Step 6: Implement an isolated one-shot zprof diagnostic**

Read the optional path near the existing benchmark environment variables:

```bash
ZPROF_FILE="${SELFISHELL_BENCHMARK_ZPROF_FILE:-}"
```

After all timed measurements and budget checks, call a new helper only when the
path is non-empty. The helper must create a temporary benchmark-home `.zshenv`
containing `zmodload zsh/zprof`, start one interactive shell with the same
`HOME`, `ZDOTDIR`, XDG paths, and `INTERACTIVE_PATH`, run `zprof`, redirect the
report to `ZPROF_FILE`, and remove the temporary `.zshenv`:

```bash
profile_interactive_zsh() {
  printf 'zmodload zsh/zprof\n' >"$TEST_HOME/.zshenv"
  HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
    XDG_CACHE_HOME="$TEST_HOME/.cache" PATH="$INTERACTIVE_PATH" \
    /bin/zsh -d -i -c 'zprof' >"$ZPROF_FILE" 2>&1
  rm -f "$TEST_HOME/.zshenv"
}
```

Do not load `zprof` during timed iterations; the report is diagnostic evidence,
not another timing metric.

- [ ] **Step 7: Run focused benchmark tests and smoke checks**

Run:

```sh
bash tests/benchmark_test.bash
SELFISHELL_BENCHMARK_ITERATIONS=1 bash scripts/benchmark.sh --mode base
```

Expected: all tests pass; base output contains the existing five metric labels
and reports optional integrations as absent.

- [ ] **Step 8: Document deterministic base mode and profiling**

Update `docs/PERFORMANCE.md` to state that base mode uses a fixed executable
path and never discovers optional integrations from the caller's environment.
Document:

```sh
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-startup.zprof \
  bash scripts/benchmark.sh --mode full
```

Explain that the normal metrics are collected before profiling, the profiler
adds no product startup code, and the report ranks initialization functions for
diagnosis rather than enforcing a budget.

- [ ] **Step 9: Commit Task 1**

```sh
git add tests/benchmark_test.bash scripts/benchmark.sh docs/PERFORMANCE.md
git commit -m "test: make shell performance evidence deterministic"
```

### Task 2: CI-derived mise selectors and profiling artifact

**Files:**
- Modify: `tests/github_actions_pins_test.bash`
- Modify: `.github/workflows/ci.yml`
- Modify: `tests/docs_test.bash`
- Modify: `docs/PERFORMANCE.md`

**Interfaces:**
- Consumes: `[tools]` entries `neovim`, `tree-sitter`, and `node` in `common/mise.toml`; `SELFISHELL_BENCHMARK_ZPROF_FILE` from Task 1.
- Produces: `MISE_E2E_TOOLS` in GitHub Actions through `GITHUB_ENV`; artifact `shell-performance-full-profile` containing both the TSV benchmark and `selfishell-startup.zprof`.

- [ ] **Step 1: Add failing workflow assertions**

Extend `tests/github_actions_pins_test.bash` so the extracted
`neovim-developer-e2e` job must:

- mention `common/mise.toml` and `$GITHUB_ENV`;
- define and consume `MISE_E2E_TOOLS` in all three mise commands;
- contain none of the `neovim`, `tree-sitter`, or `node` version literals read
  from `common/mise.toml`.

Add a second test extracting `shell-full-profile-benchmark` and requiring both
`SELFISHELL_BENCHMARK_ZPROF_FILE` and `selfishell-startup.zprof` in its uploaded
artifact paths.

- [ ] **Step 2: Add a failing performance-doc assertion**

Extend `test_performance_docs_document_full_benchmark_mode` in
`tests/docs_test.bash`:

```bash
grep -Fq 'SELFISHELL_BENCHMARK_ZPROF_FILE' "$ROOT_DIR/docs/PERFORMANCE.md" ||
  fail "docs/PERFORMANCE.md does not document opt-in zprof output"
grep -Fq 'selfishell-startup.zprof' "$ROOT_DIR/docs/PERFORMANCE.md" ||
  fail "docs/PERFORMANCE.md does not name the CI zprof artifact"
```

- [ ] **Step 3: Run both focused suites and verify RED**

Run:

```sh
bash tests/github_actions_pins_test.bash
bash tests/docs_test.bash
```

Expected: workflow assertions fail because the workflow repeats three version
literals and does not upload a zprof report; the docs assertion fails until the
CI artifact name is documented.

- [ ] **Step 4: Derive one mise selector list from common/mise.toml**

Insert a `Read pinned Neovim toolchain` step after installing mise. Use a small
Awk function scoped to the existing `[tools]` table:

```bash
mise_version() {
  awk -v requested="$1" '
    /^\[/ { in_tools = ($0 == "[tools]"); next }
    in_tools && $1 == requested {
      gsub(/[[:space:]\"]/, "", $3)
      print $3
      exit
    }
  ' common/mise.toml
}

neovim_version="$(mise_version neovim)"
tree_sitter_version="$(mise_version tree-sitter)"
node_version="$(mise_version node)"
[[ -n "$neovim_version" && -n "$tree_sitter_version" && -n "$node_version" ]]
printf 'MISE_E2E_TOOLS=neovim@%s tree-sitter@%s node@%s\n' \
  "$neovim_version" "$tree_sitter_version" "$node_version" >>"$GITHUB_ENV"
```

Replace all three hard-coded argument lists with unquoted
`$MISE_E2E_TOOLS` expansion. The value comes only from the reviewed repository
file and must remain a whitespace-separated mise selector list.

- [ ] **Step 5: Activate and upload the full-profile diagnostic**

Set the full benchmark step's diagnostic path:

```yaml
SELFISHELL_BENCHMARK_ZPROF_FILE: ${{ runner.temp }}/selfishell-startup.zprof
```

Change the existing upload step's `path` to a YAML block containing both the
TSV and zprof paths. Keep the existing artifact name and warn-only policy.

- [ ] **Step 6: Finish the performance artifact documentation**

Update `docs/PERFORMANCE.md` to say the Ubuntu full-profile artifact contains
`selfishell-benchmark-full.tsv` and `selfishell-startup.zprof`, while base
artifacts remain TSV-only.

- [ ] **Step 7: Run focused workflow and docs suites**

Run:

```sh
bash tests/github_actions_pins_test.bash
bash tests/docs_test.bash
```

Expected: both pass, and `rg` finds the three tool version literals in
`common/mise.toml` but not inside the `neovim-developer-e2e` job.

- [ ] **Step 8: Commit Task 2**

```sh
git add tests/github_actions_pins_test.bash .github/workflows/ci.yml tests/docs_test.bash docs/PERFORMANCE.md
git commit -m "ci: derive Neovim toolchain from mise config"
```

### Task 3: Document the actual platform verification boundary

**Files:**
- Modify: `docs/INSTALLATION.md`

**Interfaces:**
- Consumes: supported platform statement in `README.md` and jobs in `.github/workflows/ci.yml`.
- Produces: a `Verification coverage` section that distinguishes product support from current automated evidence.

- [ ] **Step 1: Add the verification statement**

After the opening supported-platform paragraph in `docs/INSTALLATION.md`, add a
short `## Verification coverage` section naming:

- Ubuntu 24.04 container installation lifecycle;
- GitHub-hosted macOS configuration lifecycle;
- Ubuntu-hosted pinned Neovim lifecycle;
- Ubuntu/macOS shell checks and base benchmarks;
- Ubuntu full-profile benchmark.

State explicitly that WSL and every advertised architecture are not separate CI
runners. Preserve the current supported-platform list and avoid implying that
unlisted Linux distributions are supported.

- [ ] **Step 2: Cross-check wording against CI**

Run:

```sh
rg -n 'runs-on:|container:|image:|benchmark|e2e' .github/workflows/ci.yml
sed -n '1,80p' docs/INSTALLATION.md
```

Expected: each documented automated environment maps to an existing workflow
job, and the limitation sentence does not remove a supported platform.

- [ ] **Step 3: Run the docs suite**

Run `bash tests/docs_test.bash`.

Expected: PASS. No new exact-copy regression test is added for prose.

- [ ] **Step 4: Commit Task 3**

```sh
git add docs/INSTALLATION.md
git commit -m "docs: state platform verification coverage"
```

### Task 4: Refresh current shell and Neovim screenshots

**Files:**
- Modify: `img/selfishell.png`
- Modify: `img/nvim.png`

**Interfaces:**
- Consumes: current managed Ghostty, Starship, and Neovim configuration; existing README image paths and alt text.
- Produces: sanitized PNG captures at the same two paths, with no README structural change.

- [ ] **Step 1: Prepare disposable visible content**

Use a temporary directory outside the repository for the shell capture so no
personal project names, branches, or command history appear. Initialize a small
Git repository with a neutral `demo` branch and harmless files. For Neovim,
open the Selfishell source or a disposable fixture containing public shell code;
ensure no credentials, internal URLs, environment values, or personal home path
are visible.

- [ ] **Step 2: Capture the current shell UI**

Open Ghostty with the current managed developer configuration, size the window
to a compact README-friendly aspect ratio, show the prompt before and after a
simple `ls`, and capture only the application window to a temporary PNG. Crop no
prompt content and keep the title neutral.

- [ ] **Step 3: Capture the current Neovim UI**

In a second clean Ghostty window, open Neovim with the current Selfishell config.
Show the explorer, buffer line, current syntax palette, and compact statusline.
Capture only the application window to a temporary PNG.

- [ ] **Step 4: Replace assets and inspect them**

Move the sanitized captures to `img/selfishell.png` and `img/nvim.png`. Verify:

```sh
file img/selfishell.png img/nvim.png
sips -g pixelWidth -g pixelHeight img/selfishell.png img/nvim.png
```

Open both at natural size and at approximate README display size against light
and dark backgrounds. Confirm text remains legible, no sensitive data appears,
and the existing README alt text still describes each image accurately.

- [ ] **Step 5: Commit Task 4**

```sh
git add img/selfishell.png img/nvim.png
git commit -m "docs: refresh current shell and Neovim screenshots"
```

### Task 5: Repository verification and final review

**Files:**
- Verify all files changed in Tasks 1-4.

**Interfaces:**
- Consumes: all prior task deliverables.
- Produces: local test evidence and a clean, reviewable diff; GitHub CI remains unclaimed until it runs remotely.

- [ ] **Step 1: Run focused suites once more**

```sh
bash tests/benchmark_test.bash
bash tests/github_actions_pins_test.bash
bash tests/docs_test.bash
```

Expected: all pass.

- [ ] **Step 2: Run formatting and diff checks**

```sh
git diff --check
git status --short
```

Expected: no whitespace errors and only intended files changed.

- [ ] **Step 3: Run the repository gate**

```sh
bash scripts/check.sh
```

Expected: Bash/Zsh syntax, ShellCheck, formatting, and every test suite pass.

- [ ] **Step 4: Re-run the base benchmark smoke check**

```sh
SELFISHELL_BENCHMARK_ITERATIONS=3 \
SELFISHELL_BENCHMARK_ZPROF_FILE=/tmp/selfishell-startup.zprof \
bash scripts/benchmark.sh --mode base
```

Expected: all metrics print, integrations are absent in base mode, and the
profile file is non-empty. Do not claim full-profile or changed GitHub Actions
execution from this local check.

- [ ] **Step 5: Review the final diff against the spec**

Confirm there are no product startup changes, hard performance gates, profile
package changes, duplicated tool versions in the Neovim job, new CI platforms,
or screenshot automation. Record visual inspection separately from automated
test results.
