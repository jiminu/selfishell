# Updates and Rollback

By default, `selfishell update` updates to the latest release and then
synchronizes that release's managed environment. The CLI release is switched
first; if it changed, the new CLI continues the same command so packages newly
added to that release's package list are included. If the target release is
already installed, the command reports that and exits without changing
anything. Use `--tools-only` to explicitly resynchronize the current release's
tools and configuration regardless of whether a new release is available.

A successful update finishes with one result line naming the version
transition, for example `Selfishell updated: 1.2.10 -> 1.2.14`. Release details
stay on the GitHub Release rather than being reproduced in the CLI, and output
for work that actually changed the environment, warnings, errors, and
`--dry-run` previews are unaffected. Because that result closes the whole
command, declining the tools/configuration confirmation or failing in that
phase ends the run without reporting the version change even though the CLI
release has already switched; `selfishell version` confirms the active release.
`--tools-only` closes with `Selfishell tools and configuration synchronized.`
and no version transition: that phase resynchronizes the release's tools and
configuration whether or not anything changes, so its result does not claim
one.

```sh
selfishell status
selfishell update --yes
selfishell update --cli-only --yes
selfishell update --tools-only --yes
selfishell update --skip-packages --yes
selfishell rollback --yes
```

The tools/configuration phase synchronizes apt or Homebrew packages from the
release's `packages.conf`, installs directly managed tools at the approved versions in
`dependencies.conf`, synchronizes mise-managed developer tools, reapplies
managed configuration, and synchronizes Neovim plugins;
Tree-sitter parsers install lazily the first time their filetype is
opened. Already installed operating-system packages remain
managed by apt or Homebrew; this command does not perform a general package
upgrade. A CLI-only update skips this phase, as does a default update
that finds the target release already installed;
`selfishell update --tools-only --skip-packages` reapplies just the managed
configuration for the current release.

`--skip-packages` applies only when the tools/configuration phase runs: it
skips package and tool installation and applies managed configuration only,
the same contract `selfishell install` follows.

After a successful tools/configuration update, Selfishell automatically runs
`mise prune --tools --yes`, scoped to its mise tools for the current platform.
It first registers the current release configuration with mise so its pinned
versions remain available. During cleanup only, the previous release's mise
configuration is excluded, even if mise already tracks it. Rollback-only tool
versions are not retained; versions still needed by mise's tracked, trusted
project configurations are retained. The previous CLI release and its
configuration files remain intact. Unrelated tools and
tracked configuration links are not removed. This is mise's unused-version
cleanup, not just removal of versions installed by Selfishell: versions of these
tools used only via environment variables, one-off `mise exec tool@version`, or
untracked projects can be removed. Keep such versions in a project configuration
and load it with mise before updating. See [mise prune](https://mise.jdx.dev/cli/prune.html).

Cleanup does not run on installation, rollback, `--cli-only`, `--skip-packages`,
an already-current default update, or a failed synchronization. If an optional
package failed to install, cleanup is skipped too. Cleanup or retention-preflight
failures produce a warning without undoing the successful synchronization.
For safety, configured mise `ignored_config_paths` also disables automatic
cleanup: ignored configurations cannot protect their pinned versions.
`--dry-run` describes the cleanup scope without invoking mise or writing its
tracking/cache metadata; it does not enumerate individual deletion candidates.

`status` reports local CLI, rollback, tools, and managed-resource state
only; it never checks the network. Use `selfishell version --available` to
check the latest published release, or rely on the automatic update notice.

`status` does not check Apt or Homebrew for available package updates. Use
`brew upgrade` or the operating system's Apt upgrade policy to apply system
package updates explicitly.

The CLI phase downloads a versioned platform archive, verifies its published
SHA-256 checksum, retains the active release, and switches `current` only after
validation. After a successful switch, only the active release and the previous
rollback release are retained; older inactive releases are removed. Automatic
version discovery prefers the latest stable release. If
there is no stable release, it checks the newest version tag and accepts it only
when that exact release's `VERSION` asset is available. Use `--version VERSION`
to select an exact release. `--version` cannot be combined with `--tools-only`.

`--dry-run` previews every selected phase without changing tools,
configuration, or the active CLI release.

`selfishell rollback` exchanges the `current` and `previous` release links and
does not use the network. It restores the CLI release, not the managed
configuration or tool installations. Reapplying the older environment may
require downloading tool versions removed by cleanup. An exact retained
version can be selected with `selfishell rollback VERSION`.

Direct download and Git dependency versions are changed only by reviewing and
updating `dependencies.conf` in a new Selfishell release.
That manifest is also the source of truth for exact Neovim plugin commits, so a
repository `lazy-lock.json` is intentionally unnecessary. lazy.nvim may write a
runtime lock under the Selfishell state directory, but updates cannot move a
plugin beyond the commit approved in the release manifest.
`packages.conf` declares mise-managed developer tool membership;
exact default versions are pinned only in `config/shared/mise.toml`, updated through
the same review-and-release boundary. Individual project `mise.toml` files
remain outside Selfishell's update lifecycle.

Maintainers can run `scripts/update-dependencies.sh` to discover current
upstream releases, download mise platform artifacts, calculate their
checksums, and bump mise-managed tool versions, including Starship. The weekly `Dependency
updates` workflow uses the same script and opens or refreshes a review PR
only when a tracked file changes. It never merges the PR or publishes a
Selfishell release. Review
upstream release notes and the generated checksums before merging, then publish
a normal Selfishell patch release so users receive the approved versions through
`selfishell update`.

Merging the generated `automation/dependency-updates` PR does not publish a
release by itself. A maintainer runs the normal manual release process
afterward, the same as for any other change.

Interactive Zsh sessions read the installed `VERSION` file and show a cached
notification when a newer Selfishell CLI release is available. The cache is
refreshed in the background at most once per day, so neither a CLI process nor
the network request blocks shell startup. The notification never installs an
update automatically. Disable it or change its interval in `~/.zshrc`, outside
the marked Selfishell loader block:

```zsh
export SELFISHELL_UPDATE_NOTICE=0
# Or keep notices enabled and check every 12 hours.
export SELFISHELL_UPDATE_CHECK_INTERVAL=43200
```

The default interval is 86400 seconds. Restricted-network and offline users
should disable the notice explicitly.
