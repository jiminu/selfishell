# Selfishell

Selfishell is a managed Zsh development environment for macOS, Ubuntu, and
Ubuntu on WSL.

![Selfishell shell prompt showing the current directory, Git branch, command output, and time](img/selfishell.png)

## Quick Start

Install the CLI first:

```bash
curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/main/install.sh | bash
```

If the installer says `~/.local/bin` is not in `PATH`, run the `export PATH=…`
command it prints, then continue with the default development environment in a
fresh Zsh session and verify it:

```bash
selfishell install
exec zsh
selfishell doctor
selfishell status
```

The bootstrap installs only the CLI. `selfishell install` selects the default
`developer` profile. See the [installation guide](docs/INSTALLATION.md) for
PATH setup, non-interactive and exact-version installation, offline setup,
uninstallation, Ghostty, and platform notes.

The installer guides interactive setup, including the optional macOS Ghostty
choice. Open a new terminal after installation whenever that is more convenient
than replacing the current shell with `exec zsh`.

## What Is Selfishell?

Selfishell gives you a consistent, ready-to-use terminal across the machines
where you work. It manages Zsh, a Starship prompt, useful aliases and
completions, and editor configuration, while keeping updates and offline
rollback straightforward.

It supports:

- macOS on Apple Silicon and Intel
- native Ubuntu on AMD64 and ARM64
- Ubuntu on WSL.

It is a good fit when you want the same polished shell on several machines
without assembling and maintaining a shell framework yourself.

Managed defaults remove routine setup work without taking ownership of your
personal shell customizations. You can keep your own aliases, functions,
exports, and project tooling alongside the managed environment.

Selfishell is deliberately focused on the supported platforms above. It does
not claim support for other Linux distributions.

## What You Get

- A readable Starship prompt with Git and runtime information.
- Zsh aliases and interactive completions for common shell and Git workflows.
- Editor configuration that starts with Vim and expands into Neovim for the
  `developer` profile.
- Safe release updates, checksum verification, and retained releases for
  offline rollback.

The resulting environment keeps common development tools close at hand without
requiring a separate dotfiles framework. Its managed configuration is designed
to make repeatable setup, maintenance, and recovery practical.

The `developer` profile includes a practical Neovim workspace for everyday
development.

![Selfishell Neovim workspace showing the file explorer, buffer tabs, shell-script syntax highlighting, and compact statusline](img/nvim.png)

## Profiles

Choose the profile that matches the machine. `developer` is the default;
choose `minimal` explicitly when you want the lightweight shell experience.

| Profile | Includes |
| --- | --- |
| `minimal` | Zsh, Git, Curl, Vim, Starship, Zinit, and macOS terminal fonts |
| `developer` | Everything in `minimal`, plus Neovim 0.12.4, Tree-sitter CLI 0.26.11, mise, Node.js 24.18.0, Python 3.13.14, uv 0.5.21, FZF, Zoxide, Ripgrep, Eza, Bat, jq, and compiler tools |

Read [Profiles](docs/PROFILES.md) for the complete package list, profile
behavior, and Neovim workflow.

The profile guide also explains required and optional package behavior, the
separate Ghostty choice on macOS, and how project `mise.toml` files can choose
their own runtime versions.

Choose the small profile for a machine that needs a dependable interactive
shell, or the developer profile when you also want the editor and language
toolchain ready for use.

## Everyday Commands

| Command | Use it to |
| --- | --- |
| `selfishell status` | Show the active profile, managed resources, and CLI/rollback version. |
| `selfishell doctor` | Diagnose the current installation. |
| `selfishell update` | Update the CLI, profile tools, and configuration. Installs missing apt/Homebrew packages but does not upgrade already-installed ones; see [Updates and rollback](docs/UPDATES.md). |
| `selfishell rollback` | Return to the previous retained release. |
| `selfishell uninstall --restore --dry-run` | Preview restoring backed-up configuration. |

`sfs` is the optional short alias for `selfishell`.

Run `status` after setup to see the active profile and check for a newer CLI
release. If release metadata cannot be reached, it reports availability as
`unavailable` and continues with local checks. Use `doctor` when the environment
does not look right, and use the dry run before restoring configuration so you
can review the proposed changes first.

Updates keep the installed environment current; rollback uses the retained
release when you need to return to the previous Selfishell version offline.

## Safety

Selfishell installs without root into XDG-compatible user paths, backs up
existing managed paths, keeps `.zshrc` user-owned, verifies downloaded releases
with checksums, and never installs updates automatically during shell startup.

## Documentation

### User guides

- [Installation](docs/INSTALLATION.md) — setup, offline installation,
  uninstallation, Ghostty, and platform notes.
- [Profiles](docs/PROFILES.md) — package choices and the Neovim workflow.
- [Python development](docs/PYTHON.md) — Python tooling and project setup.
- [Updates and rollback](docs/UPDATES.md) — release updates and recovery.
- [Troubleshooting](docs/TROUBLESHOOTING.md) — common installation and shell
  problems.
- [Security model](docs/SECURITY.md) — managed paths and security guarantees.
- [Company deployment](docs/COMPANY.md) — organization-specific installation.
- [Shell performance](docs/PERFORMANCE.md) — measuring and improving startup
  performance.

Start with Installation for a new machine, then use Profiles when deciding
which toolset belongs on it. The remaining user guides cover routine operation,
organization-specific deployment, and problem solving.

### Contributing

- [Contributing](CONTRIBUTING.md) — source checkout, development commands, and
  verification.

### Maintainer guides

- [Distribution channels](docs/DISTRIBUTION.md) — supported installation
  channels.
- [Release process](docs/RELEASING.md) — preparing and publishing releases.
- [Vulnerability reporting](SECURITY.md) — report a security issue privately.

The contributor and maintainer guides are separate from the quick-start path so
new users can focus on installing and using Selfishell.
