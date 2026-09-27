# Updates and Rollback

`selfishell update` switches to the latest CLI release, then uses that release's
package list to synchronize tools and configuration. If the target release is
already active, it exits without changing anything; use `--tools-only` to
resynchronize the current environment.
The CLI phase installs a checksum-verified native executable; no Go toolchain
is needed on the user's machine.

## Update modes

| Command | Effect |
| --- | --- |
| `selfishell update` | Update the CLI, then synchronize tools and configuration. |
| `selfishell update --cli-only` | Update only the CLI release. |
| `selfishell update --tools-only` | Synchronize the current release's tools and configuration. |
| `selfishell update --skip-packages` | Update the CLI, then apply configuration if the release changed. |
| `selfishell update --tools-only --skip-packages` | Reapply current configuration without network access. |

Add `--yes` for non-interactive confirmation or `--dry-run` to preview the
selected phases without changing tools, configuration, or the active release.
When the CLI would change, the tools preview uses the running release's manifest;
the actual update uses the downloaded target release. The preview identifies
this limitation and does not download that release's configuration.
Use `--version VERSION` to select an exact release; it cannot be combined with
`--tools-only`. Without it, update never moves to an older release: an active
release newer than the latest one, such as a prerelease, is kept.

A successful update reports the version transition, such as
`Selfishell updated: 1.2.10 -> 1.2.14`; release notes are on GitHub.
If the tools/configuration phase is declined or fails, the CLI may already have
switched: check `selfishell version` for the active release. Tools-only updates
finish with `Selfishell tools and configuration synchronized.`

## Tools and configuration

Synchronization installs missing Apt or Homebrew packages from `packages.conf`,
applies approved direct-tool and Git dependency versions from `dependencies.conf`,
synchronizes mise tools and Neovim plugins, and reapplies managed configuration.
Optional Apt packages are attempted automatically; a failed optional package
is reported but does not fail the whole setup. Required package failures do.
Tree-sitter parsers install on first opening their filetype. Existing Apt and
Homebrew packages are not upgraded; use `brew upgrade` or the operating system's
Apt upgrade policy separately.

Selfishell also verifies the bytes of its managed mise executable against the
release's approved checksum. A manual `mise self-update` is replaced with the
approved version on the next tools synchronization. An external mise installation
that Selfishell does not own remains untouched.

`--skip-packages` skips all package and tool installation when this phase runs,
matching `selfishell install --skip-packages`. CLI-only and already-current
default updates skip the entire phase.

### Neovim LSP servers

The tools phase refreshes the Mason registry and synchronizes only Selfishell's
seven default servers: lua_ls, pyright, bashls, jsonls, yamlls, tombi, and marksman.
Their exact versions are pinned in
`config/shared/nvim/lua/config/languages.lua` as `server@version` entries.
Missing servers are installed; servers already at the approved version are left
alone. A different installed version, including a newer manually installed one,
is replaced with this release's pin. User-added servers and other Mason packages
remain under `:Mason` control.

Run `selfishell update --tools-only` to reapply these pins when the CLI release is
already current. New LSP pins arrive through dependency update PRs and Selfishell
releases. `--cli-only`, `--skip-packages`, and `--dry-run` do not run Mason.
Registry or server installation failures fail the update and skip mise cleanup;
earlier tool and configuration changes may already have applied. Resolve the
reported error and retry with `selfishell update --tools-only`.

### Unused mise versions

After successful synchronization, Selfishell runs `mise prune --tools --yes`
for its declared mise tools on the current platform. It registers the current
release's configuration to protect its pins and excludes the previous release's
configuration during cleanup, even if already tracked. Rollback-only tool
versions can be removed; the previous CLI release and its files remain intact.

Versions needed by tracked, trusted project configurations are retained.
Unrelated tools and configuration tracking remain intact. Cleanup also covers
versions installed outside Selfishell: versions used only through environment
variables, one-off `mise exec tool@version`, or untracked projects can be
removed. Record those versions in project configuration and load it with mise
before updating. See [mise prune](https://mise.jdx.dev/cli/prune.html).

Cleanup is skipped on installation, rollback, `--cli-only`, `--skip-packages`,
an already-current default update, or incomplete synchronization, including
optional package failures. Configured mise `ignored_config_paths` also disables
cleanup because ignored configurations cannot protect their pins. Cleanup or
retention-preflight failures warn without undoing the successful synchronization.

`--dry-run` describes the cleanup scope without invoking mise or writing its
tracking/cache metadata; it does not list individual deletion candidates.

## CLI releases and rollback

The CLI phase downloads a versioned platform archive, verifies its SHA-256
checksum, and switches `current` only after validation. It retains the active
and previous releases, removing only recognized older inactive releases.
Unknown directories in `releases` and foreign `current` or `previous` links
are preserved. An occupied release-link path or failure to save `previous`
stops the update before activation, keeping the active release intact.
If activation fails after saving `previous`, Selfishell attempts to restore its
original value. Release cleanup is skipped when retention links cannot be read
or validated safely.
Version discovery prefers the latest stable release; if none exists, it accepts
the newest version tag only when that exact release's `VERSION` asset is published.
If a freshly downloaded release activates but the following state commit
fails, the active CLI can have changed even though `update` reports failure.
Check `selfishell version` and rerun the update after resolving the state error.

```sh
selfishell rollback --yes
```

Rollback exchanges `current` and `previous` without network access. It restores
only the CLI, leaving configuration and tools unchanged. Reapplying an older
environment may require downloading tool versions removed by cleanup. Select
an exact retained version with `selfishell rollback VERSION`.
If either release link is occupied by a user path, or the active release cannot
be saved as `previous`, rollback stops before changing `current`.

Release changes do not take a lock, so run one `selfishell update`,
`selfishell rollback`, or bootstrap `install.sh` at a time. Overlapping runs can
prune a release that another run activates, leaving the `selfishell` link
broken; shell startup is unaffected, and rerunning the bootstrap installer
repairs the CLI.

## Approved versions

Direct-tool and Git dependency versions, including exact Neovim plugin commits,
are approved in `dependencies.conf`. A repository `lazy-lock.json` is unnecessary:
lazy.nvim's runtime lock lives under Selfishell's state directory, and updates
cannot move plugins beyond approved commits.

`packages.conf` declares mise tool membership; `config/shared/mise.toml` pins
exact defaults. Both manifests change through review and a Selfishell release.
Project-local `mise.toml` files remain outside this lifecycle.

Maintainers use `scripts/update-dependencies.sh` to discover upstream releases,
calculate downloaded mise artifact checksums, and update mise tool and default
LSP pins. LSP candidates come from the published Mason registry; only the servers
declared in `config/shared/nvim/lua/config/languages.lua` are considered. This
launcher builds the Go maintenance tool with the version pinned in `go.mod`;
discovery requires curl and Git. `--metadata FILE` applies saved metadata without
network access. Node and Python release lines remain a manual maintainer choice.
Go's official release metadata supplies stable patch updates within the
development toolchain's existing release line. These update `go.mod` and the
root `mise.toml` together; the workflow selects the updated compiler before
validation. A new Go release line remains a maintainer choice. This development
toolchain update does not add Go to the installed environment.
All manifest and configuration edits are validated and staged before any file
is replaced, and each replacement uses an atomic rename. The
weekly workflow runs the same script, skips shell tooling setup and the full
verification suite when no pins change, and otherwise opens or refreshes
`automation/dependency-updates` only when tracked files change. It never merges
or publishes. Review upstream release notes, checksums, and CI before merging,
then follow the [release procedure](RELEASING.md) to deliver the changes.

## Status and update notices

`selfishell status` reports local CLI, rollback, tool, and managed-resource state
without checking the network or available Apt/Homebrew updates. Use
`selfishell version --available` to check the latest release.

Interactive Zsh reads the installed `VERSION` and displays a cached notice for
newer releases. Metadata refreshes in the background at most once per day;
startup does not wait for a CLI process or network request, and notices never
install updates. Configure notices in `~/.zshrc`, outside the marked loader block:

```zsh
export SELFISHELL_UPDATE_NOTICE=0
# Or keep notices enabled and check every 12 hours.
export SELFISHELL_UPDATE_CHECK_INTERVAL=43200
```

The default interval is 86400 seconds. Restricted-network and offline users
should disable the notice explicitly.
