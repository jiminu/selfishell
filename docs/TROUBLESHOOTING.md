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

`status` and `doctor` summarize healthy groups and list missing tools or other
problems individually. Use `selfishell status --verbose` for every managed path
and each tool's installed version, source, and approved version. Use
`selfishell doctor --verbose` for individual system checks and installed tools.

Both commands start with the current CLI and rollback versions. `status` checks
managed configuration integrity and tool installation. `doctor` checks platform,
architecture, package manager, compiler availability, installed tools, and Zsh
plugin checkouts. Use `status` to inspect configuration changes and `doctor` to
diagnose environment prerequisites and plugin problems.

Tool sources include Selfishell-managed, Homebrew, apt, external, or missing.
Package-manager versions are reported without an exact approved version because
those repositories control resolution. `status` returns nonzero when required
tools are missing or managed configuration is missing or changed. Optional
missing tools remain informational, including in the summary. Problem groups
include a next step, and paths beneath the current home directory use `~`.
Diagnostics do not modify files or check the network.

After completed setup, `status` also reports missing installation records for
configuration required on the current platform, including Ghostty when enabled.
Keep existing backups and review the reported records before reinstalling.

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
`selfishell update --tools-only --skip-packages` to append the current block
while preserving the file's other content. `selfishell install --skip-packages`
also repairs it. Uninstall leaves an already absent block and its user file
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
