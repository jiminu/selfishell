# Releasing Selfishell

Release archives are platform and architecture labeled even though the current
payload is shell-based. This keeps the download contract stable if native assets
are added later.

## Build Artifacts

Run the full verification suite, then build an exact version:

```bash
bash scripts/check.sh
bash scripts/build-release.sh --version 1.0.0 --output dist
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

## Publish

1. Confirm `VERSION` in the source tree matches the intended release.
2. Create a `v<version>` Git tag from a fully tested commit.
3. Create a GitHub Release for that tag.
4. Upload every generated archive, `SHA256SUMS`, and `VERSION` as release assets.
5. Verify the release with an exact-version install into a temporary prefix.
6. Verify the GitHub `releases/latest/download/VERSION` URL resolves to the new
   stable version.

Do not replace assets on an existing release. Publish a new patch version so the
version-to-checksum relationship remains immutable.

Release publication will be automated in M6. Until then, this checklist is the
required manual release contract.
