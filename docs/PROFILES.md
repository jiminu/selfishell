# Profiles

Profiles are cumulative. Choose the smallest profile that covers the machine's
role.

| Profile | Purpose |
| --- | --- |
| `minimal` | Core shell, Zinit, Vim, and macOS terminal fonts |
| `developer` | Minimal plus Neovim 0.12.4, Tree-sitter CLI 0.26.11, Node.js 24.18.0, Python 3.13.14, FZF, Zoxide, Ripgrep, Eza, Bat, jq, and compiler tooling |

`minimal` is selected when `--profile` is omitted.

The `developer` profile installs a pinned mise binary and activates it for
interactive Zsh. Selfishell keeps its defaults in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/mise/config.toml`; a project's
`mise.toml` can select different tool versions. Existing NVM, pyenv, and
system-Java installations are not removed.

Built-in mise tools use exact reviewed versions. Projects remain free to
override them in a local `mise.toml`. Updating these defaults requires a normal
Selfishell release and never happens during shell startup.

Preview without changing the machine:

```sh
selfishell install --profile developer --dry-run
```

Install or change the selected profile explicitly:

```sh
selfishell install --profile developer --yes
```

The active profile is recorded in the XDG state directory. `selfishell update`
uses that recorded profile to install missing Apt, Homebrew, and directly
managed tools before updating configuration. Apt and Homebrew retain
responsibility for versions of packages they already manage.

Profile package requirements have two failure policies:

- `required` packages must be available and install successfully;
- `optional` packages are recommended and attempted automatically, but an
  unavailable package or installation failure does not stop the rest of setup.

`optional` does not mean that Selfishell asks about each package. Ghostty is the
separate interactive installation choice on macOS.

On macOS, interactive installation separately asks whether to install Ghostty
and manage its configuration. `--yes` accepts that choice automatically. The
choice is saved and reused by `selfishell update`.

The former `kubernetes` and `full` profiles were removed during the beta. A
machine that recorded either profile should run `selfishell install --profile
developer --yes` once to select the new profile structure.
