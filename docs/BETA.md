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

- [ ] Review `selfishell install --profile minimal --dry-run`.
- [ ] Install the beta CLI and minimal profile.
- [ ] Open a new terminal and verify prompt rendering and Git completion.
- [ ] Run `selfishell status`, `self-update`, and `rollback`.
- [ ] Run uninstall with restore and confirm the original configuration returns.
- [ ] Record package-manager prompts, PATH guidance, and any usability issues.

## Result Record

```text
Release:
Platform and version:
Architecture:
Date:
Tester:
Automated CI run:
Result:
Notes or issue links:
```
