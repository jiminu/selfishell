# Selfishell

A lightweight and intuitive Zsh environment powered by Starship and Zinit. Selfishell stays fast and readable while providing useful completions, aliases, runtime information, and Git status at a glance.

## Preview

![Selfishell shell prompt showing the current directory, Git branch, command output, and time](img/preview.png)

Selfishell keeps the primary prompt focused on the current directory and Git status, while contextual information such as the time appears on the right.

Git and kubectl integration works without Oh My Zsh. Git uses Zsh's built-in
completion, while kubectl completion is cached during setup. If the cache is
unavailable, it is generated on the first completion request without delaying
shell startup.

## Usage

Clone this repository and run:

```bash
bash ./bootstrap.sh
```

`bootstrap.sh` detects the current environment and runs the appropriate legacy
setup for macOS or Ubuntu on WSL.

The repository also includes the managed configuration CLI:

```bash
./bin/selfishell help
./bin/selfishell version
./bin/selfishell doctor
./bin/selfishell install --dry-run
./bin/selfishell install --profile developer --yes
./bin/selfishell status
```

`./bin/sfs` is an optional shorthand for the same CLI. The existing `bootstrap.sh`
entrypoint remains a compatibility wrapper for the current full package
bootstrap while profiles and managed package installation are developed.

The CLI copies configuration into `${XDG_CONFIG_HOME:-$HOME/.config}/selfishell` and
links the active user's Zsh, Starship, Vim, and platform-specific configuration
to those managed copies. It stores recovery metadata under
`${XDG_STATE_HOME:-$HOME/.local/state}/selfishell`, so the source checkout can be
moved or deleted after managed installation.

Use the following command to remove managed files and restore configuration that
was backed up during installation:

```bash
./bin/selfishell uninstall --restore --yes
```

If a managed file or link was changed after installation, uninstall stops before
removing anything and preserves both the changed path and its original backup.

## Profiles

Selfishell separates package selection from installation logic:

| Profile | Included tools |
| --- | --- |
| `minimal` | Zsh, Git, Curl, Starship |
| `developer` | Minimal plus FZF, Zoxide, Eza, Bat, pyenv, NVM, Vim, build tools |
| `kubernetes` | Developer plus kubectl and context tools |
| `full` | Kubernetes plus supported macOS desktop, font, and Java integrations |

`developer` is the default. Preview another profile without changing packages or
files:

```bash
./bin/selfishell install --profile kubernetes --dry-run
```

For restricted networks, configuration can be installed without any package or
network operation:

```bash
SELFISHELL_OFFLINE=1 ./bin/selfishell install --yes
# or
./bin/selfishell install --skip-packages --yes
```

Standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` variables are inherited by
package managers and direct download commands.

Private or company packages can be added without changing the repository:

```text
# company.conf
package macos required formula company-cli
package ubuntu required apt company-cli
```

```bash
./bin/selfishell install --local-profile ./company.conf --yes
```

Private shell configuration can be placed in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh`. Selfishell loads this
file but does not overwrite, track, or remove it.

## Supported Environments

The current bootstrap officially supports:

- macOS on Apple Silicon or Intel, using Homebrew
- Ubuntu running on WSL

Native Ubuntu and other Linux distributions are not supported by the current
entrypoint yet. Broader platform support is planned as part of the Selfishell CLI
roadmap.

## Before You Run It

The bootstrap changes the current user's development environment. In particular,
it may:

- install system packages and request administrator privileges;
- change the default login shell to Zsh on Ubuntu WSL;
- move existing `.zshrc`, `.vimrc`, Starship, and Ghostty configuration files to
  timestamped backups;
- replace those paths with symbolic links into this repository checkout;
- download and execute third-party installers and clone plugin repositories.

Keep the checkout in a stable location after running the legacy `bootstrap.sh`
bootstrap because its links still point into the checkout. Configuration created
with `selfishell install` is copied to the managed XDG directory and does not have
this limitation.

On Ubuntu WSL, missing required packages stop setup with a nonzero exit status.
Unavailable optional convenience tools such as FZF, Zoxide, Eza, or Bat are
reported at the end without failing the rest of setup.

## What It Installs

### macOS

- Homebrew
- Zsh tools: Starship, Zinit, Zoxide, FZF
- CLI tools: Git, Eza, Bat, kubectl, kubectx
- Runtime tools: pyenv, NVM, OpenJDK 17
- Vim and Vundle configuration
- Ghostty
- Meslo Nerd Font and Noto Sans CJK KR

### Ubuntu on WSL

- Zsh, Git, Curl, Unzip, and build tools
- Starship, Zinit, Zoxide, FZF
- Eza, Bat, Vim
- pyenv and NVM
- Vundle and Vim plugins
- Zsh as the default shell

The setup also links the shared Zsh, Starship, and Vim configuration files into the appropriate locations under the home directory.

## Notes

- Only macOS and Ubuntu on WSL are supported.
- The setup may request administrator privileges for Homebrew, apt packages, or changing the default shell.
- Existing configuration files and symbolic links are backed up with a timestamp before replacement.
- Network access is required to download packages and plugins.
- Open a new terminal after installation, or run `source ~/.zshrc`.
- On WSL, fonts are rendered by the Windows terminal application. Install a Nerd Font on Windows and select it in Windows Terminal or VS Code to display Starship icons correctly.
- On macOS, restart Ghostty after installation to apply its configuration.
