---
name: release-selfishell
description: Prepare, publish, verify, or diagnose a Selfishell stable or pre-release using immutable semantic-version tags, GitHub Actions, and published artifact checks. Use when the user asks to check release readiness, choose a release version, publish or tag a Selfishell release, or investigate release automation. Do not use for ordinary code changes or dependency-update PR review unless release publication is in scope.
---

# Release Selfishell

Coordinate the existing tag-driven release machinery without duplicating it.
Treat `docs/RELEASING.md`, `.github/workflows/release.yml`, and the immutable
`v<version>` tag as the release sources of truth; read their current contents
before acting because the workflow can evolve.

## Classify the requested mode

Determine the highest authorized action from the user's wording:

- **Audit**: inspect readiness and report; do not edit, commit, tag, push, or publish.
- **Prepare**: determine the intended version and run appropriate verification; do not tag or push.
- **Publish**: an explicit request to release, publish, tag, or “올려” an exact version authorizes creating and pushing the annotated release tag, monitoring the workflow, and performing post-publication verification after every gate passes.
- **Diagnose**: inspect an existing workflow or release failure; do not retry, delete, replace, or republish anything unless explicitly requested after the cause is known.

Do not turn “check” or “prepare” into publication. Ask for an exact version when
publication was requested without one and the intended major/minor/patch change
cannot be inferred safely. Accept `next patch` by calculating it with
`scripts/next-patch-version.sh`; never invent a major or minor bump.

## Preflight the repository

1. Read `AGENTS.md`, `docs/RELEASING.md`, and `.github/workflows/release.yml`.
2. Inspect the worktree, current branch, upstream, remotes, recent stable tags, and the commit that would be tagged.
3. Validate the release version against `X.Y.Z` or `X.Y.Z-<prerelease>` without a leading `v`.
4. Refresh remote refs and verify that neither local nor remote tag `v<version>` nor a GitHub Release with that tag already exists.
5. Require a clean worktree before publication. Preserve unrelated changes instead of hiding, stashing, or including them.
6. Publish stable releases from the documented release branch, currently `main`. Do not silently release another branch or detached commit.
7. Confirm the release commit is already pushed and matches the intended remote release branch before tagging it.

If the requested version already exists anywhere, stop. Tags and GitHub Release
assets are immutable; never delete, move, overwrite, or recreate them as a
shortcut.

## Prepare and verify the candidate

Skip mutations in Audit mode. Preparation no longer changes a tracked version
file or creates a release-only commit.

Run the repository gate when the request calls for local release readiness
verification:

```bash
bash scripts/check.sh
```

When local artifact inspection is useful, build the exact candidate into a
fresh temporary directory:

```bash
candidate_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-candidate.XXXXXX")"
bash scripts/build-release.sh --version <version> --output "$candidate_dir"
```

If artifacts are built locally, require exactly:

- `selfishell-<version>-linux-amd64.tar.gz`
- `selfishell-<version>-linux-arm64.tar.gz`
- `selfishell-<version>-macos-amd64.tar.gz`
- `selfishell-<version>-macos-arm64.tar.gz`
- `SHA256SUMS`
- `VERSION`

Verify the generated `VERSION`, archive names, and checksum entries match the
requested version. Remove the temporary candidate directory after its evidence
is no longer needed.

Stop on any failure. Report the failing command and preserve its evidence. Do
not weaken, skip, or edit checks merely to make a release pass.

## Publish an authorized release

Perform this section only in Publish mode after the intended commit is already
on the release branch and any requested readiness checks pass.

1. Confirm `HEAD` is the exact pushed commit to release and the worktree is clean.
2. Create annotated tag `v<version>` with message `Selfishell <version>`.
3. Push only the tag:

   ```bash
   git push origin v<version>
   ```

4. If the push fails, inspect and report local versus remote tag state before attempting anything else.
5. Monitor the Release workflow through the available GitHub integration or `gh`. Report the run URL and the first failing job or step if it does not complete successfully.

The Release workflow derives the version from the tag, validates it, reruns the
repository checks on Linux and macOS, builds release artifacts, smoke-tests an
exact install, generates attestations, and publishes the GitHub Release. Do not
create or maintain a separate source `VERSION` file.

## Verify the published release

After a successful workflow, run:

```bash
bash scripts/verify-published-release.sh <version>
```

Require it to confirm the release classification, exact asset set, checksums,
available GitHub Artifact Attestations, tagged installer, isolated exact-version
bootstrap, and latest-stable behavior. The script uses temporary HOME, XDG, and
prefix paths; never substitute the developer's real environment.

Do not modify an existing release to repair a failed verification. Diagnose the
cause, fix it normally, and publish a new version—usually the next patch.

## Report the outcome

Summarize:

- requested mode and version;
- tagged commit and annotated tag, if created;
- local verification result, if run;
- GitHub workflow URL and result, if published;
- asset, checksum, attestation, bootstrap, and isolated-install results;
- any incomplete or manual follow-up.

Distinguish clearly between “candidate verified,” “tag pushed,” and “release
fully verified.” Do not call a release complete while required publication
checks remain outstanding.
