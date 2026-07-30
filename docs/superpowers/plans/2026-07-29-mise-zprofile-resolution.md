# Mise Login-Shell Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate mise shims in login environments when Selfishell's pinned `~/.local/bin/mise` exists outside the inherited `PATH`.

**Architecture:** Keep the managed `.zprofile` block focused on shim activation. Prefer Selfishell's pinned executable, then preserve PATH-based discovery as fallback for externally managed mise installations.

**Tech Stack:** macOS Bash 3.2-compatible shell implementation, Zsh login profile, repository Bash lifecycle tests.

## Global Constraints

- Preserve existing user `.zprofile` bytes outside the bounded Selfishell block.
- Keep installation and managed shell logic compatible with supported macOS and Ubuntu/WSL environments.
- Add no general `PATH` mutation to `.zprofile`.
- Run `bash scripts/check.sh` because this changes shell lifecycle behavior.

---

### Task 1: Resolve Selfishell Mise Before PATH Fallback

**Files:**
- Modify: `tests/managed_install_test.bash`
- Modify: `lib/managed.sh`

**Interfaces:**
- Consumes: `managed_block_definition user-zprofile`, `$HOME/.local/bin/mise`, and any `mise` found by `command -v`.
- Produces: a managed `.zprofile` block that calls the selected executable with `activate zsh --shims`.

- [x] **Step 1: Write the failing lifecycle test**

Add a test that installs managed configuration, places a real executable fixture at `$HOME/.local/bin/mise`, excludes that directory from `PATH`, sources `.zprofile` with `/bin/zsh`, and asserts the emitted environment plus exact activation arguments:

```bash
test_mise_shims_zprofile_uses_selfishell_mise_outside_path() {
  local output

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null
  mkdir -p "$HOME/.local/bin"
  cat >"$HOME/.local/bin/mise" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$HOME/mise-local-args"
printf '%s\n' 'export SELFISHELL_LOCAL_MISE_SHIMS_TEST=loaded'
EOF
  chmod +x "$HOME/.local/bin/mise"

  output="$(PATH="/usr/bin:/bin" /bin/zsh -dfc 'source "$HOME/.zprofile"; print "${SELFISHELL_LOCAL_MISE_SHIMS_TEST-}"')"

  [[ "$output" == loaded ]] || fail "The .zprofile block did not activate Selfishell mise outside PATH"
  assert_file_content 'activate zsh --shims' "$HOME/mise-local-args"
}
```

This test catches removal or reversal of the Selfishell-path branch. Existing lifecycle coverage continues to catch loss of the PATH fallback.

- [x] **Step 2: Run the lifecycle test file and verify RED**

Run:

```bash
SELFISHELL_TEST_JOBS=1 bash tests/managed_install_test.bash
```

Expected: FAIL only for `test_mise_shims_zprofile_uses_selfishell_mise_outside_path`, with `The .zprofile block did not activate Selfishell mise outside PATH`.

- [x] **Step 3: Implement the minimal managed block change**

Replace the `user-zprofile` body with:

```zsh
if [[ -x "$HOME/.local/bin/mise" ]]; then
  eval "$("$HOME/.local/bin/mise" activate zsh --shims)"
elif command -v mise >/dev/null 2>&1; then
  eval "$(command mise activate zsh --shims)"
fi
```

- [x] **Step 4: Verify GREEN and repository gate**

Run:

```bash
SELFISHELL_TEST_JOBS=1 bash tests/managed_install_test.bash
bash scripts/check.sh
```

Expected: both commands exit 0 with no failed checks.

- [x] **Step 5: Commit implementation**

```bash
git add lib/managed.sh tests/managed_install_test.bash docs/superpowers/plans/2026-07-29-mise-zprofile-resolution.md
git commit -m "fix: resolve managed mise for login shims"
```
