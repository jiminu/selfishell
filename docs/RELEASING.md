# Releasing Selfishell

Selfishell uses the immutable `v<version>` Git tag as the single source of truth
for a release version. The source tree does not carry a release `VERSION` file;
`VERSION` is generated only for built and published release artifacts.

Release archives contain a native executable for their labeled OS and CPU,
configuration and manifests. Users do not need the Go compiler. The source
bootstrap remains standalone Bash 3.2 and verifies an exact archive before
atomic activation.

## Optional local artifact check

The Release workflow performs the authoritative verification and build after a
tag is pushed. When you want to inspect the same artifact shape locally first,
build an exact semantic version explicitly:

```bash
bash scripts/build-release.sh --version 1.2.3 --output dist
```

The output must contain:

```text
VERSION
SHA256SUMS
selfishell-1.2.3-linux-amd64.tar.gz
selfishell-1.2.3-linux-arm64.tar.gz
selfishell-1.2.3-macos-amd64.tar.gz
selfishell-1.2.3-macos-arm64.tar.gz
```

The build script does not modify source files. The generated `VERSION` file is
release metadata consumed by installed Selfishell and by the `latest` download
contract.

## Publish

Publish only a commit that is already merged and pushed to the documented
release branch, currently `main`. The normal main CI should be green before the
release tag is created. The Release workflow then calls the complete CI workflow
for the tagged commit, including the Ubuntu, macOS and Neovim E2E jobs that
ordinary CI skips when a push changes only documentation, and rejects a tag
whose commit is not in `main` history. Any failed CI job blocks publication.
Rerun failed Release jobs for a transient failure; otherwise fix `main` and
release a new patch version, because release tags are never moved.

Before tagging, require a clean worktree, confirm `HEAD` is the intended pushed
`origin/main` commit, and verify that `v<version>` does not already exist locally
or remotely. Existing release tags are immutable; never move or recreate them.
The repository enforces this: a tag ruleset on `v*` blocks tag updates and
deletion, and GitHub immutable releases prevent changing a published release's
assets.

Choose the version explicitly. For a normal patch release, the helper can derive
the next stable patch from existing tags:

```bash
bash scripts/next-patch-version.sh
```

Create and push an annotated tag on the intended `main` commit:

```bash
version=1.2.3
git tag -a "v$version" -m "Selfishell $version"
git push origin "v$version"
```

No release-only version commit is required. The tag itself is the release
version source.

A stable tag uses `v<major>.<minor>.<patch>`. A suffix such as
`v1.2.3-beta.1` creates a GitHub pre-release. Prerelease suffixes use
dot-separated SemVer identifiers made of ASCII letters, digits, and hyphens;
numeric identifiers must not contain leading zeroes.

The Release workflow validates the pushed tag, runs the full CI workflow on Linux
and macOS, builds the six-file asset set once without restoring or
saving a Go build cache, and transfers the same artifact ID to native smoke jobs
on both hosts and the publisher. Both
smokes must pass before the publisher verifies the exact file set, `VERSION`
and checksums, generates GitHub Artifact Attestations, and creates the GitHub
Release with all archives,
`SHA256SUMS`, and generated `VERSION`. The GitHub Release title is the version
tag itself, such as `v1.2.3`; artifact filenames retain the `selfishell-`
prefix so downloaded files remain identifiable.

## Verify the published release

After the workflow completes:

```bash
bash scripts/verify-published-release.sh 1.2.3
```

This verifies the exact asset set, checksums, GitHub Artifact Attestations
signed by the Release workflow for that tag, the tag's `install.sh`, an isolated exact-version bootstrap, and
`releases/latest/download/VERSION` for stable releases. Attestation
verification requires a gh CLI with the `attestation` subcommand and fails the
script by default if it is unavailable; set
`SELFISHELL_VERIFY_SKIP_ATTESTATION=1` to explicitly verify without it.

Record failures as issues and publish a new patch release after fixes. Do not
replace assets on an existing release; keeping tag-to-artifact checksums
immutable is part of the release contract.

## Approved dependency updates

`scripts/update-dependencies.sh` builds the `selfishell-dev` maintenance tool
with the Go version pinned in `go.mod`, discovers upstream releases, and
rewrites the approved pins:

- direct downloads and Git dependencies in `dependencies.conf`, including
  checksums of downloaded mise artifacts; a Zsh plugin commit is also rewritten
  in its `zinit ice ver'…'` line in `config/shared/zsh/completion.zsh` or
  `interactive.zsh`;
- mise tool versions in `config/shared/mise.toml`;
- default LSP pins in `config/shared/nvim/lua/config/languages.lua`, from the
  published Mason registry and only for the declared servers;
- Go patch releases within the current release line, in `go.mod` and the root
  `mise.toml` together. This does not add Go to the installed environment.

Node and Python release lines and a new Go release line remain maintainer
choices. Discovery requires curl and Git; `--metadata FILE` applies saved
metadata without network access. All edits are validated and staged before any
file is replaced, and each replacement uses an atomic rename.

The weekly Dependency updates workflow runs the same script without write
permission. When no pins change, it skips shell tooling setup and the
repository gate; otherwise it runs the gate with the updated compiler, and a
separate job applies only the approved files and opens or refreshes a PR from
`automation/dependency-updates`. It never merges the PR or publishes a release.
Because the workflow pushes with its `GITHUB_TOKEN`, each CI run for the PR
stops at `action_required` without running any job. After every open or
refresh, select **Approve workflows to run** in the PR's merge box (write
access required) and wait for CI to pass before merging.

Commits you push to the branch are never overwritten. A later run that finds
updates fails instead until you merge the PR, or close it and delete the branch.

Review upstream release notes, checksums, the diff, and CI results, merge when
ready, then publish a normal patch release by creating the next release tag.
Use `scripts/next-patch-version.sh` when you want the helper to calculate that
patch version.

For manual archive verification, see the [security model](SECURITY.md).
