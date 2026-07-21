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
separate user-facing target exists, but MUST NOT manage or track it.

## Intended Result

After a developer-profile installation:

```text
~/.config/mise/config.toml                 user global configuration (not tracked)
~/.config/mise/conf.d/selfishell.toml      link to Selfishell defaults (managed)
~/.config/selfishell/mise/selfishell.toml  Selfishell-managed copied defaults (managed)
```

`mise use -g node@…`, `mise settings …`, and comparable global mise commands
must write `~/.config/mise/config.toml`; they must not alter the file in
`conf.d`.

Selfishell **does not own or manage** `~/.config/mise/config.toml`. It only ensures the file exists by creating a placeholder with comments upon fresh installation, and from that point onward, the file is treated purely as user data.

## Implementation Details

1. Remove any managed resource definition for `~/.config/mise/config.toml` (do not add `mise-config-global` to `lib/resources.sh`).
2. Delete any custom exceptions for `mise-config-global` in `lib/managed.sh` to keep the standard managed resource lifecycle clean.
3. Add a dedicated helper function `install_mise_global_config` in `lib/commands/install.sh`. This function runs only for the `developer` profile:
   - If `config.toml` already exists (as a regular file, a symlink, or a dangling symlink), it is preserved exactly as-is. Selfishell does not overwrite, backup, or change it.
   - If the target path is a directory or another unsupported type, it fails with an error and exits.
   - If it does not exist, it writes a placeholder file with descriptive comments atomically using a temporary file and rename.
   - For `--dry-run`, it prints a message and does not modify the filesystem.
4. Keep `mise-config-file` and `mise-config-link` fully managed.
5. Do not modify or set `MISE_GLOBAL_CONFIG_FILE` in `common/runtime.zsh`.

## Resource-Lifecycle Rules

- **Minimal profile**: Never creates, reads, or tracks `~/.config/mise/config.toml`.
- **Dry-run**: Never creates the directory, file, or state record. Prints a preview message if the file would be created.
- **Idempotency**: Re-running installation preserves any existing file or modifications.
- **Uninstall**: `~/.config/mise/config.toml` is **never deleted or modified** during uninstall. Leaving an empty configuration file is safe and intended.

## Tests

Add coverage to verify:

1. Fresh developer install creates a placeholder `config.toml` with guide comments when absent.
2. The created `config.toml` is not tracked (no state file is recorded).
3. Fresh minimal install does not create `config.toml`.
4. Existing user files are preserved exactly, byte-for-byte.
5. Existing symlinks and dangling symlinks are preserved exactly.
6. Re-running developer install does not overwrite user configuration.
7. Modifying `config.toml` does not cause `selfishell status` to report changes or fail.
8. Uninstalling preserves `config.toml` (even if it is unmodified/empty).
9. Dry-run does not write anything.
10. Error reporting if the target path is a directory.
