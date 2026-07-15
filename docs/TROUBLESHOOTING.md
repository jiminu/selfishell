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

`status` returns nonzero when managed configuration is missing or changed. It
does not modify files.

## Restricted Network

Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are inherited.
Use `SELFISHELL_OFFLINE=1` or `--skip-packages` for configuration-only setup.

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
