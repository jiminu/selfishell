# Releasing Selfishell

Release archives are platform and architecture labeled even though the current
payload is shell-based. This keeps the download contract stable if native assets
are added later.

## Build Artifacts

Run the release-candidate check with an exact semantic version:

```bash
bash scripts/release-check.sh 1.0.0
```

The output must contain:

```text
VERSION
SHA256SUMS
selfishell-1.0.0-linux-amd64.tar.gz
selfishell-1.0.0-linux-arm64.tar.gz
selfishell-1.0.0-macos-amd64.tar.gz
selfishell-1.0.0-macos-arm64.tar.gz
```

## Automated Publish

The `v<major>.<minor>.<patch>` tag is the stable release version source. A suffix
such as `v1.0.0-beta.1` creates a GitHub pre-release. The release workflow runs
the full suite, builds every archive, smoke-tests an exact install, and creates
the GitHub Release with all archives, `SHA256SUMS`, and `VERSION`.

```bash
git tag -a v1.0.0 -m 'Selfishell 1.0.0'
git push origin v1.0.0
```

After the workflow completes:

1. Verify all expected assets are attached to the GitHub Release.
2. Verify `releases/latest/download/VERSION` resolves to the stable version.
3. Run the exact-version bootstrap on the beta machines in
   `docs/project/BETA.md`.
4. Record failures as issues and publish a new patch release after fixes.

Do not replace assets on an existing release. Publish a new patch version so the
version-to-checksum relationship remains immutable.
