package selfishell

import (
	"fmt"
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
func commandExists(name string, env Environment) bool {
	_, err := (Process{inherit: env}).lookPath(name)
	return err == nil
}

// statusSystem checks setup prerequisites; the compiler is checked only after setup.
// It also reports whether it already suggested 'selfishell install'.
func (c CLI) statusSystem(platform Platform, configured, verbose bool) (int, bool) {
	result, installHinted := 0, false
	system := diagnosticGroup{c: c, verbose: verbose}
	if platformSupported(platform.Name) {
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
	} else if commandExists(manager, c.Env) {
		system.say("32", "OK", "Package manager: "+manager)
	} else {
		system.say("31", "ERROR", "Package manager: "+manager+" was not found")
		fmt.Fprintf(c.Out, "        Run '%s' to set up the supported toolchain.\n", c.bold("selfishell install"))
		result, installHinted = 1, true
	}
	if !configured || !platformSupported(platform.Name) {
		system.summary(platform)
		return result, installHinted
	}
	if platform.Name == "macos" {
		_, _, ok := runInventory("", c.Env, "xcode-select", "-p")
		if !ok {
			system.say("31", "ERROR", "C compiler: Xcode Command Line Tools are not installed (required for compiling Tree-sitter parsers)")
			fmt.Fprintf(c.Out, "        Install them by running: %s\n", c.bold("xcode-select --install"))
			result = 1
		} else {
			result = c.statusCompiler(platform.Name, result, &system)
		}
	} else {
		result = c.statusCompiler(platform.Name, result, &system)
	}
	system.summary(platform)
	return result, installHinted
}
func (c CLI) statusCompiler(platform string, result int, system *diagnosticGroup) int {
	// A compiler counts only if it runs; a failed candidate falls through to the next.
	var broken []string
	for _, name := range []string{"gcc", "clang"} {
		if !commandExists(name, c.Env) {
			continue
		}
		out, _, ok := runInventory("", c.Env, name, "--version")
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
