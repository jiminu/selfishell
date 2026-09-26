package selfishell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolResult uses the same status vocabulary as the shell diagnostics.
type ToolResult struct{ Installed, Source, Approved string }

// ToolInventory caches external package-manager queries for one CLI invocation.
type ToolInventory struct {
	root                                             string
	paths                                            Paths
	warnings                                         io.Writer
	dependencies                                     []Dependency
	apt                                              map[string]string
	aptReady                                         bool
	brewFormulae, brewCasks                          map[string]string
	brewJSONReady, brewFormulaeReady, brewCasksReady bool
	miseVersions                                     map[string]string
	miseApproved                                     map[string]string
	miseReady                                        bool
}

func NewToolInventory(root string, paths Paths, warnings io.Writer) (*ToolInventory, error) {
	deps, err := ReadDependencies(envDefault("SELFISHELL_DEPENDENCIES_FILE", filepath.Join(root, "dependencies.conf")))
	if err != nil {
		return nil, err
	}
	return &ToolInventory{root: root, paths: paths, warnings: warnings, dependencies: deps}, nil
}

func (i *ToolInventory) Detect(manager, name, platform, arch string) (ToolResult, error) {
	result := ToolResult{"missing", "none", "package-manager"}
	var version string
	switch manager {
	case "apt":
		version = i.aptVersion(name)
		if version != "" {
			result.Installed, result.Source = version, "apt"
			return result, nil
		}
	case "formula", "cask":
		version = i.brewVersion(manager, name)
		if version != "" {
			result.Installed = version
			result.Source = "homebrew"
			if manager == "cask" {
				result.Source = "homebrew-cask"
			}
			return result, nil
		}
	case "direct":
		return i.directVersion(name, platform, arch)
	case "mise":
		i.loadMise()
		result.Approved = i.miseApproved[name]
		if version = i.miseVersions[name]; version != "" {
			result.Installed, result.Source = version, "mise"
			return result, nil
		}
	default:
		return result, fmt.Errorf("unknown package manager: %s", manager)
	}
	executable := toolExecutable(name)
	path, err := exec.LookPath(executable)
	if err == nil {
		shims := envDefault("MISE_DATA_DIR", envDefault("XDG_DATA_HOME", os.Getenv("HOME")+"/.local/share")+"/mise") + "/shims/"
		if manager != "mise" || !strings.HasPrefix(path, shims) {
			result.Installed, result.Source = "detected", "external"
		}
	}
	return result, nil
}

func toolExecutable(name string) string {
	switch name {
	case "ripgrep":
		return "rg"
	case "neovim":
		return "nvim"
	}
	if before, _, ok := strings.Cut(name, "@"); ok {
		return before
	}
	return name
}

func runInventory(dir string, env []string, name string, args ...string) (string, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return runInventoryContext(ctx, dir, env, name, args...)
}
func runInventoryContext(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, bool) {
	var out, stderr bytes.Buffer
	p := Process{Out: &out, Err: &stderr, Dir: dir, Env: env}
	code, err := p.Run(ctx, name, args...)
	return out.String(), stderr.String(), err == nil && code == 0
}

func (i *ToolInventory) aptVersion(name string) string {
	if !i.aptReady {
		i.aptReady = true
		i.apt = map[string]string{}
		if _, err := exec.LookPath("dpkg-query"); err == nil {
			output, _, ok := runInventory("", nil, "dpkg-query", "-W", "-f=${binary:Package}\t${db:Status-Abbrev}\t${Version}\n")
			if ok {
				for _, line := range strings.Split(output, "\n") {
					f := strings.Split(line, "\t")
					if len(f) != 3 || len(f[1]) != 3 || f[1][1] != 'i' || f[1][2] != ' ' || f[2] == "" {
						continue
					}
					key, _, _ := strings.Cut(f[0], ":")
					if i.apt[key] == "" {
						i.apt[key] = f[2]
					}
				}
			}
		}
	}
	return i.apt[name]
}

func parseBrewLines(output string) map[string]string {
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		name, versions, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && versions != "" {
			found[name] = versions
		}
	}
	return found
}
func (i *ToolInventory) loadBrewJSON() {
	if i.brewJSONReady {
		return
	}
	i.brewJSONReady = true
	output, _, ok := runInventory("", nil, "brew", "list", "--versions", "--json")
	if !ok || strings.TrimSpace(output) == "" {
		return
	}
	var data struct {
		Formulae []struct {
			Name     string   `json:"name"`
			Versions []string `json:"versions"`
		} `json:"formulae"`
		Casks []struct {
			Token    string   `json:"token"`
			Versions []string `json:"versions"`
		} `json:"casks"`
	}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return
	}
	// Both typed arrays must exist; otherwise use Homebrew's legacy inventory.
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(output), &raw) != nil || raw["formulae"] == nil || raw["casks"] == nil || string(raw["formulae"]) == "null" || string(raw["casks"]) == "null" {
		return
	}
	i.brewFormulae = map[string]string{}
	i.brewCasks = map[string]string{}
	for _, entry := range data.Formulae {
		i.brewFormulae[entry.Name] = strings.Join(entry.Versions, " ")
	}
	for _, entry := range data.Casks {
		i.brewCasks[entry.Token] = strings.Join(entry.Versions, " ")
	}
	i.brewFormulaeReady = true
	i.brewCasksReady = true
}
func (i *ToolInventory) brewVersion(manager, name string) string {
	if _, err := exec.LookPath("brew"); err != nil {
		return ""
	}
	i.loadBrewJSON()
	if manager == "formula" {
		if !i.brewFormulaeReady {
			i.brewFormulaeReady = true
			output, _, ok := runInventory("", nil, "brew", "list", "--formula", "--versions")
			if ok {
				i.brewFormulae = parseBrewLines(output)
			}
		}
		if version := i.brewFormulae[name]; version != "" {
			return version
		}
		// Homebrew may list a formula under its canonical name rather than an alias.
		output, _, ok := runInventory("", nil, "brew", "list", "--versions", name)
		if !ok {
			return ""
		}
		_, version, found := strings.Cut(strings.TrimSpace(output), " ")
		if found {
			return version
		}
		return ""
	}
	if !i.brewCasksReady {
		i.brewCasksReady = true
		output, _, ok := runInventory("", nil, "brew", "list", "--cask", "--versions")
		if ok {
			i.brewCasks = parseBrewLines(output)
		}
	}
	return i.brewCasks[name]
}

func (i *ToolInventory) directVersion(name, platform, arch string) (ToolResult, error) {
	result := ToolResult{"missing", "none", ""}
	var dep *Dependency
	for n := range i.dependencies {
		d := &i.dependencies[n]
		if d.Name == name && (d.Platform == "all" || d.Platform == platform) && (d.Arch == "all" || d.Arch == arch) {
			dep = d
			break
		}
	}
	if dep == nil {
		return result, fmt.Errorf("No approved dependency entry for %s (%s/%s).", name, platform, arch)
	}
	result.Approved = dep.Version
	home := os.Getenv("HOME")
	target := home + "/" + dep.Target
	if strings.HasPrefix(dep.Target, ".local/share/") {
		target = envDefault("XDG_DATA_HOME", home+"/.local/share") + "/" + strings.TrimPrefix(dep.Target, ".local/share/")
	}
	state, err := os.ReadFile(i.paths.State + "/dependencies/" + name)
	recorded := strings.TrimRight(strings.ReplaceAll(string(state), "\x00", ""), "\n")
	if err == nil && recorded != "" {
		result.Source = "selfishell"
		if i.validDirect(*dep, target, true) {
			result.Installed = recorded
		}
		return result, nil
	}
	if i.validDirect(*dep, target, false) {
		result.Installed, result.Source = "detected", "external"
	}
	return result, nil
}
func (i *ToolInventory) validDirect(dep Dependency, target string, managed bool) bool {
	info, err := os.Stat(target)
	if err != nil {
		return false
	}
	if managed {
		if link, err := os.Lstat(target); err != nil || link.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	switch dep.Kind {
	case "download":
		return info.Mode().IsRegular() && info.Mode()&0111 != 0
	case "git":
		if !info.IsDir() {
			return false
		}
		if _, err := os.Stat(filepath.Join(target, dep.Marker)); err != nil {
			return false
		}
		if !managed {
			return true
		}
		if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
			return false
		}
		if dep.Checksum != "-" {
			out, _, ok := runInventory("", nil, "git", "-C", target, "rev-parse", "HEAD")
			if !ok || strings.TrimSpace(out) != dep.Checksum {
				return false
			}
		}
		changes, _, ok := runInventory("", append(os.Environ(), "GIT_DIR=.git", "GIT_WORK_TREE=.", "GIT_OPTIONAL_LOCKS=0"), "git", "-C", target, "status", "--porcelain", "--untracked-files=no")
		return ok && strings.TrimSpace(changes) == ""
	}
	return false
}

func (i *ToolInventory) loadMise() {
	if i.miseReady {
		return
	}
	i.miseReady = true
	i.miseApproved = map[string]string{}
	i.miseVersions = map[string]string{}
	config := filepath.Join(i.root, "config", "shared", "mise.toml")
	data, err := os.ReadFile(config)
	if err == nil {
		inTools := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				inTools = line == "[tools]"
				continue
			}
			if !inTools {
				continue
			}
			name, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
				i.miseApproved[name] = strings.Trim(value, "\"")
			}
		}
	}
	command, err := exec.LookPath("mise")
	if err != nil {
		command = filepath.Join(os.Getenv("HOME"), ".local", "bin", "mise")
		info, e := os.Stat(command)
		if e != nil || info.Mode()&0111 == 0 {
			return
		}
	}
	shared := filepath.Join(i.root, "config", "shared")
	env := append(os.Environ(), "NO_COLOR=1", "MISE_GLOBAL_CONFIG_FILE="+config)
	output, stderr, ok := runInventory(shared, env, command, "-C", shared, "ls", "--current", "--installed", "--no-header", "--no-truncate")
	if !ok {
		line, _, _ := strings.Cut(stderr, "\n")
		if i.warnings != nil {
			prefix := "selfishell: warning:"
			if os.Getenv("NO_COLOR") == "" && IsTerminal(i.warnings) {
				prefix = "\x1b[33mselfishell: warning:\x1b[0m"
			}
			if line != "" {
				line = ": " + line
			}
			fmt.Fprintf(i.warnings, "%s mise could not list installed tools%s\n", prefix, line)
		}
		return
	}
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			if old := i.miseVersions[f[0]]; old != "" {
				i.miseVersions[f[0]] = old + " " + f[1]
			} else {
				i.miseVersions[f[0]] = f[1]
			}
		}
	}
}
