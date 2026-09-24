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
```

`status` reports tools from the environment as Selfishell-managed, Homebrew,
apt, external, or missing. Package-manager versions are reported without an
exact approved version because those repositories control resolution. It
returns nonzero when required tools are missing or managed configuration is
missing or changed. It does not modify files or check the network.

## Restricted Network

Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are inherited.
Use `--skip-packages` for configuration-only setup.

Release and direct-tool downloads stop when they cannot connect or remain below
the minimum transfer rate. Git clones and Neovim plugin syncs use the same
low-speed limit unless `GIT_HTTP_LOW_SPEED_LIMIT` or `GIT_HTTP_LOW_SPEED_TIME`
is already set. Metadata checks also have a short total deadline.
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

## Clipboard over SSH

Over SSH, Neovim copies with OSC 52, so a yank reaches your local clipboard
instead of the remote host's. The local terminal must allow it: Ghostty and
Windows Terminal do by default, iTerm2 needs "Applications in terminal may
access clipboard", and tmux needs `set -g set-clipboard on`. Paste local
clipboard text with the terminal's paste shortcut; `p` pastes the last Neovim
yank.

## Modified Managed File

For a modified managed file or marked block, an interactive install or update
offers to overwrite or skip it. Overwrite first saves the full file under
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell/backups`; skip preserves the
file and continues. `--yes` and non-interactive runs never overwrite a detected
modification and stop instead. These checks run before any package or file
change, so a stop leaves everything as it was. `uninstall --purge` keeps these
backups.

Untouched marked blocks from an older Selfishell release are upgraded
automatically. Bytes outside the block markers remain unchanged. Uninstall
still refuses to remove a block that was modified after installation.

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
