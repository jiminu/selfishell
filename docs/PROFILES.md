# Profiles

Profiles are cumulative. Choose the smallest profile that covers the machine's
role.

| Profile | Purpose |
| --- | --- |
| `minimal` | Core shell, Zinit, FZF, Zoxide, Ripgrep, Eza, Bat, Vim, Vundle, and macOS terminal fonts |
| `developer` | Minimal plus jq, pyenv, pyenv-virtualenv, NVM, build tooling, Kubernetes tools, and OpenJDK 17 |

`minimal` is selected when `--profile` is omitted.

The `developer` profile installs both pyenv and pyenv-virtualenv. Shell
initialization stays lazy and enables virtualenv auto-activation when pyenv is
first used.

Preview without changing the machine:

```sh
selfishell install --profile developer --dry-run
```

Install or change the selected profile explicitly:

```sh
selfishell install --profile developer --yes
```

The active profile is recorded in the XDG state directory. `selfishell update`
uses that recorded profile when updating approved direct tools and managed
configuration. Apt and Homebrew retain responsibility for system package
versions.

On macOS, interactive installation separately asks whether to install Ghostty
and manage its configuration. `--yes` accepts that choice automatically. The
choice is saved and reused by `selfishell update`.

The former `kubernetes` and `full` profiles were removed during the beta. A
machine that recorded either profile should run `selfishell install --profile
developer --yes` once to select the new profile structure.
