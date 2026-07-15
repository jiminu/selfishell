# Updates and Rollback

Selfishell separates environment updates from CLI release updates.

```sh
selfishell status --check-updates
selfishell update --yes
selfishell self-update --yes
selfishell rollback --yes
```

`selfishell update` reapplies the installed profile's configuration and installs
directly managed tools at the approved versions in `dependencies.conf`. System
packages installed by apt or Homebrew continue to follow those package managers;
this command does not perform a general operating-system package upgrade.

`selfishell self-update` downloads a versioned platform archive, verifies its
published SHA-256 checksum, retains the active release, and switches `current`
only after validation. Use `--version VERSION` to select an exact release.

`selfishell rollback` exchanges the `current` and `previous` release links and
does not use the network. An exact retained version can be selected with
`selfishell rollback VERSION`.

Direct download and Git dependency versions are changed only by reviewing and
updating `dependencies.conf` in a new Selfishell release. Interactive shell
startup never checks the network or updates tools.
