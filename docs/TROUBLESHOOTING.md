# Troubleshooting

## Command Not Found

Add the default binary directory to the shell path and start a new shell:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## Platform or Dependency Diagnosis

```sh
selfishell doctor
selfishell status
selfishell status --check-updates
```

`status` reports tools from the active profile as Selfishell-managed, Homebrew,
apt, external, or missing. Package-manager versions are reported without an
exact approved version because those repositories control resolution. The
command returns nonzero when required tools are missing or managed
configuration is missing or changed. It does not modify files.

After changing an existing `developer` installation to mise, `doctor` may
report preserved `~/.nvm` or `~/.pyenv` directories. This is informational:
Selfishell no longer initializes those managers, but does not delete their
installed runtimes, global packages, or virtual environments. Verify the mise
replacement before removing legacy data manually.

## Restricted Network

Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are inherited.
Use `SELFISHELL_OFFLINE=1` or `--skip-packages` for configuration-only setup.

Release and direct-tool downloads stop when they cannot connect or remain below
the minimum transfer rate. Metadata checks also have a short total deadline.
Slow or high-latency networks can tune the positive-integer values, in seconds
or bytes per second as appropriate:

```sh
export SELFISHELL_CURL_CONNECT_TIMEOUT=20
export SELFISHELL_CURL_LOW_SPEED_LIMIT=256
export SELFISHELL_CURL_LOW_SPEED_TIME=120
export SELFISHELL_CURL_METADATA_MAX_TIME=30
```

Archive downloads deliberately have no fixed total deadline, so a slow but
progressing download can finish.

## Modified Managed File

Selfishell refuses to overwrite or remove a managed file whose checksum changed.
Move the customized file aside, compare it with the corresponding file under
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell`, then rerun the command. Selfishell
does not discard the customized copy automatically.

## Failed CLI Update

A failed download or checksum validation leaves `current` unchanged. After a
successful update, return to the retained release without network access:

```sh
selfishell rollback --yes
```

## Removal and Restore

```sh
selfishell uninstall --dry-run --restore
selfishell uninstall --restore --yes
```

Restore stops if its destination is occupied, preserving both paths.
