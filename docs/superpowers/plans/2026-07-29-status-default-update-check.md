# Default `status` Update Check Implementation Plan

**Goal:** Make `selfishell status` check the latest CLI release by default,
degrade cleanly when offline, and remove `--check-updates` from the product and
its documentation.

**Architecture:** Keep release discovery in `release_latest_version`. The status
command starts with `Available: unavailable`, calls that helper only when
`SELFISHELL_OFFLINE` is not `1`, and treats lookup failure as non-fatal.

**Tech Stack:** Bash 3.2-compatible shell, repository Bash test harness, Markdown.

---

### Task 1: Lock behavior with failing tests

**Files:**
- Modify: `tests/release_bootstrap_test.bash`

1. Change the prerelease test to invoke plain `status`.
2. Add coverage for a successful default lookup.
3. Add coverage for failed lookup returning `Available: unavailable` without a
   release lookup error.
4. Add offline coverage proving `SELFISHELL_OFFLINE=1` reports `unavailable`.
5. Add coverage proving `--check-updates` is rejected as an unknown option.
6. Run the focused suite and confirm the new expectations fail against the old
   implementation.

### Task 2: Implement the minimal command change

**Files:**
- Modify: `lib/commands/status.sh`

1. Remove the `check_updates` option and help entry.
2. Initialize the available version as `unavailable`.
3. Unless offline mode is active, attempt `release_latest_version`; retain the
   fallback silently when discovery fails.
4. Run the focused suite and confirm it passes.

### Task 3: Update every related Markdown document

**Files:**
- Modify: `README.md`
- Modify: `docs/UPDATES.md`
- Modify: `docs/TROUBLESHOOTING.md`
- Modify: `docs/superpowers/plans/2026-07-26-readme-restructure.md`

1. Remove `status --check-updates` as a separate command.
2. Explain that plain `status` checks the available CLI release and degrades to
   `unavailable` offline or on lookup failure.
3. Update the prior README plan's command inventory so it no longer prescribes
   the removed option.
4. Search all Markdown files for stale command examples or old output.

### Task 4: Verify and commit

1. Run focused release/bootstrap tests.
2. Run `git diff --check` and inspect the complete diff.
3. Run `bash scripts/check.sh` with the trusted installed Neovim binary first in
   `PATH` if the isolated worktree's mise trust boundary blocks the shim.
4. Commit the tested implementation and documentation as one reviewable change.
