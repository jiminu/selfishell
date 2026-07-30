# Test Reliability Design

## Goal

Make the existing test suite trustworthy before reorganizing or optimizing it.
This slice fixes false-pass risks, guarantees temporary-home cleanup, prevents
manually omitted tests, and runs the Neovim configuration tests in the pinned
Neovim CI environment.

## Scope

This slice changes test infrastructure and CI only. It does not change
Selfishell product behavior, split large test files, extract embedded Lua, or
optimize test runtime. Those remain separate follow-up slices so this change is
small enough to review and diagnose.

## Test Runner

`tests/test_helper.bash` will provide one sequential discovered-test runner for
suites that do not need custom parallel execution. It will:

- discover every `test_*` function automatically;
- execute each test in its own subshell;
- enable `set -euo pipefail` inside that subshell without invoking the test
  function from an `if`, `!`, `&&`, or `||` condition;
- optionally call named setup and teardown hooks;
- install teardown as an `EXIT` trap so it runs after success, assertion
  failure, or an unexpected command failure;
- emit one terminal `PASS`, `FAIL`, or `SKIP` result for each executed test;
- stop the suite after the first failure, matching the effective behavior of
  the current `fail` helper.

The existing custom parallel runners in `tests/run.bash` and
`tests/managed_install_test.bash` remain. The managed-install runner will be
adjusted separately to preserve its four-way parallelism while executing each
test in a fail-safe subshell with guaranteed teardown.

Suites that intentionally share expensive state may keep explicit sequencing
in this slice. Their manual lists must be complete, and their status output must
not print `PASS` after a test has reported `SKIP`.

## Cleanup

Per-test setup must pair with teardown in the same isolated execution. Tests may
not install a new suite-level `EXIT` trap for every test because later traps
replace earlier ones and leak temporary homes. The dependency-update and
dependency-release suites will use the shared runner hooks instead.

The change will add a regression test for both runner properties:

- an unexpected command failure cannot continue to a false `PASS`;
- teardown runs when the test fails.

The test will use a child Bash process so intentionally failing fixtures do not
terminate the infrastructure test itself.

## Omitted and Skipped Tests

The uncalled `test_minimal_profile_installs_vimrc` test will be removed rather
than enabled because `test_install_copies_configuration_and_tracks_resources`
already checks both the installed Vimrc bytes and the user Vimrc link.

The shared runner will use exit status `77` for a skipped test. A `skip` helper
will print the sole `SKIP` result and exit with that status. The runner will
recognize it without also printing `PASS` or `FAIL`.

## Neovim CI Coverage

The pinned `neovim-developer-e2e` job already installs Neovim 0.12.4 through
mise. That job will run `tests/neovim_config_test.bash` inside the same pinned
mise environment before the real Neovim lifecycle. The ordinary check job may
continue to skip those tests when Neovim is unavailable; the pinned job becomes
the required execution path.

## Verification

- The new infrastructure regression test must fail against the old runner
  behavior and pass after the shared runner is implemented.
- Every migrated suite must pass when run directly.
- A repository scan must find no manually defined but unexecuted test in suites
  that retain explicit sequencing.
- A before/after temporary-directory count around the dependency suites must
  show no leaked `selfishell-test.*` directories.
- `bash scripts/check.sh` must pass.
- The local environment cannot prove the GitHub job ran; the workflow diff and
  local pinned-Neovim invocation will be verified separately and reported as
  local evidence only.
