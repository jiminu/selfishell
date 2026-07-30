# Default Update Check for `status`

## Problem

`selfishell status` previously printed `Available: not checked` unless the user added
`--check-updates`. This makes the default status output incomplete even though a
release metadata lookup is normally quick. The opt-in flag also adds an option
that users must discover and remember.

## Behavior

- `selfishell status` checks the latest CLI release by default.
- A successful lookup prints the discovered version in `Available`.
- If `SELFISHELL_OFFLINE=1` is set, `status` skips the lookup and prints
  `Available: unavailable`.
- If the lookup fails because the network or release endpoint is unavailable,
  `status` continues without an error and prints `Available: unavailable`.
- A failed update lookup does not change the exit status determined by local
  resource and tool checks.
- `--check-updates` is removed. Supplying it produces the standard unknown-option
  usage error.
- No inverse `--no-check-updates` option is added.
- `--check-package-updates` and `--verbose` keep their existing behavior.

This changes only an explicitly invoked CLI command. Ordinary shell startup
continues to avoid blocking on the network.

## Implementation

The status command will initialize `Available` to `unavailable`. Unless offline
mode is enabled, it will call the existing release lookup helper and replace the
value only when a version is returned successfully. Network failure remains
non-fatal and silent because local status information is still useful.

The removed flag will be deleted from option parsing, help text, and current user
documentation.

## Verification

Regression tests will cover:

- a plain `status` command reporting the latest stable release;
- prerelease fallback without the removed flag;
- an unavailable release endpoint preserving local status and reporting
  `Available: unavailable`;
- offline mode skipping the release lookup and reporting `unavailable`;
- rejection of the removed `--check-updates` option.

The focused release/bootstrap tests and the full repository gate will run before
the change is considered complete.
