package selfishell

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// trustMise keeps the caller's environment, unlike InstallMise's release-scoped process.
func (c CLI) trustMise(paths Paths) {
	binary, e := (Process{inherit: c.Env}).lookPath("mise")
	if e != nil {
		binary = c.Env.Get("HOME") + "/.local/bin/mise"
		info, e := os.Stat(binary)
		if e != nil || info.Mode()&0111 == 0 {
			return
		}
	}
	trustManagedMiseConfig(c.invocationContext(), Process{Dir: c.Root + "/config/shared", inherit: c.Env}, binary, paths)
}
func (c CLI) defaultShell(dry, yes bool) {
	c.progress.Pause()
	if filepath.Base(c.Env.Get("SHELL")) == "zsh" {
		return
	}
	file := c.Env.Default("SELFISHELL_TEST_SHELLS_FILE", "/etc/shells")
	f, e := os.Open(file)
	if e != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	selected := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "/") && filepath.Base(line) == "zsh" {
			info, e := os.Stat(line)
			if e == nil && info.Mode()&0111 != 0 {
				selected = line
				break
			}
		}
	}
	if selected == "" {
		if _, e := (Process{inherit: c.Env}).lookPath("zsh"); e == nil {
			c.report("Notes", reportWarning, "Zsh is not listed in /etc/shells; the login shell was not changed.")
		}
		return
	}
	if dry {
		c.report("Configuration", reportPreview, "Would set login shell to: %s", selected)
		return
	}
	if !yes {
		if !c.interactive() {
			return
		}
		fmt.Fprint(c.Out, "Set login shell to Zsh? [Y/n] ")
		answer, _ := c.readAnswer()
		if negative(answer) {
			return
		}
	}
	terminal := c.Env.Default("SELFISHELL_TEST_TERMINAL", "/dev/tty")
	tty, e := os.Open(terminal)
	if e != nil {
		c.report("Notes", reportInfo, "To use Zsh as your login shell, run: chsh -s %s", selected)
		return
	}
	defer tty.Close()
	var userOut strings.Builder
	code, e := (Process{Out: &userOut, inherit: c.Env}).Run(context.Background(), "id", "-un")
	if e != nil || code != 0 {
		return
	}
	code, e = (Process{In: tty, Out: c.Out, Err: c.Err, inherit: c.Env}).Run(context.Background(), "chsh", "-s", selected, strings.TrimSpace(userOut.String()))
	if e == nil && code == 0 {
		c.report("Configuration", reportSuccess, "Set login shell to: %s", selected)
	} else {
		c.report("Notes", reportWarning, "Could not set login shell to Zsh.")
	}
}
