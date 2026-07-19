# Updates and Rollback

`selfishell update` updates the managed environment and CLI release by default.
The CLI release is switched first. If it changed, the new CLI continues the
same command so packages newly added to that release's profile are included.

```sh
selfishell status --check-updates
selfishell status --check-package-updates
selfishell update --yes
selfishell update --cli-only --yes
selfishell update --tools-only --yes
selfishell rollback --yes
```

The tools/configuration phase synchronizes apt or Homebrew packages from the
installed profile, installs directly managed tools at the approved versions in
`dependencies.conf`, synchronizes mise-managed developer tools, reapplies
managed configuration, and synchronizes Neovim plugins and Tree-sitter parsers
for the developer profile. Already installed operating-system packages remain
managed by apt or Homebrew; this command does not perform a general package
upgrade. A CLI-only installation skips this phase.

`status --check-package-updates` reads Homebrew's outdated inventory or Apt's
local upgradable inventory and reports `Update: available` without installing
anything or refreshing package indexes. The normal `status` command does not run
these slower package-manager queries. Use `brew upgrade` or the operating
system's Apt upgrade policy to apply system package updates explicitly.

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
does not use the network. An exact retained version can be selected with
`selfishell rollback VERSION`.

Direct download and Git dependency versions are changed only by reviewing and
updating `dependencies.conf` in a new Selfishell release.
That manifest is also the source of truth for exact Neovim plugin commits, so a
repository `lazy-lock.json` is intentionally unnecessary. lazy.nvim may write a
runtime lock under the Selfishell state directory, but updates cannot move a
plugin beyond the commit approved in the release manifest.
The exact mise-managed defaults in `profiles/developer.conf` and
`common/mise.toml` are updated together through the same review-and-release
boundary. Individual project `mise.toml` files remain outside Selfishell's
update lifecycle.

Maintainers can run `scripts/update-dependencies.sh` to discover current
upstream releases, download platform artifacts, and calculate Starship and mise
checksums. The weekly `Dependency updates` workflow
uses the same script and opens or refreshes a review PR only when the manifest
changes. It never merges the PR or publishes a Selfishell release. Review
upstream release notes and the generated checksums before merging, then publish
a normal Selfishell patch release so users receive the approved versions through
`selfishell update`.

When the generated `automation/dependency-updates` PR changes only
`dependencies.conf`, a maintainer merge dispatches the next stable patch release
automatically. The merge remains the approval boundary: discovery never chooses
or publishes a version by itself. The release workflow calculates the next patch
from the latest stable tag, reruns macOS and Ubuntu verification, builds and
smoke-tests the artifacts, and creates the tag only after those gates pass.

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
