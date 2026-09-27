package selfishell

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (c CLI) trustMise() {
	link := envDefault("XDG_CONFIG_HOME", os.Getenv("HOME")+"/.config") + "/mise/conf.d/selfishell.toml"
	info, present, _ := exists(link)
	if !present || !(info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return
	}
	binary, e := exec.LookPath("mise")
	if e != nil {
		binary = os.Getenv("HOME") + "/.local/bin/mise"
		info, e = os.Stat(binary)
		if e != nil || info.Mode()&0111 == 0 {
			return
		}
	}
	(Process{Dir: c.Root + "/config/shared"}).Run(context.Background(), binary, "trust", link)
}
func (c CLI) defaultShell(dry, yes bool) {
	if filepath.Base(os.Getenv("SHELL")) == "zsh" {
		return
	}
	file := envDefault("SELFISHELL_TEST_SHELLS_FILE", "/etc/shells")
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
		if _, e := exec.LookPath("zsh"); e == nil {
			fmt.Fprintln(c.Out, "Zsh is not listed in /etc/shells; the login shell was not changed.")
		}
		return
	}
	if dry {
		fmt.Fprintf(c.Out, "Would set login shell to: %s\n", selected)
		return
	}
	if !yes {
		if !c.interactive() {
			return
		}
		fmt.Fprint(c.Out, "Set login shell to Zsh? [Y/n] ")
		answer, _ := c.readAnswer()
		if answer == "n" || answer == "N" || answer == "no" || answer == "NO" {
			return
		}
	}
	terminal := envDefault("SELFISHELL_TEST_TERMINAL", "/dev/tty")
	tty, e := os.Open(terminal)
	if e != nil {
		fmt.Fprintf(c.Out, "To use Zsh as your login shell, run: chsh -s %s\n", selected)
		return
	}
	defer tty.Close()
	var userOut strings.Builder
	code, e := (Process{Out: &userOut}).Run(context.Background(), "id", "-un")
	if e != nil || code != 0 {
		return
	}
	code, e = (Process{In: tty, Out: c.Out, Err: c.Err}).Run(context.Background(), "chsh", "-s", selected, strings.TrimSpace(userOut.String()))
	if e == nil && code == 0 {
		fmt.Fprintf(c.Out, "Set login shell to: %s\n", selected)
	} else {
		fmt.Fprintln(c.Out, "Could not set login shell to Zsh.")
	}
}
