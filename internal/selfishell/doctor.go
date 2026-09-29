package selfishell

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func platformLabel(name string) string {
	switch name {
	case "macos":
		return "macOS"
	case "ubuntu":
		return "Ubuntu"
	case "ubuntu-wsl":
		return "Ubuntu on WSL"
	case "unsupported-wsl":
		return "Unsupported WSL distribution"
	case "unsupported-linux":
		return "Unsupported Linux distribution"
	}
	return "Unsupported operating system"
}
func platformSupported(name string) bool {
	return name == "macos" || name == "ubuntu" || name == "ubuntu-wsl"
}
func commandExists(name string) bool { _, err := exec.LookPath(name); return err == nil }
func (c CLI) doctor(args []string) int {
	verbose := false
	for _, arg := range args {
		switch arg {
		case "--verbose":
			verbose = true
		case "help", "--help", "-h":
			fmt.Fprintln(c.Out, "Usage: selfishell doctor [--verbose]")
			return 0
		default:
			c.error("Unknown doctor option: " + arg)
			return 2
		}
	}
	platform := DetectPlatform()
	paths, err := UserPaths()
	if err != nil {
		return c.diagnosticError(err)
	}
	c.diagnosticHeader()
	result := 0
	system := diagnosticGroup{c: c, verbose: verbose}
	_, configuredErr := os.Stat(paths.State + "/configured")
	configured := configuredErr == nil
	if platformSupported(platform.Name) {
		if configured {
			c.sayDiagnostic("36", "INFO", "Selfishell configuration is installed.")
		}
		system.say("32", "OK", "Platform: "+platformLabel(platform.Name))
	} else {
		system.say("31", "ERROR", "Platform: "+platformLabel(platform.Name))
		switch platform.Name {
		case "unsupported-wsl":
			fmt.Fprintln(c.Out, "        Only Ubuntu on WSL is currently supported.")
		case "unsupported-linux":
			fmt.Fprintln(c.Out, "        Ubuntu is the only supported native Linux distribution.")
		default:
			fmt.Fprintln(c.Out, "        Use macOS, Ubuntu, or Ubuntu on WSL.")
		}
		result = 1
	}
	if platform.Arch == "amd64" || platform.Arch == "arm64" {
		system.say("32", "OK", "Architecture: "+platform.Arch)
	} else {
		system.say("31", "ERROR", "Architecture: "+platform.Arch+" (supported: amd64, arm64)")
		result = 1
	}
	manager := "unknown"
	if platform.Name == "macos" {
		manager = "brew"
	} else if platform.Name == "ubuntu" || platform.Name == "ubuntu-wsl" {
		manager = "apt-get"
	}
	if manager == "unknown" {
		system.say("31", "ERROR", "Package manager: unavailable for this platform")
	} else if commandExists(manager) {
		system.say("32", "OK", "Package manager: "+manager)
	} else {
		system.say("31", "ERROR", "Package manager: "+manager+" was not found")
		fmt.Fprintf(c.Out, "        Run '%s' to set up the supported toolchain.\n", c.bold("selfishell install"))
		result = 1
	}
	if !configured || !platformSupported(platform.Name) {
		system.summary(platform)
		return result
	}
	if platform.Name == "macos" {
		_, _, ok := runInventory("", nil, "xcode-select", "-p")
		if !ok {
			system.say("31", "ERROR", "C compiler: Xcode Command Line Tools are not installed (required for compiling Tree-sitter parsers)")
			fmt.Fprintf(c.Out, "        Install them by running: %s\n", c.bold("xcode-select --install"))
			result = 1
		} else {
			result = c.doctorCompiler(platform.Name, result, &system)
		}
	} else {
		result = c.doctorCompiler(platform.Name, result, &system)
	}
	system.summary(platform)
	packages, err := diagnosticPackages(c.Root, platform.Name)
	if err != nil {
		return c.diagnosticError(err)
	}
	inventory, err := NewToolInventory(c.Root, paths, c.Err)
	if err != nil {
		return c.diagnosticError(err)
	}
	present, requiredMissing, optionalMissing := 0, 0, 0
	for _, p := range packages {
		tool, e := inventory.Detect(p.Manager, p.Name, dependencyPlatform(platform.Name), platform.Arch)
		if e != nil {
			return c.diagnosticError(e)
		}
		if tool.Installed == "missing" {
			c.missingTool(p)
			if p.Requirement == "required" {
				requiredMissing++
				result = 1
			} else {
				optionalMissing++
			}
		} else {
			present++
			if verbose {
				c.sayDiagnostic("32", "OK", fmt.Sprintf("Tool: %s %s (%s)", p.Name, tool.Installed, tool.Source))
			}
		}
	}
	c.toolsSummary(present, requiredMissing, optionalMissing)
	if err := c.doctorPlugins(paths, inventory.dependencies); err {
		result = 1
	}
	return result
}
func (c CLI) doctorCompiler(platform string, result int, system *diagnosticGroup) int {
	// A compiler counts only if it runs; a failed candidate falls through to the next.
	var broken []string
	for _, name := range []string{"gcc", "clang"} {
		if !commandExists(name) {
			continue
		}
		out, _, ok := runInventory("", nil, name, "--version")
		if !ok {
			broken = append(broken, name)
			continue
		}
		first, _, _ := strings.Cut(out, "\n")
		system.say("32", "OK", fmt.Sprintf("C compiler: %s (%s)", name, first))
		return result
	}
	if len(broken) != 0 {
		system.say("31", "ERROR", "C compiler: "+strings.Join(broken, " and ")+" failed to run (required for compiling Tree-sitter parsers)")
		fmt.Fprintf(c.Out, "        Run '%s' to see the error.\n", c.bold(broken[0]+" --version"))
		return 1
	}
	system.say("31", "ERROR", "C compiler: gcc or clang was not found (required for compiling Tree-sitter parsers)")
	if platform == "macos" {
		fmt.Fprintf(c.Out, "        Install Xcode Command Line Tools by running: %s\n", c.bold("xcode-select --install"))
	} else {
		fmt.Fprintf(c.Out, "        Install build tools by running: %s\n", c.bold("sudo apt install build-essential"))
	}
	return 1
}
func (c CLI) doctorPlugins(paths Paths, dependencies []Dependency) bool {
	dataHome := envDefault("XDG_DATA_HOME", os.Getenv("HOME")+"/.local/share")
	zinit, err := os.Stat(dataHome + "/zinit/zinit.git/zinit.zsh")
	if err != nil || zinit.Size() == 0 {
		return false
	}
	var missing, dirty, drifted []string
	for _, d := range dependencies {
		if d.Kind != "zsh-plugin" {
			continue
		}
		dir := dataHome + "/zinit/plugins/" + strings.ReplaceAll(d.Name, "/", "---")
		gitDir, err := os.Stat(dir + "/.git")
		if err != nil || !gitDir.IsDir() {
			missing = append(missing, d.Name)
			continue
		}
		changes, _, ok := runInventory("", append(os.Environ(), "GIT_OPTIONAL_LOCKS=0"), "git", "-C", dir, "status", "--porcelain")
		if !ok {
			drifted = append(drifted, d.Name)
			continue
		}
		if changes != "" {
			dirty = append(dirty, d.Name)
			continue
		}
		head, err := inventoryGitHead(dir)
		if err != nil || head != d.Version {
			drifted = append(drifted, d.Name)
		}
	}
	if len(missing) > 0 {
		c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Zsh plugins: %d not provisioned (%s)", len(missing), strings.Join(missing, " ")))
		fmt.Fprintf(c.Out, "        Run '%s' to provision them; shell startup never downloads plugins.\n", c.bold("selfishell install"))
		return true
	}
	reported := false
	if len(dirty) > 0 {
		c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Zsh plugins: %d modified locally (%s)", len(dirty), strings.Join(dirty, " ")))
		reported = true
	}
	if len(drifted) > 0 {
		c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Zsh plugins: %d at an unapproved revision (%s)", len(drifted), strings.Join(drifted, " ")))
		reported = true
	}
	if reported {
		c.diagnosticHint("Restore the approved Zsh plugin revisions with:", "selfishell update --tools-only")
	} else {
		c.sayDiagnostic("32", "OK", "Zsh plugins: provisioned")
	}
	return reported
}

// diagnosticGroup keeps successful system checks concise without hiding failures.
type diagnosticGroup struct {
	c              CLI
	verbose        bool
	passed, failed int
}

func (d *diagnosticGroup) say(color, label, message string) {
	if label == "OK" {
		d.passed++
		if !d.verbose {
			return
		}
	} else {
		d.failed++
	}
	d.c.sayDiagnostic(color, label, message)
}

func (d *diagnosticGroup) summary(platform Platform) {
	message := fmt.Sprintf("System: %s (%s), %d checks passed", platformLabel(platform.Name), platform.Arch, d.passed)
	if d.failed == 0 {
		d.c.sayDiagnostic("32", "OK", message)
	} else {
		d.c.sayDiagnostic("31", "ERROR", fmt.Sprintf("%s, %d failed", message, d.failed))
	}
}
