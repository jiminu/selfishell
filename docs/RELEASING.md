# Releasing Selfishell

Release archives are platform and architecture labeled even though the current
payload is shell-based. This keeps the download contract stable if native assets
are added later.

## Build Artifacts

Run the release-candidate check with an exact semantic version:

```bash
bash scripts/release-check.sh 0.2.2
```

The output must contain:

```text
VERSION
SHA256SUMS
selfishell-0.2.2-linux-amd64.tar.gz
selfishell-0.2.2-linux-arm64.tar.gz
selfishell-0.2.2-macos-amd64.tar.gz
selfishell-0.2.2-macos-arm64.tar.gz
```

## Automated Publish

The `v<major>.<minor>.<patch>` tag is the stable release version source. A suffix
such as `v0.2.2-beta.1` creates a GitHub pre-release. The release workflow runs
the full suite, builds every archive, smoke-tests an exact install, and creates
the GitHub Release with all archives, `SHA256SUMS`, and `VERSION`. The GitHub
Release title is the version tag itself, such as `v0.2.2`; artifact filenames
retain the `selfishell-` prefix so downloaded files remain identifiable.

```bash
git tag -a v0.2.2 -m 'Selfishell 0.2.2'
git push origin v0.2.2
```

After the workflow completes:

1. Verify all expected assets are attached to the GitHub Release.
2. Verify `releases/latest/download/VERSION` resolves to the stable version.
3. Run the exact-version bootstrap on the beta machines in
   `docs/project/BETA.md`.
4. Record failures as issues and publish a new patch release after fixes.

Do not replace assets on an existing release. Publish a new patch version so the
version-to-checksum relationship remains immutable.

## Approved dependency patch releases

The weekly dependency workflow opens or refreshes a PR from
`automation/dependency-updates`. It never merges the PR. When a maintainer merges
that exact branch and the PR changes only `dependencies.conf`, the dependency
release workflow calculates the next patch after the latest stable tag and
dispatches the regular release workflow against the merge commit. Verification,
artifact building, the exact-version smoke test, tag creation, and GitHub Release
publication all remain part of the regular release gate.

Every release asset receives signed build provenance through GitHub Artifact
Attestations before publication. Verification requires GitHub CLI:

```bash
gh attestation verify PATH_TO_ARCHIVE --repo jiminu/selfishell
```

Release, dependency discovery, and dependency patch-dispatch failures create a
deduplicated `automation-failure` issue. A later successful run closes the open
issue. Maintainers receive email when GitHub issue or repository-watch email
notifications are enabled in their account settings.

General feature and documentation PRs never trigger this path. If an automated
dependency PR needs another file changed, close it and use the normal manual
release process instead.
