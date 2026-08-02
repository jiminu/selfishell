# Quality Closeout Design

## Goal

Close the highest-value verification and presentation gaps found in the final
project review without changing Selfishell's runtime behavior. This slice makes
performance evidence easier to trust, prevents CI tool versions from drifting
away from the developer tool source of truth, refreshes the public screenshots,
and states exactly which platforms are verified.

## Scope

This work has four deliverables:

1. make base benchmark inputs deterministic and add an opt-in full-profile
   startup diagnostic;
2. verify that the pinned Neovim CI job consumes the selected versions from
   `common/mise.toml` instead of repeating version literals;
3. replace the README shell and Neovim screenshots with captures of the current
   managed configuration;
4. document the difference between supported platforms and the environments
   that CI actually exercises.

This slice does not introduce blocking performance budgets, optimize startup,
remove plugins, change profile contents, merge the distinct responsibilities of
`profiles/developer.conf` and `common/mise.toml`, add new platform runners, or
automate screenshot capture.

## Performance Measurement

Base mode remains the fast, network-free benchmark. Its executable discovery
must not depend on the developer machine's ambient `PATH`: optional integrations
are either explicitly provided to the benchmark or reported as absent. Existing
result labels and TSV output remain stable so historical CI artifacts stay
comparable.

Full mode remains an advisory Ubuntu CI job. Add an opt-in diagnostic that uses
Zsh's built-in profiler during a representative full-profile startup and saves
the report beside the existing benchmark result. The diagnostic is evidence for
finding expensive initialization paths; it is not a product timer, a new runtime
dependency, or a pass/fail threshold. Normal shell startup must remain unchanged.

The performance guide will explain the deterministic base environment, how to
request the diagnostic locally, and how to interpret the separate benchmark and
profiling artifacts.

## Mise Version Consistency

`common/mise.toml` remains the source of truth for mise-managed developer tool
versions. `profiles/developer.conf` still declares what the installer must
install; it is not treated as duplicate configuration.

The `neovim-developer-e2e` workflow will read the Neovim, Tree-sitter, and Node
selectors from `common/mise.toml` once and expose them to its later steps. Its
install, configuration-test, and lifecycle commands will consume those derived
selectors without embedding version numbers. A focused existing workflow test
will assert both that the job reads `common/mise.toml` and that version literals
for those tools are absent from the job. This prevents a default-version upgrade
from silently leaving one of the CI commands behind.

No general TOML parser or new dependency will be added. The file's existing
simple `[tools]` layout is sufficient for a small shell extraction, and the test
will fail clearly if a required key is missing.

## Screenshots

Replace `img/selfishell.png` and `img/nvim.png` with fresh captures produced from
the current managed developer configuration. The shell capture must show the
current prompt hierarchy and Git context without personal or sensitive data.
The Neovim capture must show the current explorer, buffer line, syntax colors,
and compact statusline using a small public or disposable fixture.

Keep the existing README image locations and concise alt text so this remains an
asset refresh rather than a documentation restructure. Inspect both images at
README display size and against light and dark page backgrounds. Screenshot
pixel equality is not a product contract, so no image regression test is added.

## Platform Verification Statement

The documentation will continue to distinguish product support from evidence.
Selfishell supports macOS on Intel and Apple Silicon, native Ubuntu on AMD64 and
ARM64, and Ubuntu on WSL, as already stated. A concise verification note will
name the current automated environments:

- Ubuntu 24.04 container installation lifecycle;
- macOS configuration lifecycle on the GitHub-hosted macOS runner;
- Ubuntu-hosted pinned Neovim developer lifecycle;
- Ubuntu and macOS shell checks and base performance measurements;
- Ubuntu full-profile performance measurement.

It will also state that WSL and every advertised architecture are not exercised
as separate CI runners. This is an honest verification boundary, not a reduction
of the supported-platform contract. No platform claim will be expanded.

## Verification

- Extend the existing focused workflow test first so the old hard-coded CI
  selectors fail it, then update the workflow and confirm the test passes.
- Run benchmark argument and smoke checks for deterministic base behavior and
  the opt-in full-profile diagnostic without touching the real `HOME`.
- Run the repository gate because benchmark, workflow, and test code change:
  `bash scripts/check.sh`.
- Inspect both replacement images at their natural size and README display size
  on light and dark backgrounds; report this as manual visual verification.
- Review the final documentation wording against the actual workflow matrix and
  report CI execution as unavailable until the changed workflow runs on GitHub.
