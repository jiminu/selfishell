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
	if len(args) > 0 {
		c.error("doctor does not accept arguments")
		return 2
	}
	platform := DetectPlatform()
	paths, err := UserPaths()
	if err != nil {
		return c.diagnosticError(err)
	}
	fmt.Fprint(c.Out, "Selfishell doctor\n\n")
	result := 0
	_, configuredErr := os.Stat(paths.State + "/configured")
	configured := configuredErr == nil
	if platformSupported(platform.Name) {
		if configured {
			c.sayDiagnostic("36", "INFO", "Selfishell configuration is installed.")
		}
		c.sayDiagnostic("32", "OK", "Platform: "+platformLabel(platform.Name))
	} else {
		c.sayDiagnostic("31", "ERROR", "Platform: "+platformLabel(platform.Name))
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
		c.sayDiagnostic("32", "OK", "Architecture: "+platform.Arch)
	} else {
		c.sayDiagnostic("31", "ERROR", "Architecture: "+platform.Arch+" (supported: amd64, arm64)")
		result = 1
	}
	manager := "unknown"
	if platform.Name == "macos" {
		manager = "brew"
	} else if platform.Name == "ubuntu" || platform.Name == "ubuntu-wsl" {
		manager = "apt-get"
	}
	if manager == "unknown" {
		c.sayDiagnostic("31", "ERROR", "Package manager: unavailable for this platform")
	} else if commandExists(manager) {
		c.sayDiagnostic("32", "OK", "Package manager: "+manager)
	} else {
		c.sayDiagnostic("31", "ERROR", "Package manager: "+manager+" was not found")
		fmt.Fprintf(c.Out, "        Run '%s' to set up the supported toolchain.\n", c.bold("selfishell install"))
		result = 1
	}
	if !configured || !platformSupported(platform.Name) {
		return result
	}
	if platform.Name == "macos" {
		_, _, ok := runInventory("", nil, "xcode-select", "-p")
		if !ok {
			c.sayDiagnostic("31", "ERROR", "C compiler: Xcode Command Line Tools are not installed (required for compiling Tree-sitter parsers)")
			fmt.Fprintf(c.Out, "        Install them by running: %s\n", c.bold("xcode-select --install"))
			result = 1
		} else {
			result = c.doctorCompiler(platform.Name, result)
		}
	} else {
		result = c.doctorCompiler(platform.Name, result)
	}
	packages, err := diagnosticPackages(c.Root, platform.Name)
	if err != nil {
		return c.diagnosticError(err)
	}
	inventory, err := NewToolInventory(c.Root, paths, c.Err)
	if err != nil {
		return c.diagnosticError(err)
	}
	for _, p := range packages {
		tool, e := inventory.Detect(p.Manager, p.Name, dependencyPlatform(platform.Name), platform.Arch)
		if e != nil {
			return c.diagnosticError(e)
		}
		if tool.Installed == "missing" {
			if p.Requirement == "required" {
				c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Tool: %s is missing (%s)", p.Name, p.Manager))
				result = 1
			} else {
				c.sayDiagnostic("36", "INFO", fmt.Sprintf("Optional tool: %s is not installed (%s)", p.Name, p.Manager))
			}
		} else {
			c.sayDiagnostic("32", "OK", fmt.Sprintf("Tool: %s %s (%s)", p.Name, tool.Installed, tool.Source))
		}
	}
	if err := c.doctorPlugins(paths, inventory.dependencies); err {
		result = 1
	}
	return result
}
func (c CLI) doctorCompiler(platform string, result int) int {
	for _, name := range []string{"gcc", "clang"} {
		if commandExists(name) {
			out, _, ok := runInventory("", nil, name, "--version")
			if !ok {
				out = ""
			}
			first, _, _ := strings.Cut(out, "\n")
			c.sayDiagnostic("32", "OK", fmt.Sprintf("C compiler: %s (%s)", name, first))
			return result
		}
	}
	c.sayDiagnostic("31", "ERROR", "C compiler: gcc or clang was not found (required for compiling Tree-sitter parsers)")
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
		if ok && changes != "" {
			dirty = append(dirty, d.Name)
			continue
		}
		head, _, ok := runInventory("", append(os.Environ(), "GIT_OPTIONAL_LOCKS=0"), "git", "-C", dir, "rev-parse", "HEAD")
		if !ok || strings.TrimSpace(head) != d.Version {
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
		fmt.Fprintf(c.Out, "        Run '%s' to reset it to the approved revision.\n", c.bold("selfishell update --tools-only"))
		reported = true
	}
	if len(drifted) > 0 {
		c.sayDiagnostic("31", "ERROR", fmt.Sprintf("Zsh plugins: %d at an unapproved revision (%s)", len(drifted), strings.Join(drifted, " ")))
		fmt.Fprintf(c.Out, "        Run '%s' to reset it to the approved revision.\n", c.bold("selfishell update --tools-only"))
		reported = true
	}
	if !reported {
		c.sayDiagnostic("32", "OK", "Zsh plugins: provisioned")
	}
	return reported
}
