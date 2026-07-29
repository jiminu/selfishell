# Mise Login-Shell Resolution Design

## Problem

Selfishell installs its pinned `mise` binary at `~/.local/bin/mise`, but the
managed `.zprofile` block currently activates shims only when `mise` is already
on `PATH`. Login environments such as Dock-launched IDEs may evaluate
`.zprofile` before Selfishell's interactive configuration prepends
`~/.local/bin`, so shim activation is skipped.

## Design

The managed `.zprofile` block will use executable
`$HOME/.local/bin/mise` first. This matches Selfishell's direct-download target
and the interactive shell's preference for `~/.local/bin`. When that executable
does not exist, the block will retain the existing `command -v mise` fallback
for Homebrew, Apt, and other user-managed installations.

The block will not add another general `PATH` mutation. Its only responsibility
remains activating mise shims for login environments.

## Verification

Lifecycle coverage will prove that a Selfishell-installed mise outside `PATH`
receives `activate zsh --shims`. Existing PATH-based coverage will continue to
prove fallback behavior. The full repository gate will run after the focused
test passes.
