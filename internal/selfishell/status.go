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

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/release"
	"github.com/jiminu/selfishell/internal/terminal"
)

func (c CLI) marker(color, label string) string {
	if progressColor(c.Out, c.Env) {
		return "\x1b[" + color + "m" + label + "\x1b[0m"
	}
	return label
}
func (c CLI) sayDiagnostic(color, label, message string) {
	fmt.Fprintf(c.Out, "%s %s\n", c.marker(color, "["+label+"]"), displayHome(message, c.Env.Get("HOME")))
}
func (c CLI) bold(value string) string {
	if progressColor(c.Out, c.Env) {
		return "\x1b[1m" + value + "\x1b[0m"
	}
	return value
}
func (c CLI) diagnosticError(err error) int {
	c.error(displayHome(err.Error(), c.Env.Get("HOME")))
	var invalid invalidPackageNameError
	if errors.As(err, &invalid) {
		return 2
	}
	return 1
}

func diagnosticPackages(root, platform string, env process.Environment) ([]Package, error) {
	all, err := ReadPackages(filepath.Join(root, "packages.conf"))
	if err != nil {
		return nil, err
	}
	paths, err := managed.UserPaths(env.Get)
	if err != nil {
		return nil, err
	}
	// terminal.WindowsStatus reports a malformed choice; diagnose without its fonts.
	choice, err := terminal.ReadWindowsChoice(paths)
	if err != nil {
		choice = &terminal.WindowsChoice{}
	}
	eligible, err := selectPackages(root, paths, all, platform, choice, env)
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
	version, err := release.Retained(releases, link)
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
func hasConfiguredMarker(paths managed.Paths) bool {
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
	paths, err := managed.UserPaths(c.Env.Get)
	if err != nil {
		return c.diagnosticError(err)
	}
	c.diagnosticHeader()
	count, intact := 0, 0
	changedPaths, recordIssues := false, false
	configured := hasConfiguredMarker(paths)
	platform := detectPlatform(c.Env)
	var packages []Package
	var inventory *ToolInventory
	if configured {
		c.sayDiagnostic("36", "INFO", "Selfishell configuration is installed.")
		if packages, err = diagnosticPackages(c.Root, platform.Name, c.Env); err != nil {
			return c.diagnosticError(err)
		}
		if inventory, err = NewToolInventory(c.Root, paths, c.Err, c.Env); err != nil {
			return c.diagnosticError(err)
		}
	}
	result, installHinted := c.statusSystem(platform, configured, verbose)
	resources, err := ManagedResources(c.Root, c.Env)
	if err != nil {
		return c.diagnosticError(err)
	}
	expected := map[string]bool{}
	if configured && platformSupported(platform.Name) {
		ghostty := false
		if platform.Name == "macos" {
			choice, e := managed.ReadFile(paths.State + "/ghostty")
			// Install and update reject this choice; status keeps diagnosing.
			if e != nil && !errors.Is(e, fs.ErrNotExist) {
				count++
				recordIssues = true
				c.sayDiagnostic("31", "MALFORMED", paths.State+"/ghostty")
				result = 1
			}
			ghostty = e == nil && string(choice) == "1\n"
		}
		selected, e := ResourcesForPlatform(c.Root, platform.Name, ghostty, c.Env)
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
		state, e := managed.ReadState(statePath)
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
			checksum, e := managed.Checksum(context.Background(), state.Target)
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
			data, e := managed.ReadFile(state.Target)
			if e == nil {
				view, ve := managed.InspectBlock(name, data)
				if ve == nil && view.Status == "intact" && view.Checksum == state.Checksum {
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
	if tracked, profileIntact, profileRecordIssue := terminal.WindowsStatus(paths, c.Env.Get("WSL_DISTRO_NAME"), verbose, c.sayDiagnostic); tracked {
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
		c.legacyZinitNote(paths)
	}
	return result
}

// statusTools reports missing required tools and failed tool inspections.
func (c CLI) statusTools(packages []Package, inventory *ToolInventory, platform Platform, verbose bool) (bool, error) {
	present, requiredMissing, optionalMissing, unknown := 0, 0, 0, 0
	for _, p := range packages {
		tool, e := inventory.Detect(p.Manager, p.Name, dependencyPlatform(platform.Name), platform.Arch)
		if e != nil {
			if tool.Installed != "unknown" {
				return false, e
			}
			unknown++
			c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Tool: %s could not be inspected (%s): %s", p.Name, p.Manager, e))
		}
		if tool.Installed == "missing" {
			c.missingTool(p)
			if p.Requirement == "required" {
				requiredMissing++
			} else {
				optionalMissing++
			}
		} else if tool.Installed != "unknown" {
			present++
		}
		if verbose {
			installed, approved := tool.Installed, tool.Approved
			if p.Manager == "direct" {
				installed, approved = shortRevision(installed), shortRevision(approved)
			}
			fmt.Fprintf(c.Out, "[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n", p.Name, installed, tool.Source, approved)
		}
	}
	c.toolsSummary(present, requiredMissing, optionalMissing, unknown)
	return requiredMissing+unknown > 0, nil
}

// shortRevision abbreviates a full Git commit (a git direct package pin) for display.
func shortRevision(version string) string {
	if len(version) == 40 && strings.Trim(version, "0123456789abcdef") == "" {
		return version[:12]
	}
	return version
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

func (c CLI) toolsSummary(present, requiredMissing, optionalMissing, unknown int) {
	message := fmt.Sprintf("Tools: %d present", present)
	color, label := "32", "OK"
	if unknown > 0 {
		color, label = "31", "ERROR"
		message += fmt.Sprintf(", %d unknown", unknown)
	}
	if requiredMissing > 0 {
		color, label = "31", "ERROR"
		message += fmt.Sprintf(", %d required missing", requiredMissing)
	}
	if optionalMissing > 0 {
		if requiredMissing+unknown == 0 {
			color, label = "36", "INFO"
		}
		message += fmt.Sprintf(", %d optional not installed", optionalMissing)
	}
	c.sayDiagnostic(color, label, message)
	if unknown > 0 {
		c.diagnosticHint("Resolve the reported inspection errors, then rerun selfishell status.", "")
	}
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
