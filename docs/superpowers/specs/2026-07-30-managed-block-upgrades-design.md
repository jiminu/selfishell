# Managed Block Upgrade Design

## Goal

Upgrade intact Selfishell blocks in place and handle user-edited blocks like
modified managed files, without requiring uninstall/reinstall.

## Behavior

- If the live block matches the current definition, keep it unchanged.
- If it matches the recorded checksum but not the current definition, treat it
  as an untouched old block and replace it automatically.
- If it matches neither checksum, treat it as user-modified. Interactive runs
  ask before replacement; yes backs up the full target file and replaces only
  the bounded block, while no skips that block. `--yes` and non-interactive
  runs preserve it and fail.
- Preserve all bytes outside the bounded block. Malformed or duplicate markers,
  changed path types, and symlinks remain hard failures.
- Dry-run reports the action and creates no state, backup, or temporary file.

## Implementation

Use one shared block preflight for `.zshrc`, `.zprofile`, and Ghostty. It records
overwrite or skip decisions only for the current process so prompts occur before
package changes and are not repeated during installation.

Replace a block with one temporary copy containing prefix, current block, and
suffix, then atomically rename it. Before replacement, write pending state with
the live checksum; after success, write active state with the new checksum. A
retry can therefore distinguish the old and new complete files after an
interruption. User-approved overwrites also copy the full target into the normal
conflict-backup directory.

## Verification

Cover untouched old-block migration, interactive overwrite and skip,
non-interactive preservation, backup creation, exact preservation outside the
block, dry-run, malformed paths, and write/copy/rename failure recovery. Run the
managed lifecycle suite and `bash scripts/check.sh`.
