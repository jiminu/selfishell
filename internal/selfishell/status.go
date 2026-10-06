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
	if progressColor(c.Out) {
		return "\x1b[" + color + "m" + label + "\x1b[0m"
	}
	return label
}
func (c CLI) sayDiagnostic(color, label, message string) {
	fmt.Fprintf(c.Out, "%s %s\n", c.marker(color, "["+label+"]"), displayHome(message, os.Getenv("HOME")))
}
func (c CLI) bold(value string) string {
	if progressColor(c.Out) {
		return "\x1b[1m" + value + "\x1b[0m"
	}
	return value
}
func (c CLI) diagnosticError(err error) int {
	c.error(displayHome(err.Error(), os.Getenv("HOME")))
	var invalid invalidPackageNameError
	if errors.As(err, &invalid) {
		return 2
	}
	return 1
}

func diagnosticPackages(root, platform string) ([]Package, error) {
	all, err := ReadPackages(filepath.Join(root, "packages.conf"))
	if err != nil {
		return nil, err
	}
	paths, err := UserPaths()
	if err != nil {
		return nil, err
	}
	// statusWindowsTerminal reports a malformed choice; diagnose without its fonts.
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil {
		choice = &windowsTerminalChoice{}
	}
	eligible, err := selectPackages(root, paths, all, platform, choice)
	if err != nil {
		return nil, err
	}
	selected := make([]Package, 0, len(eligible))
	seen := map[string]bool{}
	for _, p := range eligible {
		if !seen[p.Name] {
			selected = append(selected, p)
			seen[p.Name] = true
		}
	}
	return selected, nil
}

func packageMatches(p Package, platform string) bool {
	return p.Platform == "all" || p.Platform == platform || p.Platform == packagePlatform(platform)
}

// packagePlatform maps a detected platform to its packages.conf platform.
func packagePlatform(platform string) string {
	if platform == "ubuntu-wsl" {
		return "ubuntu"
	}
	return platform
}

// dependencyPlatform maps a detected platform to its dependencies.conf platform.
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
	link, err := os.Readlink(filepath.Join(filepath.Dir(releases), "previous"))
	if os.IsNotExist(err) {
		return "none"
	}
	if err != nil {
		return "invalid"
	}
	version, err := retainedRelease(releases, link)
	if err != nil {
		return "invalid"
	}
	return version
}

func (c CLI) diagnosticHeader() {
	version := "unknown"
	if data, e := os.ReadFile(filepath.Join(c.Root, "VERSION")); e == nil {
		version = strings.TrimRight(strings.ReplaceAll(string(data), "\x00", ""), "\n")
	}
	fmt.Fprintf(c.Out, "[CLI] Current: %s | Rollback: %s\n", version, rollbackStatusVersion(c.Root))
}

// hasConfiguredMarker reports completed setup for diagnostics; only a regular
// file counts, so a replaced marker path is not mistaken for setup.
func hasConfiguredMarker(paths Paths) bool {
	info, err := os.Stat(paths.State + "/configured")
	return err == nil && info.Mode().IsRegular()
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
	c.diagnosticHeader()
	count, intact := 0, 0
	changedPaths, recordIssues := false, false
	configured := hasConfiguredMarker(paths)
	platform := DetectPlatform()
	var packages []Package
	var inventory *ToolInventory
	if configured {
		c.sayDiagnostic("36", "INFO", "Selfishell configuration is installed.")
		if packages, err = diagnosticPackages(c.Root, platform.Name); err != nil {
			return c.diagnosticError(err)
		}
		if inventory, err = NewToolInventory(c.Root, paths, c.Err); err != nil {
			return c.diagnosticError(err)
		}
	}
	result, installHinted := c.statusSystem(platform, configured, verbose)
	resources, err := ManagedResources(c.Root)
	if err != nil {
		return c.diagnosticError(err)
	}
	expected := map[string]bool{}
	if configured && platformSupported(platform.Name) {
		ghostty := false
		if platform.Name == "macos" {
			choice, e := readStateFile(paths.State + "/ghostty")
			// Install and update reject this choice; status keeps diagnosing.
			if e != nil && !errors.Is(e, fs.ErrNotExist) {
				count++
				recordIssues = true
				c.sayDiagnostic("31", "MALFORMED", paths.State+"/ghostty")
				result = 1
			}
			ghostty = e == nil && string(choice) == "1\n"
		}
		selected, e := ResourcesForPlatform(c.Root, platform.Name, ghostty)
		if e != nil {
			return c.diagnosticError(e)
		}
		for _, r := range selected {
			expected[r.Name] = true
		}
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
			if expected[name] {
				count++
				recordIssues = true
				c.sayDiagnostic("31", "MISSING", "Installation record: "+statePath)
				result = 1
			}
			continue
		}
		count++
		if e != nil {
			recordIssues = true
			c.sayDiagnostic("31", "MALFORMED", statePath)
			result = 1
			continue
		}
		if state.Status != "active" {
			recordIssues = true
			c.sayDiagnostic("33", "PENDING", state.Target)
			result = 1
			continue
		}
		switch state.Kind {
		case "link":
			target, e := os.Readlink(state.Target)
			if e == nil && target == state.Reference {
				intact++
				if verbose {
					c.sayDiagnostic("32", "OK", state.Target+" -> "+state.Reference)
				}
			} else {
				changedPaths = true
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
			}
		case "file":
			checksum, e := Checksum(context.Background(), state.Target)
			if e == nil && checksum == state.Checksum {
				intact++
				if verbose {
					c.sayDiagnostic("32", "OK", state.Target)
				}
			} else {
				changedPaths = true
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
			}
		case "block":
			label := blockLabel(name)
			if label == "" {
				changedPaths = true
				c.sayDiagnostic("33", "CHANGED", state.Target)
				result = 1
				continue
			}
			suffix := state.Target + " (" + label + ")"
			data, e := readStateFile(state.Target)
			if e == nil {
				view, ve := inspectBlock(name, data)
				if ve == nil && view.status == "intact" && view.checksum == state.Checksum {
					intact++
					if verbose {
						c.sayDiagnostic("32", "OK", suffix)
					}
					continue
				}
			}
			changedPaths = true
			c.sayDiagnostic("33", "CHANGED", suffix)
			result = 1
		}
	}
	if tracked, profileIntact, profileRecordIssue := c.statusWindowsTerminal(paths, verbose); tracked {
		count++
		if profileIntact {
			intact++
		} else {
			result = 1
			recordIssues = recordIssues || profileRecordIssue
			changedPaths = changedPaths || !profileRecordIssue
		}
	}
	if count == 0 {
		fmt.Fprintln(c.Out, "Selfishell configuration is not installed.")
		if !installHinted {
			c.diagnosticHint("Set up the Selfishell environment with:", "selfishell install")
		}
		return 1
	}
	if intact == count {
		c.sayDiagnostic("32", "OK", fmt.Sprintf("Configuration: %d paths intact", intact))
	} else {
		c.sayDiagnostic("33", "WARN", fmt.Sprintf("Configuration: %d intact, %d issues", intact, count-intact))
	}
	if recordIssues {
		c.diagnosticHint("Review installation records and existing backups before reinstalling.", "")
	}
	if changedPaths {
		c.diagnosticHint("Review changed configuration before synchronizing with:", "selfishell update --tools-only --skip-packages")
	}
	if configured {
		missing, e := c.statusTools(packages, inventory, platform, verbose)
		if e != nil {
			return c.diagnosticError(e)
		}
		if missing {
			result = 1
		}
		if c.statusPlugins(inventory.dependencies) {
			result = 1
		}
	}
	return result
}

// statusTools reports whether a required tool is missing.
func (c CLI) statusTools(packages []Package, inventory *ToolInventory, platform Platform, verbose bool) (bool, error) {
	present, requiredMissing, optionalMissing := 0, 0, 0
	for _, p := range packages {
		tool, e := inventory.Detect(p.Manager, p.Name, dependencyPlatform(platform.Name), platform.Arch)
		if e != nil {
			return false, e
		}
		if tool.Installed == "missing" {
			c.missingTool(p)
			if p.Requirement == "required" {
				requiredMissing++
			} else {
				optionalMissing++
			}
		} else {
			present++
		}
		if verbose {
			fmt.Fprintf(c.Out, "[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n", p.Name, tool.Installed, tool.Source, tool.Approved)
		}
	}
	c.toolsSummary(present, requiredMissing, optionalMissing)
	return requiredMissing > 0, nil
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

func (c CLI) missingTool(p Package) {
	if p.Requirement == "required" {
		c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Tool: %s is missing (%s)", p.Name, p.Manager))
	} else {
		c.sayDiagnostic("36", "INFO", fmt.Sprintf("Optional tool: %s is not installed (%s)", p.Name, p.Manager))
	}
}

func (c CLI) toolsSummary(present, requiredMissing, optionalMissing int) {
	message := fmt.Sprintf("Tools: %d present", present)
	color, label := "32", "OK"
	if requiredMissing > 0 {
		color, label = "31", "ERROR"
		message += fmt.Sprintf(", %d required missing", requiredMissing)
	}
	if optionalMissing > 0 {
		if requiredMissing == 0 {
			color, label = "36", "INFO"
		}
		message += fmt.Sprintf(", %d optional not installed", optionalMissing)
	}
	c.sayDiagnostic(color, label, message)
	if requiredMissing+optionalMissing > 0 {
		c.diagnosticHint("Synchronize missing tools with:", "selfishell update --tools-only")
	}
}

func (c CLI) diagnosticHint(message, command string) {
	c.sayDiagnostic("36", "INFO", message)
	if command != "" {
		fmt.Fprintf(c.Out, "       %s\n", c.bold(command))
	}
}

// displayHome changes presentation only, preserving literal paths for operations.
func displayHome(message, home string) string {
	prefix := strings.TrimRight(home, "/") + "/"
	if prefix == "/" {
		return message
	}
	var out strings.Builder
	offset := 0
	for {
		index := strings.Index(message[offset:], prefix)
		if index < 0 {
			out.WriteString(message[offset:])
			return out.String()
		}
		index += offset
		out.WriteString(message[offset:index])
		if index == 0 || strings.ContainsRune(" \t\r\n\"'(", rune(message[index-1])) {
			out.WriteString("~/")
		} else {
			out.WriteString(prefix)
		}
		offset = index + len(prefix)
	}
}
