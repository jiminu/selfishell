# Releasing Selfishell

Selfishell uses the immutable `v<version>` Git tag as the single source of truth
for a release version. The source tree does not carry a release `VERSION` file;
`VERSION` is generated only for built and published release artifacts.

Release archives are platform and architecture labeled even though the current
payload is shell-based. This keeps the download contract stable if native assets
are added later.

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
release branch, currently `main`. The normal PR CI should be green before the
release tag is created; the Release workflow reruns the repository checks on
Linux and macOS before publication and rejects a tag whose commit is not in
`main` history.

Before tagging, require a clean worktree, confirm `HEAD` is the intended pushed
`origin/main` commit, and verify that `v<version>` does not already exist locally
or remotely. Existing release tags are immutable; never move or recreate them.

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

The Release workflow validates the pushed tag, runs the full verification
suite, builds every platform archive, smoke-tests an exact install, generates
GitHub Artifact Attestations, and creates the GitHub Release with all archives,
`SHA256SUMS`, and generated `VERSION`. The GitHub Release title is the version
tag itself, such as `v1.2.3`; artifact filenames retain the `selfishell-`
prefix so downloaded files remain identifiable.

## Verify the published release

After the workflow completes:

```bash
bash scripts/verify-published-release.sh 1.2.3
```

This verifies the exact asset set, checksums, GitHub Artifact Attestations, the
tag's `install.sh`, an isolated exact-version bootstrap, and
`releases/latest/download/VERSION` for stable releases. Attestation
verification requires a gh CLI with the `attestation` subcommand and fails the
script by default if it is unavailable; set
`SELFISHELL_VERIFY_SKIP_ATTESTATION=1` to explicitly verify without it.

Record failures as issues and publish a new patch release after fixes. Do not
replace assets on an existing release; keeping tag-to-artifact checksums
immutable is part of the release contract.

## Approved dependency updates

The weekly dependency workflow opens or refreshes a PR from
`automation/dependency-updates`. It never merges the PR or publishes a release.
Review the diff and CI results, merge when ready, then publish a normal patch
release by creating the next release tag. Use `scripts/next-patch-version.sh`
when you want the helper to calculate that patch version.

Every release asset receives signed build provenance through GitHub Artifact
Attestations before publication. Verification requires GitHub CLI:

```bash
gh attestation verify PATH_TO_ARCHIVE --repo jiminu/selfishell
```
