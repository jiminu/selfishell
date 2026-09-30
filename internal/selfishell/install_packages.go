package selfishell

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// installPackages follows the requirement/manager order with one operation.
func (c CLI) installPackages(ctx context.Context, o *PackageOperation, paths Paths, packages []Package, platform, arch string, dry bool) error {
	selected := packagePlatform(platform)
	groups := map[string][]string{}
	for _, p := range packages {
		if p.Platform == "all" || p.Platform == selected {
			key := p.Requirement + ":" + p.Manager
			groups[key] = append(groups[key], p.Name)
		}
	}
	manifest := envDefault("SELFISHELL_DEPENDENCIES_FILE", c.Root+"/dependencies.conf")
	for _, pair := range []struct{ requirement, manager string }{
		{"required", "apt"}, {"optional", "apt"},
		{"required", "formula"}, {"optional", "formula"},
		{"required", "cask"}, {"optional", "cask"},
		{"required", "direct"}, {"optional", "direct"},
		{"required", "mise"}, {"optional", "mise"},
	} {
		names := groups[pair.requirement+":"+pair.manager]
		if len(names) == 0 {
			continue
		}
		var err error
		c.progress.stage("Synchronizing " + pair.manager + " packages")
		switch pair.manager {
		case "apt":
			err = o.InstallApt(ctx, pair.requirement, dry, names...)
		case "formula", "cask":
			err = o.InstallHomebrew(ctx, pair.requirement, pair.manager, dry, names...)
		case "direct":
			for _, name := range names {
				if err = o.InstallDirect(ctx, paths, manifest, pair.requirement, name, platform, arch, dry); err != nil {
					break
				}
			}
		case "mise":
			pins, e := approvedMisePins(c.Root+"/config/shared/mise.toml", names)
			if e != nil {
				return e
			}
			before := len(o.SkippedOptional)
			err = o.InstallMise(ctx, c.Root, paths, pair.requirement, dry, pins...)
			for i := before; i < len(o.SkippedOptional); i++ {
				o.SkippedOptional[i], _, _ = strings.Cut(o.SkippedOptional[i], "@")
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// platformMiseTools lists the mise tools packages.conf declares for platform.
func platformMiseTools(packages []Package, platform string) []string {
	platform = packagePlatform(platform)
	var tools []string
	for _, p := range packages {
		if p.Manager == "mise" && (p.Platform == "all" || p.Platform == platform) {
			tools = append(tools, p.Name)
		}
	}
	return tools
}

// checkMisePins fails before any change when a declared mise tool has no release pin.
func checkMisePins(root string, packages []Package, platform string) error {
	names := platformMiseTools(packages, platform)
	if len(names) == 0 {
		return nil
	}
	_, err := approvedMisePins(root+"/config/shared/mise.toml", names)
	return err
}

func approvedMisePins(file string, names []string) ([]string, error) {
	versions, err := approvedMiseVersions(file)
	if err != nil {
		return nil, err
	}
	pins := make([]string, 0, len(names))
	for _, name := range names {
		version := versions[name]
		if version == "" {
			return nil, fmt.Errorf("missing approved mise version: %s", name)
		}
		pins = append(pins, name+"@"+version)
	}
	return pins, nil
}

// approvedMiseVersions reads the quoted [tools] values of a release mise.toml.
func approvedMiseVersions(file string) (map[string]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
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
		name, version, ok := strings.Cut(line, "=")
		version = strings.TrimSpace(version)
		if ok && len(version) > 1 && version[0] == '"' && version[len(version)-1] == '"' {
			versions[strings.TrimSpace(name)] = version[1 : len(version)-1]
		}
	}
	return versions, nil
}
