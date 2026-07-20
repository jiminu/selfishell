# Mise Global Configuration Ownership Plan

## Context

The `developer` profile installs Selfishell's reviewed tool selectors into
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/selfishell.toml` and exposes
them to mise through this link:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/mise/conf.d/selfishell.toml
```

When the conventional mise global configuration file
`${XDG_CONFIG_HOME:-$HOME/.config}/mise/config.toml` does not exist, a global
write such as `mise use -g tool@version` can select the loaded Selfishell
`conf.d` file as its write target. That mutates a file which must remain a
Selfishell-owned copy of `common/mise.toml`. It can then conflict with a later
`selfishell update`, be reported as user-modified state, and mix personal tool
choices with reviewed Selfishell defaults.

Mise documents `~/.config/mise/config.toml` as its normal global configuration
file and as the `--global` write target. Selfishell should ensure that this
separate user-facing target exists.

## Intended Result

After a developer-profile installation:

```text
~/.config/mise/config.toml                 user global configuration
~/.config/mise/conf.d/selfishell.toml      link to Selfishell defaults
~/.config/selfishell/mise/selfishell.toml  Selfishell-managed copied defaults
```

`mise use -g node@…`, `mise settings …`, and comparable global mise commands
must write `~/.config/mise/config.toml`; they must not alter the file in
`conf.d`. Project-local `mise.toml` files must retain their existing override
behavior.

## Implementation Slice

1. Add a deliberately empty template file, for example
   `common/mise-global-config.toml`. Keep it separate from `common/mise.toml`:
   the former is a user-global write target and the latter contains reviewed
   Selfishell selectors.
2. Add a managed resource for
   `${XDG_CONFIG_HOME:-$HOME/.config}/mise/config.toml`, selected only for the
   `developer` profile alongside `mise-config-link`.
3. Reuse the existing managed-resource state lifecycle rather than using an
   untracked `touch` operation. This is needed to preserve the project's
   backup, atomic-write, dry-run, and uninstall invariants.
4. Treat an existing `config.toml` as user data:
   - never overwrite it during install or update;
   - let the normal resource logic back it up before placing the empty template,
     if that is the established behavior for a newly managed regular file;
   - never replace it merely because Selfishell's empty template changes.
5. Record the checksum after creating the empty file. On uninstall, remove it
   only while the managed state and checksum still show the original empty file.
   If mise or the user has written any content, preserve it as user data.
6. Keep `mise-config-file` and `mise-config-link` unchanged: they remain the
   source of Selfishell's pinned tool versions and are still removed/restored by
   the existing lifecycle.
7. Remove or correct the obsolete conditional in `common/runtime.zsh` that
   checks `MISE_GLOBAL_CONFIG_FILE` against
   `.../selfishell/mise/config.toml`. The current managed default file is named
   `selfishell.toml`, and normal shell activation should not set
   `MISE_GLOBAL_CONFIG_FILE` at all. Verify the change does not override a
   user-supplied environment variable.

Do not solve this by exporting `MISE_GLOBAL_CONFIG_FILE` during shell startup.
That would redirect all user global writes into a Selfishell-owned location and
would violate the separation above.

## Resource-Lifecycle Decisions to Verify During Implementation

Before editing resource code, inspect the current create/update/uninstall
paths in `lib/resources.sh` and the state implementation. Confirm that adding
the new resource has all of these properties:

- the `minimal` profile neither creates nor tracks `~/.config/mise/config.toml`;
- `--dry-run` creates no directory, file, backup, or state record;
- an occupied path of another type (directory, regular file, or link) follows
  the existing safe conflict/backup rules and never loses data;
- re-running developer installation is idempotent;
- a user-modified file is not replaced or deleted by update/uninstall;
- a pre-existing personal `config.toml` survives install and uninstall exactly
  according to the normal managed-resource backup/restore contract;
- state format is not changed unless adding this resource exposes a missing
  field. If it must change, increment the internal state format version.

## Tests

Add coverage near `tests/managed_install_test.bash`, `tests/common_zsh_test.bash`,
and any resource/uninstall test that exercises regular files.

1. Fresh developer install creates an empty
   `$XDG_CONFIG_HOME/mise/config.toml`, the Selfishell `conf.d` link, and state
   for both resources.
2. Fresh minimal install does not create `$XDG_CONFIG_HOME/mise/config.toml` or
   the `conf.d` link.
3. A fake `mise use -g` (or an integration-compatible equivalent) writes to
   `$XDG_CONFIG_HOME/mise/config.toml`; assert that
   `$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml` remains byte-identical to
   `common/mise.toml`.
4. Two developer installs leave the empty global config intact and create no
   extra backup or state.
5. Existing `config.toml` content is preserved safely on install, including the
   project's defined backup behavior; no content is silently overwritten.
6. After adding content to the generated global config, uninstall preserves the
   file but removes the intact Selfishell `conf.d` resource as appropriate.
7. Uninstall removes a still-empty, unmodified Selfishell-created global config
   and its state record.
8. Dry-run covers absent and existing global configs without creating any mise
   directory, file, backup, or state.
9. `common/runtime.zsh` keeps a caller-provided
   `MISE_GLOBAL_CONFIG_FILE` unchanged and does not set it for normal developer
   activation.

Run the applicable shell syntax checks and the focused test files, followed by
the repository's normal test command before marking this work complete.

## Documentation Follow-up

Update `docs/PROFILES.md` and, if needed, `README.md` to distinguish:

- reviewed Selfishell defaults in `mise/conf.d/selfishell.toml`; and
- personal global mise choices in `mise/config.toml`, including `mise use -g`.

Document that project `mise.toml` files still take precedence over global
defaults. Do not document `MISE_GLOBAL_CONFIG_FILE` as a recommended workaround.
