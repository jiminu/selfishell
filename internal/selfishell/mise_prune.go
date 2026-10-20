package selfishell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var releaseVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`)

func ValidReleaseVersion(version string) bool {
	if !releaseVersionPattern.MatchString(version) {
		return false
	}
	_, prerelease, found := strings.Cut(version, "-")
	if !found {
		return true
	}
	for _, id := range strings.Split(prerelease, ".") {
		if len(id) > 1 && id[0] == '0' && strings.Trim(id, "0123456789") == "" {
			return false
		}
	}
	return true
}

// ValidRetainedRelease authorizes a previous release for cleanup or rollback.
// A missing previous link returns empty version and directory without error.
func ValidRetainedRelease(root string) (string, string, error) {
	rawReleases := rawParent(root)
	current, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	releases := filepath.Dir(current)
	if filepath.Base(releases) != "releases" {
		return "", "", fmt.Errorf("not a versioned release: %s", root)
	}
	previous := filepath.Dir(releases) + "/previous"
	link, err := os.Readlink(previous)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	version := filepath.Base(link)
	if !ValidReleaseVersion(version) || (link != "releases/"+version && link != releases+"/"+version && link != rawReleases+"/"+version) {
		return "", "", fmt.Errorf("invalid previous release link: %s", link)
	}
	dir, err := validReleaseDirectory(releases, version)
	if err != nil {
		return "", "", err
	}
	return version, dir, nil
}

// PruneMise scopes cleanup to the current platform's declared tools. Callers
// decide whether a returned cleanup failure is warning-only.
func (o *PackageOperation) PruneMise(ctx context.Context, root string, paths Paths, packages []Package, platform string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tools := platformMiseTools(packages, platform)
	if len(tools) == 0 {
		return nil
	}
	if dryRun {
		o.report(reportPreview, "Would prune unused mise versions for: %s (keeping current and tracked project versions, not rollback-only versions).", strings.Join(tools, " "))
		return nil
	}
	if len(o.SkippedOptional) != 0 {
		o.warn("Skipping mise cleanup because some optional packages could not be installed.")
		return nil
	}
	mise, err := o.miseCommand(paths)
	if err != nil {
		return err
	}
	_, previous, err := ValidRetainedRelease(root)
	if err != nil {
		return err
	}
	config := root + "/config/shared/mise.toml"
	info, err := os.Stat(config)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("invalid current mise config: %s", config)
	}
	base := o.miseProcess(root, true)
	trusted := envValue(base.Env, "MISE_TRUSTED_CONFIG_PATHS")
	if trusted != "" {
		trusted += ":"
	}
	base = withEnvironment(base, map[string]string{"MISE_TRUSTED_CONFIG_PATHS": trusted + root + "/config/shared"})
	if envValue(base.Env, "MISE_IGNORED_CONFIG_PATHS") != "" {
		return fmt.Errorf("ignored_config_paths can exclude retained versions")
	}
	shared := root + "/config/shared"
	ignored, err := o.miseOutput(ctx, base, mise, "-C", shared, "settings", "get", "ignored_config_paths")
	if err != nil {
		return err
	}
	if ignored != "[]" {
		return fmt.Errorf("ignored_config_paths can exclude retained versions")
	}
	if previous != "" {
		previousConfig := previous + "/config/shared/mise.toml"
		for _, path := range []string{previous + "/config", previous + "/config/shared", previousConfig} {
			member, err := os.Lstat(path)
			if err == nil && member.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("unsafe previous mise configuration: %s", path)
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		previousInfo, err := os.Stat(previousConfig)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !os.SameFile(info, previousInfo) {
			if strings.ContainsAny(previousConfig, ":*?[{") {
				return fmt.Errorf("unsafe previous mise configuration path: %s", previousConfig)
			}
			base = withEnvironment(base, map[string]string{"MISE_IGNORED_CONFIG_PATHS": previousConfig})
		}
	}
	if _, err := o.miseOutput(ctx, base, mise, "-C", shared, "config", "ls"); err != nil {
		return err
	}
	tracked, err := o.miseOutput(ctx, base, mise, "-C", shared, "config", "ls", "--tracked-configs")
	if err != nil {
		return err
	}
	found := false
	for _, line := range strings.Split(tracked, "\n") {
		trackedInfo, err := os.Stat(line)
		if err == nil && os.SameFile(info, trackedInfo) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("current mise config is not tracked: %s", config)
	}
	data := envValue(base.Env, "MISE_DATA_DIR")
	if data == "" {
		data = envValue(base.Env, "XDG_DATA_HOME")
		if data == "" {
			data = envValue(base.Env, "HOME") + "/.local/share"
		}
		data += "/mise"
	}
	installs := data + "/installs"
	before := miseInstalledVersions(installs)
	code, err := base.Run(ctx, mise, append([]string{"-C", shared, "prune", "--tools", "--yes"}, tools...)...)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("mise prune exited %d", code)
	}
	if o.Process.progress != nil {
		o.report(reportSuccess, "Cleaned up unused mise versions.")
	}
	after := miseInstalledVersions(installs)
	removed := map[string]bool{}
	for version := range before {
		if !after[version] {
			removed[version] = true
		}
	}
	if len(removed) != 0 {
		// Open shells keep absolute paths, such as Starship's init, to removed versions.
		o.note("Run 'exec zsh' in open shells to reload updated tools.")
	}
	dataHome := envValue(base.Env, "XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = envValue(base.Env, "HOME") + "/.local/share"
	}
	if stale := masonPackagesOnRemovedPython(dataHome+"/nvim/mason/packages", installs, removed); len(stale) != 0 {
		o.note("Run ':MasonInstall --force %s' in Neovim: its virtualenv used a removed Python.", strings.Join(stale, " "))
	}
	return nil
}

// masonPackagesOnRemovedPython lists Mason packages whose virtualenv was built
// on a pruned mise Python; a virtualenv keeps its interpreter's absolute path.
func masonPackagesOnRemovedPython(packages, installs string, removed map[string]bool) []string {
	var stale []string
	entries, _ := os.ReadDir(packages)
	for _, entry := range entries {
		config, err := os.ReadFile(packages + "/" + entry.Name() + "/venv/pyvenv.cfg")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(config), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok || strings.TrimSpace(key) != "home" {
				continue
			}
			version, ok := strings.CutPrefix(strings.TrimSpace(value), installs+"/python/")
			if version, _, _ = strings.Cut(version, "/"); ok && removed["python/"+version] {
				stale = append(stale, entry.Name())
			}
			break
		}
	}
	return stale
}

// miseInstalledVersions lists <tool>/<version> directories, skipping alias links.
func miseInstalledVersions(installs string) map[string]bool {
	versions := map[string]bool{}
	tools, _ := os.ReadDir(installs)
	for _, tool := range tools {
		entries, _ := os.ReadDir(installs + "/" + tool.Name())
		for _, entry := range entries {
			if entry.IsDir() {
				versions[tool.Name()+"/"+entry.Name()] = true
			}
		}
	}
	return versions
}
