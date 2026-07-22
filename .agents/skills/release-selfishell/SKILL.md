---
name: release-selfishell
description: Prepare, verify, publish, or diagnose a Selfishell stable or pre-release by coordinating VERSION, the release-candidate gate, immutable annotated tags, atomic Git pushes, GitHub Actions, and published artifact checks. Use when the user asks to check release readiness, prepare a version, publish or tag a Selfishell release, or investigate its release automation. Do not use for ordinary code changes or dependency-update PR review unless release publication is in scope.
---

# Release Selfishell

Coordinate the existing release machinery without duplicating it. Treat `VERSION`, `docs/RELEASING.md`, `scripts/release-check.sh`, and `.github/workflows/release.yml` as sources of truth; read their current contents before acting because the workflow can evolve.

## Classify the requested mode

Determine the highest authorized action from the user's wording:

- **Audit**: inspect readiness and report; do not edit, commit, tag, push, or dispatch.
- **Prepare**: update `VERSION` and run the complete local release gate; do not commit, tag, push, or dispatch.
- **Publish**: an explicit request to release, publish, tag, or “올려” an exact version authorizes the normal version commit, annotated tag, atomic push, workflow monitoring, and post-publication verification after every gate passes.
- **Diagnose**: inspect an existing workflow or release failure; do not retry, delete, replace, or republish anything unless explicitly requested after the cause is known.

Do not turn “check” or “prepare” into publication. Ask for an exact version when publication was requested without one and the intended major/minor/patch change cannot be inferred safely. Accept `next patch` by calculating it with `scripts/next-patch-version.sh`; never invent a major or minor bump.

## Preflight the repository

1. Read `AGENTS.md`, `docs/RELEASING.md`, `VERSION`, `scripts/release-check.sh`, and `.github/workflows/release.yml`.
2. Inspect the worktree, current branch, upstream, remotes, recent stable tags, and the commit that would be tagged.
3. Validate the version against `X.Y.Z` or `X.Y.Z-<prerelease>` without a leading `v`.
4. For publication, refresh remote refs and verify that neither local nor remote tag `v<version>` nor a GitHub Release with that tag already exists.
5. Require a clean worktree before preparation. When publishing a candidate prepared in an earlier turn, allow only an unstaged `VERSION` change that exactly matches the requested version; rerun the complete gate and stop on any other change. Preserve unrelated changes instead of hiding, stashing, or including them.
6. Publish stable releases from the documented release branch, currently `main`. Do not silently release another branch or detached commit.
7. Confirm the release branch tracks the intended remote and its pre-release history is expected. Do not push the new release commit separately; the final atomic push must publish that commit and its tag together.

If the requested version already exists anywhere, stop. Tags and GitHub Release assets are immutable; never delete, move, overwrite, or recreate them as a shortcut.

## Prepare and verify the candidate

Skip mutations in Audit mode. Otherwise:

1. Change only `VERSION` to the exact requested version.
2. Create a fresh temporary candidate directory and run the canonical gate
   against it so artifacts from an earlier version cannot contaminate the
   candidate:

   ```bash
   candidate_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-candidate.XXXXXX")"
   bash scripts/release-check.sh <version> "$candidate_dir"
   ```

3. Require all checks and the build to succeed. Confirm the candidate directory
   contains exactly the expected release contract:
   - `selfishell-<version>-linux-amd64.tar.gz`
   - `selfishell-<version>-linux-arm64.tar.gz`
   - `selfishell-<version>-macos-amd64.tar.gz`
   - `selfishell-<version>-macos-arm64.tar.gz`
   - `SHA256SUMS`
   - `VERSION`
4. Verify the candidate `VERSION`, archive names, and checksum entries match the requested version.
5. Recheck the worktree. Only `VERSION` may be an intentional tracked release change. Remove the temporary candidate directory after its evidence is no longer needed.

Stop on any failure. Report the failing command and preserve its evidence. Do not weaken, skip, or edit checks merely to make a release pass.

## Publish an authorized release

Perform this section only in Publish mode after the complete gate passes.

1. Review the `VERSION` diff. If it is uncommitted, stage only that file and create the release commit with `chore: release <version>`.
2. If `HEAD` already contains the scoped release commit that changed `VERSION` to the target, verify and reuse it; never create an empty release commit. Stop if the target version came from an unrelated or ambiguous commit.
3. Create annotated tag `v<version>` with message `Selfishell <version>`.
4. Push the documented release branch and tag together:

   ```bash
   git push --atomic origin <release-branch> v<version>
   ```

5. Do not separately push the tag first. If the atomic push fails, inspect and report local versus remote state before attempting anything else.
6. Monitor the Release workflow through the available GitHub integration or `gh`. Report the run URL and the first failing job or step if it does not complete successfully.

The automated dependency patch path is separate: a qualifying merge from `automation/dependency-updates` dispatches the release workflow itself. Do not add a redundant manual version commit or tag to that path.

## Verify the published release

After a successful workflow, run:

```bash
bash scripts/verify-published-release.sh <version>
```

Require it to confirm the release classification, exact asset set, checksums,
available GitHub Artifact Attestations, tagged installer, isolated exact-version
bootstrap, and latest-stable behavior. The script uses temporary HOME, XDG, and
prefix paths; never substitute the developer's real environment.

Do not modify an existing release to repair a failed verification. Diagnose the cause, fix it normally, and publish a new version—usually the next patch.

## Report the outcome

Summarize:

- requested mode and version;
- release commit and annotated tag, if created;
- local gate result;
- GitHub workflow URL and result, if published;
- asset, checksum, attestation, bootstrap, and isolated-install results;
- any incomplete or manual follow-up.

Distinguish clearly between “candidate prepared,” “tag pushed,” and “release fully verified.” Do not call a release complete while required publication checks remain outstanding.
