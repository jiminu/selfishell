package selfishell

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Package struct{ Platform, Requirement, Manager, Name string }
type Dependency struct{ Kind, Name, Version, Platform, Arch, Source, Checksum, Target, Marker string }

type invalidPackageNameError string

func (e invalidPackageNameError) Error() string { return "Invalid package name: " + string(e) }

func parseManifest(path string, fields int, accept func([]string) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		record := strings.Fields(line)
		if fields == 5 && record[0] != "package" {
			return fmt.Errorf("Unknown package manifest record: %s", record[0])
		}
		if len(record) != fields {
			return fmt.Errorf("invalid manifest record: %s", path)
		}
		if err := accept(record); err != nil {
			return err
		}
	}
	return scanner.Err()
}
func ReadPackages(path string) ([]Package, error) {
	var packages []Package
	seen := map[Package]bool{}
	err := parseManifest(path, 5, func(f []string) error {
		if f[0] != "package" {
			return fmt.Errorf("Unknown package manifest record: %s", f[0])
		}
		p := Package{f[1], f[2], f[3], f[4]}
		if p.Platform != "macos" && p.Platform != "ubuntu" && p.Platform != "ubuntu-wsl" && p.Platform != "all" {
			return fmt.Errorf("Invalid package platform: %s", p.Platform)
		}
		if p.Requirement != "required" && p.Requirement != "optional" {
			return fmt.Errorf("Invalid package requirement: %s", p.Requirement)
		}
		if p.Manager != "apt" && p.Manager != "formula" && p.Manager != "cask" && p.Manager != "direct" && p.Manager != "mise" {
			return fmt.Errorf("Invalid package manager: %s", p.Manager)
		}
		if strings.HasPrefix(p.Name, "-") || strings.IndexFunc(p.Name, func(r rune) bool {
			return !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789@+._/-", r)
		}) >= 0 {
			return invalidPackageNameError(p.Name)
		}
		if !seen[p] {
			packages = append(packages, p)
			seen[p] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packages, nil
}
func ReadDependencies(path string) ([]Dependency, error) {
	var dependencies []Dependency
	err := parseManifest(path, 9, func(f []string) error {
		d := Dependency{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8]}
		if d.Kind != "download" && d.Kind != "git" && d.Kind != "zsh-plugin" && d.Kind != "nvim-plugin" {
			return fmt.Errorf("Unknown dependency manifest record: %s", d.Kind)
		}
		if d.Platform != "all" && d.Platform != "macos" && d.Platform != "linux" {
			return fmt.Errorf("Invalid dependency platform: %s", d.Platform)
		}
		if d.Arch != "all" && d.Arch != "amd64" && d.Arch != "arm64" {
			return fmt.Errorf("Invalid dependency architecture: %s", d.Arch)
		}
		if strings.ContainsAny(strings.Join(f, ""), "\x00\r\n") {
			return fmt.Errorf("Invalid dependency record: %s", path)
		}
		dependencies = append(dependencies, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dependencies, nil
}

// selectPackages applies the same platform and saved-choice rules to setup and
// diagnostics. Only font downloads require Windows Terminal opt-in; other WSL
// packages remain available independently of that choice.
func selectPackages(root string, paths Paths, packages []Package, platform string, choice *windowsTerminalChoice) ([]Package, error) {
	var dependencies []Dependency
	selected := make([]Package, 0, len(packages))
	for _, p := range packages {
		if !packageMatches(p, platform) {
			continue
		}
		font := false
		if p.Platform == "ubuntu-wsl" && p.Manager == "direct" {
			if dependencies == nil {
				var err error
				dependencies, err = ReadDependencies(envDefault("SELFISHELL_DEPENDENCIES_FILE", root+"/dependencies.conf"))
				if err != nil {
					return nil, err
				}
			}
			for _, dep := range dependencies {
				if dep.Name == p.Name && dep.Kind == "download" && dep.Marker == "font" && (dep.Platform == "all" || dep.Platform == dependencyPlatform(platform)) {
					font = true
					break
				}
			}
		}
		if font {
			if choice == nil {
				var err error
				choice, err = readWindowsTerminalChoice(paths)
				if err != nil {
					return nil, err
				}
				if choice == nil {
					choice = &windowsTerminalChoice{}
				}
			}
			if !choice.Enabled {
				continue
			}
		}
		selected = append(selected, p)
	}
	return selected, nil
}
