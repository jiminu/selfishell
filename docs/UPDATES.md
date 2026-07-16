# Updates and Rollback

`selfishell update` updates the managed environment and CLI release by default.
The environment is updated first and the CLI release is switched last.

```sh
selfishell status --check-updates
selfishell update --yes
selfishell update --cli-only --yes
selfishell update --tools-only --yes
selfishell rollback --yes
```

The tools/configuration phase reapplies the installed profile's configuration
and installs directly managed tools at the approved versions in
`dependencies.conf`. System packages installed by apt or Homebrew continue to
follow those package managers; this command does not perform a general
operating-system package upgrade. A CLI-only installation skips this phase.

The CLI phase downloads a versioned platform archive, verifies its published
SHA-256 checksum, retains the active release, and switches `current` only after
validation. Automatic version discovery prefers the latest stable release and
falls back to the most recently published pre-release when no stable release is
available. Use `--version VERSION` to select an exact release. `--version` cannot
be combined with `--tools-only`.

`--dry-run` previews every selected phase without changing tools,
configuration, or the active CLI release.

`selfishell rollback` exchanges the `current` and `previous` release links and
does not use the network. An exact retained version can be selected with
`selfishell rollback VERSION`.

Direct download and Git dependency versions are changed only by reviewing and
updating `dependencies.conf` in a new Selfishell release.

Interactive Zsh sessions show a cached notification when a newer Selfishell CLI
release is available. The cache is refreshed in the background at most once per
day, so the network request does not block shell startup. The notification never
installs an update automatically. Disable it or change its interval in
`~/.config/selfishell/local.zsh`:

```zsh
export SELFISHELL_UPDATE_NOTICE=0
# Or keep notices enabled and check every 12 hours.
export SELFISHELL_UPDATE_CHECK_INTERVAL=43200
```

The default interval is 86400 seconds. Restricted-network and offline users
should disable the notice explicitly.
