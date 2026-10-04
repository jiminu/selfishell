# Troubleshooting

## Command Not Found

Add the default binary directory to the shell path and start a new shell:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## Platform or Dependency Diagnosis

```sh
selfishell status
```

`status` starts with the current CLI and rollback versions, then checks the
platform, architecture, package manager, C compiler, managed configuration
integrity, installed tools, and Zsh plugin checkouts. Each healthy group is
summarized in one line; problems are listed individually. Before setup, it
checks only the platform, architecture, and package manager. Use
`selfishell status --verbose` for every system check and managed path, and each
tool's installed version, source, and approved version. `selfishell doctor`
remains an alias of `status`.

A tool's source is `selfishell`, `mise`, `homebrew`, `homebrew-cask`, `apt`,
`external` (found, but installed some other way), or `none` when the tool is
missing. Package-manager versions are reported without an exact approved
version because those repositories control resolution. `status` returns
nonzero when a system check fails, required tools are missing, a Zsh plugin
checkout is missing, modified, or at an unapproved revision, or managed
configuration is not installed, missing, or changed. Optional missing tools
remain informational, including in the summary.
Problem groups include a next step, and paths beneath the current home
directory use `~`.

Diagnostics do not check the network or change Selfishell configuration,
state, or tools. They list installed mise tools with `mise ls`, which can
update mise's own cache and tracking metadata.

After completed setup, `status` also reports missing installation records for
configuration required on the current platform, including Ghostty when enabled.
Keep existing backups and review the reported records before reinstalling.

## Restricted Network

Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are inherited.
Use `--skip-packages` for configuration-only setup; see
[update modes](UPDATES.md#update-modes). To download Selfishell releases from a
mirror, see [Company deployment](COMPANY.md#release-mirror).

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
progressing download can finish. The bootstrap installer retries a release
download up to three times within one minute after a timeout or an HTTP 408,
429, 500, 502, 503, or 504 response; a failure reports curl's final error.

## Windows Terminal cannot find the font

During the first WSL font installation, an already-open Windows Terminal may
warn that it cannot find `JetBrainsMonoNL Nerd Font Mono` when Selfishell applies
the profile settings. The running Terminal may not have picked up the newly
registered font yet.

Let installation finish, then close all Windows Terminal windows and reopen
your configured WSL profile. If the font works after restarting, no reinstall
is needed.

If the warning remains, check the installation output for optional font
installation failures and run `selfishell status --verbose`. An installation
with `--skip-packages` does not download or register fonts. Resolve any reported
font installation failure and rerun `selfishell install` without that flag,
then restart Terminal again. See [Windows Terminal on WSL](INSTALLATION.md#windows-terminal-on-wsl).

## Clipboard over SSH and on WSL

Over SSH and on WSL, Neovim copies with OSC 52, so a yank reaches your local
or Windows clipboard; WSL has no clipboard tool Neovim can use by default. The
terminal must allow it: Ghostty and Windows Terminal do by default, iTerm2
needs "Applications in terminal may access clipboard", and tmux needs
`set -g set-clipboard on`. Paste local clipboard text with the terminal's paste
shortcut; `p` pastes the last Neovim yank.

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

If the whole block was deleted or a regular rc file was replaced, run
`selfishell update --tools-only --skip-packages` to add the current block at the
top of the file while preserving the file's other content.
`selfishell install --skip-packages` also repairs it. Uninstall leaves an already absent block and its user file
alone. Incomplete or duplicated markers must still be resolved before retrying.

Install and update reprovision a Zinit checkout whose tracked files were
edited. An edited Neovim plugin checkout stops them instead, because lazy.nvim
will not check out over local changes: remove that plugin's directory under
`${XDG_DATA_HOME:-$HOME/.local/share}/nvim/lazy`, then retry.

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
