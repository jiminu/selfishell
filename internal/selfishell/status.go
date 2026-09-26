package selfishell

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (c CLI) marker(color, label string) string {
	if os.Getenv("NO_COLOR") == "" && IsTerminal(c.Out) {
		return "\x1b[" + color + "m" + label + "\x1b[0m"
	}
	return label
}
func (c CLI) sayDiagnostic(color, label, message string) {
	fmt.Fprintf(c.Out, "%s %s\n", c.marker(color, "["+label+"]"), message)
}
func (c CLI) bold(value string) string {
	if os.Getenv("NO_COLOR") == "" && IsTerminal(c.Out) {
		return "\x1b[1m" + value + "\x1b[0m"
	}
	return value
}
func (c CLI) diagnosticError(err error) int { c.error(err.Error()); return 1 }

func diagnosticPackages(root, platform string) ([]Package, error) {
	all, err := ReadPackages(filepath.Join(root, "packages.conf"))
	if err != nil {
		return nil, err
	}
	selected := make([]Package, 0, len(all))
	seen := map[string]bool{}
	if platform == "ubuntu-wsl" {
		platform = "ubuntu"
	}
	for _, p := range all {
		if (p.Platform == "all" || p.Platform == platform) && !seen[p.Name] {
			selected = append(selected, p)
			seen[p.Name] = true
		}
	}
	return selected, nil
}
func dependencyPlatform(platform string) string {
	if platform == "ubuntu" || platform == "ubuntu-wsl" {
		return "linux"
	}
	return platform
}

func rollbackStatusVersion(root string) string {
	releases := filepath.Dir(root)
	if filepath.Base(releases) != "releases" {
		return "none"
	}
	previous := filepath.Join(filepath.Dir(releases), "previous")
	link, err := os.Readlink(previous)
	if err != nil {
		return "none"
	}
	version := filepath.Base(link)
	release := filepath.Join(releases, version)
	info, err := os.Lstat(release)
	if err != nil || !info.IsDir() {
		return "invalid"
	}
	data, err := os.ReadFile(filepath.Join(release, "VERSION"))
	if err != nil || strings.TrimRight(strings.ReplaceAll(string(data), "\x00", ""), "\n") != version {
		return "invalid"
	}
	cli, err := os.Stat(filepath.Join(release, "bin/selfishell"))
	if err != nil || cli.Mode().Perm()&0111 == 0 {
		return "invalid"
	}
	return version
}

func (c CLI) status(args []string) int {
	verbose := false
	for _, arg := range args {
		switch arg {
		case "--verbose":
			verbose = true
		case "help", "--help", "-h":
			fmt.Fprintln(c.Out, "Usage: selfishell status [--verbose]")
			return 0
		default:
			c.error("Unknown status option: " + arg)
			return 2
		}
	}
	paths, err := UserPaths()
	if err != nil {
		return c.diagnosticError(err)
	}
	version := "unknown"
	if data, e := os.ReadFile(filepath.Join(c.Root, "VERSION")); e == nil {
		version = strings.TrimRight(strings.ReplaceAll(string(data), "\x00", ""), "\n")
	}
	fmt.Fprintf(c.Out, "[CLI] Current: %s | Rollback: %s\n", version, rollbackStatusVersion(c.Root))
	result, present, missing, count := 0, 0, 0, 0
	platform := DetectPlatform()
	if info, e := os.Stat(paths.State + "/configured"); e == nil && info.Mode().IsRegular() {
		c.sayDiagnostic("36", "INFO", "Selfishell configuration is installed.")
		packages, e := diagnosticPackages(c.Root, platform.Name)
		if e != nil {
			return c.diagnosticError(e)
		}
		inventory, e := NewToolInventory(c.Root, paths, c.Err)
		if e != nil {
			return c.diagnosticError(e)
		}
		for _, p := range packages {
			tool, e := inventory.Detect(p.Manager, p.Name, dependencyPlatform(platform.Name), platform.Arch)
			if e != nil {
				return c.diagnosticError(e)
			}
			if tool.Installed == "missing" {
				missing++
				if p.Requirement == "required" {
					result = 1
				}
			} else {
				present++
			}
			if verbose {
				fmt.Fprintf(c.Out, "[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n", p.Name, tool.Installed, tool.Source, tool.Approved)
			}
		}
	}
	resources, err := ManagedResources(c.Root)
	if err != nil {
		return c.diagnosticError(err)
	}
	names := make([]string, 0, len(resources))
	known := map[string]bool{}
	for _, r := range resources {
		names = append(names, r.Name)
		known[r.Name] = true
	}
	entries, e := os.ReadDir(paths.Resources)
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return c.diagnosticError(e)
	}
	var extra []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".state") {
			name = strings.TrimSuffix(name, ".state")
			if name != "" && !known[name] {
				extra = append(extra, name)
				known[name] = true
			}
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)
	for _, name := range names {
		statePath := paths.Resources + "/" + name + ".state"
		state, e := ReadState(statePath)
		if errors.Is(e, fs.ErrNotExist) {
			continue
		}
		count++
		if e != nil {
			c.sayDiagnostic("31", "MALFORMED", statePath)
			result = 1
			continue
		}
		if state.Status != "active" {
			c.sayDiagnostic("33", "PENDING", state.Target)
			result = 1
			continue
		}
		switch state.Kind {
		case "link":
			target, e := os.Readlink(state.Target)
			if e == nil && target == state.Reference {
				c.sayDiagnostic("32", "OK", state.Target+" -> "+state.Reference)
			} else {
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
			}
		case "file":
			checksum, e := Checksum(context.Background(), state.Target)
			if e == nil && checksum == state.Checksum {
				c.sayDiagnostic("32", "OK", state.Target)
			} else {
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
			}
		case "block":
			label := blockLabel(name)
			if label == "" {
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
				continue
			}
			suffix := state.Target + " (" + label + ")"
			data, e := os.ReadFile(state.Target)
			info, le := os.Lstat(state.Target)
			if e == nil && le == nil && info.Mode().IsRegular() {
				view, ve := inspectBlock(name, data)
				if ve == nil && view.status == "intact" && view.checksum == state.Checksum {
					c.sayDiagnostic("32", "OK", suffix)
					continue
				}
			}
			c.sayDiagnostic("33", "CHANGED", suffix)
			result = 1
		}
	}
	if count == 0 {
		fmt.Fprintln(c.Out, "Selfishell configuration is not installed.")
		return 1
	}
	if !verbose {
		c.sayDiagnostic("36", "SUMMARY", fmt.Sprintf("Managed paths: %d | Tools: %d present, %d missing", count, present, missing))
	}
	return result
}
func blockLabel(name string) string {
	switch name {
	case "user-zshrc":
		return "Selfishell initialize"
	case "user-zprofile":
		return "Selfishell mise shims"
	case "user-zshenv":
		return "Selfishell zshenv"
	case "user-vimrc":
		return "Selfishell vimrc"
	case "user-ghostty":
		return "Selfishell ghostty"
	}
	return ""
}
