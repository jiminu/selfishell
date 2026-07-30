# Managed Block Upgrades Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade untouched old managed blocks automatically and give user-edited blocks the same interactive preserve/overwrite choice as managed files.

**Architecture:** Classify blocks from current, recorded, and expected checksums during preflight. Cache overwrite/skip decisions for the process, then atomically splice the selected block during installation while preserving all bytes outside its markers.

**Tech Stack:** macOS Bash 3.2-compatible shell, existing fixed-line resource state, isolated Bash lifecycle tests.

## Global Constraints

- Apply the behavior to `user-zshrc`, `user-zprofile`, and `user-ghostty`.
- Prompt before package or configuration changes and never prompt twice.
- `--yes` and non-interactive runs never overwrite user-modified blocks.
- Dry-run creates no state, backup, temporary file, or target change.
- Preserve malformed-marker, duplicate-marker, symlink, and path-type hard stops.
- Preserve every byte outside the bounded block.

---

### Task 1: Atomically Upgrade Untouched Old Blocks

**Files:**
- Modify: `lib/managed.sh`
- Test: `tests/managed_install_test.bash`

**Interfaces:**
- Produces: `managed_replace_block RESOURCE TARGET NEW_CHECKSUM [BACKUP]`.
- Consumes: `MANAGED_BLOCK_START`, `MANAGED_BLOCK_LENGTH`, recorded state, and `managed_block_content`.

- [ ] **Step 1: Write failing lifecycle coverage**

Add a test that installs `user-zprofile`, rewrites its bounded body to the
0.6.8 definition, records that old checksum as active state, and runs
`update --tools-only`. Assert success, the new `$HOME/.local/bin/mise` branch,
updated active checksum, and byte-exact prefix/suffix preservation.

- [ ] **Step 2: Verify RED**

Run:

```bash
SELFISHELL_TEST_JOBS=1 bash tests/managed_install_test.bash
```

Expected: the new migration test fails with the current legacy-state error.

- [ ] **Step 3: Add one-pass atomic block replacement**

Implement `managed_replace_block` to:

```text
inspect block -> optional full-file backup -> write pending state with live
checksum -> copy prefix + new block + suffix into a mode-preserving temp file
-> mv temp over target -> write active state with new checksum
```

On retry, live content equal to the new expected checksum refreshes active
state; live content equal to pending state resumes replacement.

- [ ] **Step 4: Route untouched old content to replacement**

In `managed_install_block`, replace the legacy error for:

```bash
[[ "$MANAGED_BLOCK_CHECKSUM" == "$MANAGED_STATE_CHECKSUM" ]]
```

with a dry-run message or `managed_replace_block`. Do not alter malformed or
path-conflict handling.

- [ ] **Step 5: Verify GREEN and commit**

Run the focused suite, `bash -n lib/managed.sh tests/managed_install_test.bash`,
ShellCheck, and shfmt. Commit:

```bash
git add lib/managed.sh tests/managed_install_test.bash
git commit -m "fix: upgrade untouched managed blocks"
```

### Task 2: Prompt Once for User-Modified Blocks

**Files:**
- Modify: `lib/managed.sh`
- Modify: `lib/commands/install.sh`
- Modify: `lib/commands/update.sh`
- Modify: `docs/TROUBLESHOOTING.md`
- Test: `tests/managed_install_test.bash`

**Interfaces:**
- Produces: `managed_preflight_block_target RESOURCE TARGET ASSUME_YES DRY_RUN` and process-local overwrite/skip decision lists.
- Changes: `managed_preflight_zsh_loader ASSUME_YES DRY_RUN`.

- [ ] **Step 1: Write failing conflict tests**

Cover these cases for a well-formed block whose live checksum differs from both
recorded and expected checksums:

```text
interactive yes -> full-file conflict backup + bounded replacement
interactive no  -> block skipped, later resources still updated
--yes           -> preserve and fail
dry-run         -> report decision requirement, create nothing
```

Also assert the prompt occurs before a fake package operation and only once.

- [ ] **Step 2: Verify RED**

Run the four new tests through `SELFISHELL_TEST_JOBS=1 bash tests/managed_install_test.bash`.
Expected: current code stops with `Cannot manage` or `Legacy` and offers no
overwrite choice.

- [ ] **Step 3: Add process-local decisions**

Store resource names in two whitespace-delimited globals because managed
resource names cannot contain whitespace:

```bash
MANAGED_BLOCK_OVERWRITE_RESOURCES=""
MANAGED_BLOCK_SKIP_RESOURCES=""
```

Preflight classifies the block. Interactive yes records overwrite; any other
answer records skip. `--yes` or non-interactive mode reports preservation and
returns an error. Dry-run records skip only for that process and reports the
future decision without writing files.

- [ ] **Step 4: Pass command policy into every block preflight**

Change install and update callers to pass `assume_yes` and `dry_run` into both
the Zsh loader and block-target preflights. Make the Zsh loader retain its
special legacy/symlink/untracked checks, then use the common conflict policy.

- [ ] **Step 5: Consume decisions during installation**

For skip, print `Skipped modified Selfishell block` and return success. For
overwrite, create a unique full-file backup under
`$SELFISHELL_STATE_DIR/backups/$resource`, then call `managed_replace_block`.
Do not record that conflict backup as the original restore backup.

- [ ] **Step 6: Document the behavior**

Add a short troubleshooting subsection explaining automatic untouched-block
upgrades, interactive modified-block choices, and non-interactive preservation.

- [ ] **Step 7: Verify and commit**

Run the focused lifecycle suite and static checks for all modified shell files.
Commit:

```bash
git add lib/managed.sh lib/commands/install.sh lib/commands/update.sh docs/TROUBLESHOOTING.md tests/managed_install_test.bash
git commit -m "fix: resolve managed block conflicts interactively"
```

### Task 3: Prove Failure Recovery and Repository Safety

**Files:**
- Modify: `tests/managed_install_test.bash`

**Interfaces:**
- Verifies: pending-state recovery and atomic target preservation.

- [ ] **Step 1: Add failure-injection coverage**

Use existing command shims to fail backup copy, temporary-file write, final
`mv`, and active-state write. Assert the original target or complete new target
remains, temporary files are removed, pending state is recoverable, and no
success message is printed after failure.

- [ ] **Step 2: Run focused and full gates**

Run:

```bash
SELFISHELL_TEST_JOBS=1 bash tests/managed_install_test.bash
bash scripts/check.sh
```

Expected: all managed lifecycle tests, including the new cases, pass; the
repository gate passes syntax, ShellCheck, formatting, version consistency, and
23 suites.

- [ ] **Step 3: Scan final scope and commit**

Run `git diff --check`, inspect `git diff --stat main...HEAD`, and ensure only
the design, plan, managed-block implementation, related tests, and
troubleshooting documentation changed. Commit any final test-only additions:

```bash
git add tests/managed_install_test.bash
git commit -m "test: cover managed block recovery"
```
