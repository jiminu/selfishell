# Updates and Rollback

`selfishell update` switches to the latest CLI release, then uses that release's
package list to synchronize tools and configuration. If the target release is
already active, it exits without changing anything; use `--tools-only` to
resynchronize the current environment.

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
Use `--version VERSION` to select an exact release; it cannot be combined with
`--tools-only`.

A successful update reports the version transition, such as
`Selfishell updated: 1.2.10 -> 1.2.14`; release notes are on GitHub.
If the tools/configuration phase is declined or fails, the CLI may already have
switched: check `selfishell version` for the active release. Tools-only updates
finish with `Selfishell tools and configuration synchronized.`

## Tools and configuration

Synchronization installs missing Apt or Homebrew packages from `packages.conf`,
applies approved direct-tool and Git dependency versions from `dependencies.conf`,
synchronizes mise tools and Neovim plugins, and reapplies managed configuration.
Tree-sitter parsers install on first opening their filetype. Existing Apt and
Homebrew packages are not upgraded; use `brew upgrade` or the operating system's
Apt upgrade policy separately.

`--skip-packages` skips all package and tool installation when this phase runs,
matching `selfishell install --skip-packages`. CLI-only and already-current
default updates skip the entire phase.

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
and previous releases, removing older inactive releases. Version discovery
prefers the latest stable release; if none exists, it accepts the newest version
tag only when that exact release's `VERSION` asset is published.

```sh
selfishell rollback --yes
```

Rollback exchanges `current` and `previous` without network access. It restores
only the CLI, leaving configuration and tools unchanged. Reapplying an older
environment may require downloading tool versions removed by cleanup. Select
an exact retained version with `selfishell rollback VERSION`.

## Approved versions

Direct-tool and Git dependency versions, including exact Neovim plugin commits,
are approved in `dependencies.conf`. A repository `lazy-lock.json` is unnecessary:
lazy.nvim's runtime lock lives under Selfishell's state directory, and updates
cannot move plugins beyond approved commits.

`packages.conf` declares mise tool membership; `config/shared/mise.toml` pins
exact defaults. Both manifests change through review and a Selfishell release.
Project-local `mise.toml` files remain outside this lifecycle.

Maintainers use `scripts/update-dependencies.sh` to discover upstream releases,
calculate downloaded mise artifact checksums, and update mise tool pins. The
weekly workflow runs the same script and opens or refreshes
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
