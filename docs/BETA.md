# Public Beta Verification

Selfishell does not require dedicated clean physical machines for beta entry.
Automated lifecycle tests run in GitHub-hosted Ubuntu and macOS environments that
are recreated for each job. Human verification uses an existing machine and a
public GitHub pre-release.

Container-only testing is insufficient for macOS, but a GitHub-hosted macOS
runner is an acceptable clean OS environment for configuration-only lifecycle
coverage. Package-manager prompts and terminal ergonomics remain manual checks.

## Automated Clean Runner Gate

- [x] Ubuntu runner verifies exact bootstrap, minimal configuration install,
      doctor, CLI upgrade, offline rollback, uninstall, and backup restoration.
- [x] macOS runner verifies the same lifecycle using the native macOS platform
      and system Bash.
- [x] Direct downloads and package installation are skipped in lifecycle E2E;
      their adapters, checksums, and failure behavior are covered separately.

Reference run: [Ubuntu and macOS native lifecycle CI](https://github.com/jiminu/selfishell/actions/runs/29426591218).

## Pre-release Gate

- [x] Publish a `v<version>-beta.<number>` GitHub pre-release.
- [x] Verify the README curl command against the published assets.
- [x] Confirm `VERSION`, `SHA256SUMS`, and all four platform archives are present.
- [ ] Leave the pre-release available long enough to collect installation
      feedback before declaring a stable release.

Published candidate: [v0.1.0-beta.1](https://github.com/jiminu/selfishell/releases/tag/v0.1.0-beta.1).
The public installer was verified on Linux AMD64 using an isolated `HOME` and
prefix on 2026-07-16.

## Existing Machine Smoke Test

Run these checks on one existing macOS or Ubuntu development machine. A fresh
user account or temporary `HOME` is preferred but not required.

- [x] Review `selfishell install --profile minimal --dry-run`.
- [x] Install the beta CLI and minimal profile.
- [ ] Open a new terminal and verify prompt rendering and Git completion.
- [ ] Run `selfishell status`, `self-update`, and `rollback`.
- [x] Run uninstall with restore and confirm the original configuration returns.
- [x] Record package-manager prompts, PATH guidance, and any usability issues.

## Result Record

```text
Release: v0.1.0-beta.1
Platform and version: macOS 26.5.2 (25F84)
Architecture: arm64
Date: 2026-07-16
Tester: Codex-assisted isolated-home smoke test
Automated CI run: https://github.com/jiminu/selfishell/actions/runs/29426591218
Result: Follow-up required before completing M6
Notes or issue links:
- The public archive installed successfully and printed correct PATH guidance.
- The minimal dry-run, offline configuration install, doctor, status, and
  uninstall with restoration passed in an isolated HOME.
- Exact-version self-update correctly reported that the beta was already active.
- Rollback correctly reported that no previous release existed in the clean
  prefix, so a successful retained-release rollback still needs manual coverage.
- Git completion was unavailable because minimal does not install Zinit while
  completion initialization depended on Zinit. The completion initialization
  is now independent of Zinit and has regression coverage; verify it in the next
  published beta.
- Package-manager prompts were intentionally not exercised to avoid changing
  the development machine outside the isolated HOME.
```

### Local follow-up candidate

The completion fix was packaged locally as `0.1.0-beta.2` and tested on the
same machine and isolated home. Updating from the public `0.1.0-beta.1` release
to that candidate retained beta.1 as `previous`. A new TTY-backed login Zsh
initialized Starship and mapped Git completion to `_git` without Zinit. An
offline rollback restored beta.1, and uninstall restored the original `.zshrc`.

These checks validate the candidate behavior but do not replace verification of
the next published pre-release. Keep the remaining smoke-test items open until
the candidate is published and installed through the public release path.
