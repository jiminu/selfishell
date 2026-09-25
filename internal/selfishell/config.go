package selfishell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const installHelp = `Usage:
  selfishell install [--skip-packages] [--dry-run] [--yes]

Options:
  --skip-packages Skip package and tool installation and apply managed configuration only
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
`
const uninstallHelp = `Usage:
  selfishell uninstall [--restore] [--purge] [--dry-run] [--yes]

Options:
  --restore  Restore configuration files backed up during installation
  --purge    Also remove the Selfishell CLI, releases, cache, and state
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
`

func (c CLI) install(args []string) int {
	skip, dry, yes := false, false, false
	for _, arg := range args {
		switch arg {
		case "--skip-packages":
			skip = true
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "help", "--help", "-h":
			fmt.Fprint(c.Out, installHelp)
			return 0
		default:
			c.error("Unknown install option: " + arg)
			return 2
		}
	}
	if !skip {
		c.error("Package installation is not implemented in the Go candidate; use --skip-packages.")
		return 1
	}
	if _, err := ReadPackages(filepath.Join(c.Root, "packages.conf")); err != nil {
		c.error(err.Error())
		return 1
	}
	if _, err := ReadDependencies(envDefault("SELFISHELL_DEPENDENCIES_FILE", filepath.Join(c.Root, "dependencies.conf"))); err != nil {
		c.error(err.Error())
		return 1
	}
	platform := DetectPlatform().Name
	if platform != "macos" && platform != "ubuntu" && platform != "ubuntu-wsl" {
		c.error("Managed installation is unavailable on " + platform + ".")
		return 1
	}
	if !yes && !dry {
		if !c.interactive() {
			c.error("Confirmation requires an interactive terminal; use --yes.")
			return 2
		}
		fmt.Fprint(c.Out, "Install Selfishell configuration? [y/N] ")
		answer, _ := c.readAnswer()
		if !affirmative(answer) {
			fmt.Fprintln(c.Out, "Cancelled.")
			return 1
		}
	}
	if err := c.installConfig(platform, dry, yes); err != nil {
		c.error(err.Error())
		return 1
	}
	return 0
}
func (c CLI) uninstall(args []string) int {
	restore, purge, dry, yes := false, false, false, false
	for _, arg := range args {
		switch arg {
		case "--restore":
			restore = true
		case "--purge":
			purge = true
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "help", "--help", "-h":
			fmt.Fprint(c.Out, uninstallHelp)
			return 0
		default:
			c.error("Unknown uninstall option: " + arg)
			return 2
		}
	}
	if !yes && !dry {
		if !c.interactive() {
			c.error("Confirmation requires an interactive terminal; use --yes.")
			return 2
		}
		prompt := "Uninstall Selfishell configuration? [y/N] "
		if purge {
			prompt = "Uninstall and purge Selfishell? [y/N] "
		}
		fmt.Fprint(c.Out, prompt)
		answer, _ := c.readAnswer()
		if !affirmative(answer) {
			fmt.Fprintln(c.Out, "Cancelled.")
			return 1
		}
	}
	if err := c.uninstallConfig(restore, purge, dry); err != nil {
		c.error(err.Error())
		return 1
	}
	return 0
}
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	return replaceRaw(path, data, mode)
}

func (c CLI) installConfig(platform string, dry, yes bool) error {
	paths, err := UserPaths()
	if err != nil {
		return err
	}
	ghostty := false
	if platform == "macos" {
		if data, e := os.ReadFile(paths.State + "/ghostty"); e == nil {
			ghostty = string(data) == "1\n"
		} else {
			ghostty = yes || dry
			if !ghostty && c.interactive() {
				fmt.Fprint(c.Out, "Install Ghostty terminal and managed configuration? [y/N] ")
				answer, _ := c.readAnswer()
				ghostty = affirmative(answer)
			}
		}
	}
	resources, err := ResourcesForPlatform(c.Root, platform, ghostty)
	if err != nil {
		return err
	}
	m := managed{c: c, paths: paths, dry: dry, yes: yes, actions: map[string]string{}}
	for _, name := range []string{"user-zshrc", "user-zprofile", "user-zshenv", "user-vimrc", "user-ghostty"} {
		for _, r := range resources {
			if r.Name == name {
				if err = m.installResource(r, true); err != nil {
					return err
				}
			}
		}
	}
	miseGlobal := envDefault("XDG_CONFIG_HOME", os.Getenv("HOME")+"/.config") + "/mise/config.toml"
	if info, present, e := exists(miseGlobal); e != nil {
		return e
	} else if present && !(info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("user mise config path is not a regular file or symlink: %s", miseGlobal)
	}
	for _, r := range resources {
		if r.Kind != "block" {
			if err = m.installResource(r, true); err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(c.Out, "Skipping package and tool installation.")
	for _, r := range resources {
		if r.Name == "zsh-interactive" && !dry {
			same := false
			source, e := os.ReadFile(r.Source)
			if e == nil {
				target, e := os.ReadFile(r.Target)
				same = e == nil && string(target) == string(source)
			}
			if !same {
				for _, name := range []string{"zoxide-init.zsh", "fzf-init.zsh", "starship-init.zsh"} {
					os.Remove(paths.Cache + "/" + name)
				}
			}
		}
		if err = m.installResource(r, false); err != nil {
			return err
		}
	}
	if !dry {
		c.trustMise()
	}
	if dry {
		if _, present, _ := exists(miseGlobal); present {
			fmt.Fprintf(c.Out, "user mise config exists; preserving it: %s\n", miseGlobal)
		} else {
			fmt.Fprintf(c.Out, "Would create user mise config: %s\n", miseGlobal)
		}
		c.defaultShell(dry, yes)
		if m.unchanged > 0 {
			fmt.Fprintf(c.Out, "%d items unchanged.\n", m.unchanged)
		}
		fmt.Fprintln(c.Out, "Dry run complete; no files were changed.")
		return nil
	}
	if _, present, _ := exists(miseGlobal); !present {
		if err = writeOnce(miseGlobal, nil); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Created user mise config: %s\n", miseGlobal)
	}
	c.defaultShell(dry, yes)
	if err = writeAtomic(paths.State+"/configured", []byte("1\n"), 0600); err != nil {
		return err
	}
	value := "0\n"
	if ghostty {
		value = "1\n"
	}
	if err = writeAtomic(paths.State+"/ghostty", []byte(value), 0600); err != nil {
		return err
	}
	if m.unchanged > 0 {
		fmt.Fprintf(c.Out, "%d items unchanged.\n", m.unchanged)
	}
	fmt.Fprintln(c.Out, "Selfishell configuration installed.")
	return nil
}
func writeOnce(path string, data []byte) error { return createRawOnce(path, data) }
func (c CLI) interactive() bool                { return IsTerminal(c.In) || os.Getenv("SELFISHELL_TEST_TTY") != "" }
func (c CLI) readAnswer() (string, error) {
	if c.In == nil {
		return "", io.EOF
	}
	var result []byte
	var b [1]byte
	for {
		n, e := c.In.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				return strings.TrimSuffix(string(result), "\r"), nil
			}
			result = append(result, b[0])
		}
		if e != nil {
			return string(result), e
		}
	}
}
func affirmative(s string) bool { return s == "y" || s == "Y" || s == "yes" || s == "YES" }
