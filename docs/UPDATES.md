# Updates and Rollback

`selfishell update` updates the managed environment and CLI release by default.
The CLI release is switched first. If it changed, the new CLI continues the
same command so packages newly added to that release's profile are included.

```sh
selfishell status --check-updates
selfishell update --yes
selfishell update --cli-only --yes
selfishell update --tools-only --yes
selfishell rollback --yes
```

The tools/configuration phase synchronizes apt or Homebrew packages from the
installed profile, installs directly managed tools at the approved versions in
`dependencies.conf`, reapplies managed configuration, and installs Vim plugins
declared by the managed vimrc. Already installed operating-system packages
remain managed by apt or Homebrew; this command does not perform a general
package upgrade. A CLI-only installation skips this phase.

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

Interactive Zsh sessions read the installed `VERSION` file and show a cached
notification when a newer Selfishell CLI release is available. The cache is
refreshed in the background at most once per day, so neither a CLI process nor
the network request blocks shell startup. The notification never installs an
update automatically. Disable it or change its interval in
`~/.config/selfishell/local.zsh`:

```zsh
export SELFISHELL_UPDATE_NOTICE=0
# Or keep notices enabled and check every 12 hours.
export SELFISHELL_UPDATE_CHECK_INTERVAL=43200
```

The default interval is 86400 seconds. Restricted-network and offline users
should disable the notice explicitly.
