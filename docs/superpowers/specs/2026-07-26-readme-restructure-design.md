# README Restructure Design

## Goal

Make the README a concise entry point for new users. A reader should quickly
understand what Selfishell is, install it, choose a profile, and find detailed
documentation without reading operational or maintainer detail.

Target roughly 140–180 lines. Clarity takes precedence over an exact line
count.

## Audience

The primary reader is a new Selfishell user evaluating or installing the
project. Contributor and maintainer procedures do not belong in the main
README.

## README Structure

Use this order:

1. Project name, one-sentence description, and shell screenshot.
2. Quick Start: install CLI, install the default profile, open Zsh, and verify.
3. What Selfishell Is: supported platforms and the managed-environment model.
4. What You Get: short outcome-focused feature list and Neovim screenshot.
5. Profiles: compact `developer` and `minimal` comparison.
6. Everyday Commands: only the commands most users need.
7. Safety: one short summary of backups, checksums, user-owned `.zshrc`, and
   non-root XDG installation.
8. Documentation: links grouped by user task.

The one-sentence description must precede installation commands so readers
know what they are about to run. Installation otherwise appears before the
longer product explanation.

## Content Placement

- Keep the standard installation path and `developer`/`minimal` choice in the
  README.
- Move detailed bootstrap options, exact-version installation, offline setup,
  PATH management, uninstall, and platform notes to `docs/INSTALLATION.md`.
- Move detailed Neovim workflow information to `docs/PROFILES.md`.
- Move Ghostty customization details to `docs/INSTALLATION.md`.
- Link update and rollback details to `docs/UPDATES.md`.
- Link local company profile guidance to `docs/COMPANY.md`.
- Move source-checkout development commands to a new `CONTRIBUTING.md`.
- Delete repeated explanations already covered by a linked document.

Moved content must remain discoverable. Existing detail absent from its target
document must be added there before removal from the README.

## Accuracy and Scope

- Preserve the canonical `selfishell` command and optional `sfs` alias.
- Preserve current supported platforms, default profile, pinned tool versions,
  public installer URL, and safety claims.
- Do not add product behavior, distribution endpoints, or platform claims.
- Do not change scripts, CLI behavior, profiles, dependencies, or releases.
- Do not edit milestone status.

## Verification

- Compare all retained commands, versions, links, and claims against repository
  sources of truth.
- Check Markdown links and headings for broken or duplicate destinations.
- Run `bash scripts/check.sh`, as required by repository rules for project
  documentation that describes installation and lifecycle behavior.
- Report only checks actually run.

## Success Criteria

- README follows the approved installation-first flow.
- README stays within roughly 140–180 lines unless clarity requires a small
  deviation.
- New users can install and verify Selfishell from the README alone.
- Detailed behavior removed from the README exists in an obvious linked
  document.
- Contributor instructions live outside the README.
