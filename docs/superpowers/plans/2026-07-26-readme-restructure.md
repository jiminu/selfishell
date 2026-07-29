# README Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the README into a concise, installation-first entry point for new users while preserving removed detail in focused documentation.

**Architecture:** Detailed operational guidance moves into existing topic documents, and contributor setup moves into a new root-level guide. The README becomes a 140–180-line navigation layer containing only product identity, quick installation, essential choices, everyday commands, safety promises, and links.

**Tech Stack:** GitHub-flavored Markdown, POSIX shell verification commands, repository validation through `bash scripts/check.sh`

## Global Constraints

- Use `selfishell` as the canonical command; mention `sfs` only as an optional convenience alias.
- Keep the supported platforms limited to macOS on Apple Silicon and Intel, native Ubuntu on AMD64 and ARM64, and Ubuntu on WSL.
- Keep `developer` as the default profile and `minimal` as the explicit lightweight profile.
- Preserve pinned developer tool versions from `common/mise.toml`: Neovim 0.12.4, Tree-sitter CLI 0.26.11, Node.js 24.18.0, Python 3.13.14, and uv 0.5.21.
- Keep the public installer URL under `raw.githubusercontent.com/jiminu/selfishell`; do not introduce `selfishell.dev`.
- Preserve the non-root, XDG-compatible install model, checksum verification, user-owned `~/.zshrc`, safe backups, and offline rollback claims.
- Do not change scripts, CLI behavior, profiles, dependencies, releases, or milestone status.
- Keep the README near 140–180 lines; clarity may justify a small deviation.

---

## File Structure

- Modify `docs/INSTALLATION.md`: own detailed bootstrap flags, exact-version and offline installation, uninstall, Ghostty customization, and platform notes.
- Modify `docs/PROFILES.md`: own detailed developer-profile Neovim workflow.
- Create `CONTRIBUTING.md`: own source-checkout development and verification commands.
- Modify `README.md`: provide the installation-first new-user overview and link to the detailed guides.

### Task 1: Move Detailed Guidance into Focused Documents

**Files:**
- Modify: `docs/INSTALLATION.md`
- Modify: `docs/PROFILES.md`
- Create: `CONTRIBUTING.md`

**Interfaces:**
- Consumes: current detailed sections in `README.md`, current behavior described by `install.sh`, `lib/commands/install.sh`, and `lib/commands/uninstall.sh`
- Produces: stable link targets for the shortened README: installation, uninstallation, Ghostty, profiles, Neovim, and contributing guidance

- [ ] **Step 1: Expand installation documentation with README-only detail**

Add clear subsections to `docs/INSTALLATION.md` without duplicating its existing introduction or legacy transition:

```markdown
## Bootstrap options

### Add the CLI directory to PATH
### Install CLI and default profile together
### Install an exact release
### Install configuration without network access

## Ghostty customization

## Uninstallation

### Restore configuration
### Restore configuration and purge Selfishell

## Platform notes
```

Under these headings, preserve the current README commands and guarantees:

- `--add-to-path` edits only the current shell's regular startup file and tracks an idempotent block.
- `--setup --yes` installs the CLI and default profile non-interactively.
- `--version <version>` uses an exact release.
- `SELFISHELL_OFFLINE=1` and `--skip-packages` perform configuration-only installation.
- Ghostty defaults live under `~/.config/selfishell/ghostty`, its entrypoint contains a managed block, and `~/.config/ghostty/user.ghostty` remains fully user-owned.
- `selfishell uninstall --restore --dry-run`, `selfishell uninstall --restore`, and `selfishell uninstall --restore --purge` remain documented with their distinct effects.
- WSL font selection, macOS Ghostty restart, and optional-package failure behavior remain documented.

- [ ] **Step 2: Add Neovim workflow detail to the profile guide**

Append `## Neovim workflow` to `docs/PROFILES.md`. Preserve these user-facing behaviors from the README:

- `Space` is the leader key and opens which-key after a pause.
- Lua, Python, Bash, and sh LSP support appears when a configured server attaches.
- Splits open right and below; substitutions preview before application.
- Bufferline uses `[b`, `]b`, and `Space b d` for navigation and closing.
- `vim` resolves to Neovim in `developer`; `vi` remains the system editor.

- [ ] **Step 3: Create the contributor entry point**

Create `CONTRIBUTING.md` with this minimal structure and commands:

````markdown
# Contributing to Selfishell

Keep each change focused and preserve unrelated worktree changes. Behavioral
changes require isolated tests using a temporary `HOME`.

## Local development

```bash
./bin/selfishell help
./bin/selfishell install --dry-run
bash scripts/check.sh
```

See `AGENTS.md` for repository implementation constraints and
`docs/RELEASING.md` for maintainer release procedures.
````

Use Markdown links for `AGENTS.md` and `docs/RELEASING.md` in the actual file.

- [ ] **Step 4: Verify the detailed documents**

Run:

```bash
rg -n '^## (Bootstrap options|Ghostty customization|Uninstallation|Platform notes)$' docs/INSTALLATION.md
rg -n '^## Neovim workflow$' docs/PROFILES.md
rg -n '^# Contributing to Selfishell$|^## Local development$' CONTRIBUTING.md
git diff --check
```

Expected: every named heading appears once; `git diff --check` prints nothing and exits 0.

- [ ] **Step 5: Commit focused documentation targets**

```bash
git add docs/INSTALLATION.md docs/PROFILES.md CONTRIBUTING.md
git commit -m "docs: organize detailed user guidance"
```

### Task 2: Rewrite README as the New-User Entry Point

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: detailed link targets completed in Task 1, `profiles/minimal.conf`, `profiles/developer.conf`, `common/mise.toml`, and `dependencies.conf`
- Produces: a concise standalone quick-start path plus navigation into focused documentation

- [ ] **Step 1: Replace the README structure**

Rewrite `README.md` using exactly this top-level order:

```markdown
# Selfishell

Selfishell is a managed Zsh development environment for macOS, Ubuntu, and
Ubuntu on WSL.

![Selfishell shell prompt ...](img/selfishell.png)

## Quick Start
## What Is Selfishell?
## What You Get
## Profiles
## Everyday Commands
## Safety
## Documentation
```

Keep the existing Neovim screenshot under `What You Get`. Do not restore separate top-level sections for Neovim, Ghostty, uninstallation, advanced setup, development, or platform notes.

- [ ] **Step 2: Write the standalone quick-start path**

Include only these commands, in this order:

```bash
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
selfishell install
exec zsh
selfishell doctor
selfishell status
```

State that the bootstrap installs only the CLI, `selfishell install` selects the default `developer` profile, and detailed options live in `docs/INSTALLATION.md`.

- [ ] **Step 3: Write concise product, profile, command, and safety sections**

Keep product prose outcome-focused. Include:

- managed Zsh, Starship, aliases, completions, editor configuration, updates, and offline rollback;
- the exact supported platform list from Global Constraints;
- a two-row `minimal`/`developer` table using versions from Global Constraints;
- links to `docs/PROFILES.md` for full package and Neovim detail;
- `status`, `doctor`, `update`, `rollback`, and `uninstall --restore --dry-run` as everyday commands;
- one sentence that `sfs` is the optional short alias;
- one short safety paragraph covering non-root XDG installation, backups, user-owned `.zshrc`, checksum verification, and no automatic startup updates.

Avoid implementation internals, long flag explanations, and repeated instructions already present in linked documents.

- [ ] **Step 4: Replace the documentation list with task-oriented links**

Include links to:

```text
docs/INSTALLATION.md
docs/PROFILES.md
docs/PYTHON.md
docs/UPDATES.md
docs/TROUBLESHOOTING.md
docs/SECURITY.md
docs/COMPANY.md
CONTRIBUTING.md
docs/RELEASING.md
docs/MILESTONES.md
SECURITY.md
```

Also keep `docs/PERFORMANCE.md` under user guides and `docs/DISTRIBUTION.md`
under maintainer guides. Group user guides before contributor and maintainer
guides.

- [ ] **Step 5: Check size, structure, facts, and local links**

Run:

```bash
wc -l README.md
rg -n '^## ' README.md
rg -n 'Neovim 0\.12\.4|Tree-sitter CLI 0\.26\.11|Node\.js 24\.18\.0|Python 3\.13\.14|uv 0\.5\.21' README.md
test -f docs/INSTALLATION.md
test -f docs/PROFILES.md
test -f docs/PYTHON.md
test -f docs/UPDATES.md
test -f docs/TROUBLESHOOTING.md
test -f docs/SECURITY.md
test -f docs/COMPANY.md
test -f docs/PERFORMANCE.md
test -f CONTRIBUTING.md
test -f docs/DISTRIBUTION.md
test -f docs/RELEASING.md
test -f docs/MILESTONES.md
test -f SECURITY.md
git diff --check
```

Expected: README is roughly 140–180 lines; headings match the approved order; current pinned versions appear; every `test -f` and `git diff --check` exits 0.

- [ ] **Step 6: Run the repository gate**

Run:

```bash
bash scripts/check.sh
```

Expected: command exits 0 with all syntax, lint, format, and test checks passing.

- [ ] **Step 7: Review the final documentation diff**

Run:

```bash
git diff --stat HEAD~1
git diff HEAD~1 -- README.md docs/INSTALLATION.md docs/PROFILES.md CONTRIBUTING.md
git status --short
```

Confirm no detail removed from README lacks an obvious destination, no unsupported claim was added, and only the planned documentation files plus this plan are changed.

- [ ] **Step 8: Commit the README rewrite**

```bash
git add README.md
git commit -m "docs: streamline README for new users"
```
