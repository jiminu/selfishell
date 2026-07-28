# mise Shims Zprofile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mise-managed tools available to login environments and IDEs through a safe, managed `~/.zprofile` shims block.

**Architecture:** Reuse the existing managed-block lifecycle. Declare `user-zprofile`, define its block, and preflight it before package/configuration changes; keep normal interactive activation in `.zshrc` unchanged.

**Tech Stack:** Bash 3.2-compatible lifecycle code, Zsh startup configuration, existing Bash test harness.

## Global Constraints

- Preserve all user bytes outside the bounded block.
- Reject symlinks, non-regular paths, malformed markers, and untracked blocks before other installation changes.
- Dry-run must write nothing; reinstall must not duplicate the block; uninstall removes only an intact block.
- No new option, dependency, or state format.

---

### Task 1: Managed mise shims block

**Files:**
- Modify: `lib/resources.sh`
- Modify: `lib/managed.sh`
- Modify: `lib/commands/install.sh`
- Modify: `lib/commands/update.sh`
- Test: `tests/managed_install_test.bash`
- Modify: `docs/INSTALLATION.md`

**Interfaces:**
- Consumes: existing `managed_install_block`, `managed_remove_block`, and resource iteration.
- Produces: `user-zprofile` block at `$HOME/.zprofile` running guarded `mise activate zsh --shims`.

- [x] Add lifecycle test: preserve existing `.zprofile`, install one block, invoke mise with `activate zsh --shims`, remain idempotent, and restore exact user bytes on uninstall.
- [x] Add preflight test: `.zprofile` symlink rejects install before config/state creation.
- [x] Run `SELFISHELL_TEST_JOBS=4 bash tests/managed_install_test.bash`; confirm new tests fail because no `.zprofile` resource exists.
- [x] Declare `block user-zprofile $HOME/.zprofile`, define guarded block content, and preflight it in install/update before package work.
- [x] Run `SELFISHELL_TEST_JOBS=4 bash tests/managed_install_test.bash`; confirm pass.
- [x] Document login/IDE shims plus retained interactive PATH activation.
- [x] Run `bash scripts/check.sh`; confirm exit 0.
