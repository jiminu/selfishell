package selfishell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/terminal"
)

func (c CLI) uninstallConfig(restore, purge, dry bool) error {
	paths, err := managed.UserPaths(c.Env.Get)
	if err != nil {
		return err
	}
	if purge {
		if err = preflightPurge(c.Root); err != nil {
			return err
		}
		if _, err = backupInventory(paths); err != nil {
			return err
		}
	}
	resources, err := ManagedResources(c.Root, c.Env)
	if err != nil {
		return err
	}
	records, err := LoadResourceStates(paths.Resources, resources)
	if err != nil {
		return fmt.Errorf("Uninstall cancelled because the managed resources could not be listed: %w", err)
	}
	m := c.managedLifecycle(paths, dry, false)
	failed := false
	distro := c.Env.Get("WSL_DISTRO_NAME")
	if err := terminal.RemoveFragment(&m, paths, distro, true); err != nil {
		c.error(err.Error())
		failed = true
	}
	for _, record := range records {
		if e := m.preflightUninstall(record, restore); e != nil {
			c.error(e.Error())
			if record.State.Status == "pending" && !errors.Is(e, errInterruptedInstall) {
				c.error(errInterruptedInstall.Error())
			}
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("Uninstall cancelled because managed resources were changed.")
	}
	if err := terminal.RemoveFragment(&m, paths, distro, false); err != nil {
		return fmt.Errorf("Uninstall was incomplete; preserved remaining state for a retry: %w", err)
	}
	for i := len(records) - 1; i >= 0; i-- {
		if err = m.removeResource(records[i], restore); err != nil {
			return fmt.Errorf("Uninstall was incomplete; preserved remaining state for a retry: %w", err)
		}
	}
	if dry {
		fmt.Fprintln(c.Out, "Dry run complete; no files were changed.")
		if purge {
			purgeDry(c, paths)
		}
		return nil
	}
	os.Remove(paths.State + "/configured")
	os.Remove(paths.State + "/ghostty")
	os.Remove(paths.State + "/windows-terminal.json")
	for _, dir := range managedDirectories(resources, paths) {
		syscall.Rmdir(dir)
	}
	if purge {
		return purgeFiles(c, paths)
	}
	fmt.Fprintln(c.Out, "Selfishell configuration uninstalled.")
	fmt.Fprintln(c.Out, "The Selfishell CLI is still installed.")
	fmt.Fprintln(c.Out, "Run 'selfishell uninstall --purge' to also remove the CLI, releases, cache, and configuration state.")
	return nil
}

// managedDirectories lists the directories above declared resources inside
// paths.Config, children first, then the config, resources and state roots.
func managedDirectories(resources []managed.Resource, paths managed.Paths) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, r := range resources {
		if !strings.HasPrefix(r.Target, paths.Config+"/") {
			continue
		}
		for dir := managed.RawParent(r.Target); dir != paths.Config && !seen[dir]; dir = managed.RawParent(dir) {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	// Reverse lexical order places every directory before its parent.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	return append(dirs, paths.Config, paths.Resources, paths.State)
}

func preflightPurge(root string) error {
	releases := filepath.Dir(root)
	share := filepath.Dir(releases)
	if filepath.Base(releases) != "releases" {
		return fmt.Errorf("This command requires a versioned Selfishell installation.")
	}
	if info, err := os.Lstat(share + "/current"); err != nil || info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("This command requires a versioned Selfishell installation.")
	}
	bin := filepath.Dir(filepath.Dir(share)) + "/bin/selfishell"
	info, present, err := managed.Exists(bin)
	if err != nil {
		return err
	}
	if present {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("Refusing to remove non-Selfishell path: %s", bin)
		}
		dest, _ := os.Readlink(bin)
		actual, e := resolvedLinkTarget(bin, dest)
		if e != nil {
			return fmt.Errorf("Refusing to remove non-Selfishell path: %s", bin)
		}
		expected, e := resolvedLinkTarget(bin, share+"/current/bin/selfishell")
		if e != nil {
			return e
		}
		if actual != expected {
			return fmt.Errorf("Refusing to remove non-Selfishell path: %s", bin)
		}
	}
	return nil
}
func purgeFiles(c CLI, paths managed.Paths) error {
	if err := preflightPurge(c.Root); err != nil {
		return err
	}
	keepBackups, err := backupInventory(paths)
	if err != nil {
		return err
	}
	stateInfo, err := purgeStateDirectory(paths)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(paths.State)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	share := filepath.Dir(filepath.Dir(c.Root))
	bin := filepath.Dir(filepath.Dir(share)) + "/bin"
	ownedSfs := false
	if _, present, e := managed.Exists(bin + "/sfs"); e != nil {
		return e
	} else if present {
		ownedSfs = isOwnedSfs(bin + "/sfs")
		if !ownedSfs {
			fmt.Fprintf(c.Out, "Leaving %s in place; it is not the Selfishell sfs link.\n", bin+"/sfs")
		}
	}
	if ownedSfs {
		if err := os.Remove(bin + "/sfs"); err != nil {
			return err
		}
	}

	if err := os.Remove(bin + "/selfishell"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(paths.Cache); err != nil {
		return err
	}
	if err := os.RemoveAll(share); err != nil {
		return err
	}
	backups := paths.State + "/backups"
	currentState, err := purgeStateDirectory(paths)
	if err != nil {
		return err
	}
	if (stateInfo == nil) != (currentState == nil) || (stateInfo != nil && !os.SameFile(stateInfo, currentState)) {
		return fmt.Errorf("Selfishell state directory changed during purge; preserving it: %s", paths.State)
	}
	keptDependencies := false
	for _, entry := range entries {
		switch entry.Name() {
		case "dependencies", "retained-fonts", "pending-fonts":
			// Packages survive purge, so their ownership and recovery records must too.
			keptDependencies = true
			continue
		case "backups":
			if keepBackups {
				continue
			}
		}
		if err := os.RemoveAll(paths.State + "/" + entry.Name()); err != nil {
			return err
		}
	}
	if stateInfo != nil {
		syscall.Rmdir(paths.State)
	}

	fmt.Fprintln(c.Out, "Selfishell configuration, CLI, releases, and cache removed.")
	if keptDependencies {
		fmt.Fprintf(c.Out, "Kept ownership and recovery records for installed tools and fonts: %s\n", paths.State)
	}
	if keepBackups {
		fmt.Fprintf(c.Out, "Kept backups of modified files: %s\n", backups)
	}
	fmt.Fprintln(c.Out, "User-owned files it created once and never manages, such as the mise config.toml, are left in place.")
	return nil
}

func resolvedLinkTarget(link, target string) (string, error) {
	if !filepath.IsAbs(target) {
		target = managed.RawParent(link) + "/" + target
	}
	parent, err := filepath.EvalSymlinks(managed.RawParent(target))
	if err != nil {
		return "", err
	}
	return parent + "/" + filepath.Base(target), nil
}
func isOwnedSfs(path string) bool {
	info, present, e := managed.Exists(path)
	if e != nil || !present || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	dest, e := os.Readlink(path)
	if e != nil {
		return false
	}
	actual, e := resolvedLinkTarget(path, dest)
	if e != nil {
		return false
	}
	expected, e := resolvedLinkTarget(path, "selfishell")
	return e == nil && actual == expected
}
func purgeDry(c CLI, paths managed.Paths) {
	share := filepath.Dir(filepath.Dir(c.Root))
	bin := filepath.Dir(filepath.Dir(share)) + "/bin"
	if _, present, _ := managed.Exists(bin + "/sfs"); present {
		if isOwnedSfs(bin + "/sfs") {
			fmt.Fprintf(c.Out, "Would remove Selfishell CLI link: %s\n", bin+"/sfs")
		} else {
			fmt.Fprintf(c.Out, "Leaving %s in place; it is not the Selfishell sfs link.\n", bin+"/sfs")
		}
	}
	fmt.Fprintf(c.Out, "Would remove Selfishell CLI link: %s\n", bin+"/selfishell")
	fmt.Fprintf(c.Out, "Would remove Selfishell releases: %s\n", share)
	fmt.Fprintf(c.Out, "Would remove Selfishell cache: %s\n", paths.Cache)
	fmt.Fprintf(c.Out, "Would remove Selfishell configuration state, keeping ownership and recovery records for installed tools and fonts: %s\n", paths.State)
	entries, _ := os.ReadDir(paths.State + "/backups")
	if len(entries) > 0 {
		fmt.Fprintf(c.Out, "Would keep backups of modified files: %s\n", paths.State+"/backups")
	}
}

func backupInventory(paths managed.Paths) (bool, error) {
	if _, err := purgeStateDirectory(paths); err != nil {
		return false, err
	}
	path := paths.State + "/backups"
	info, present, err := managed.Exists(path)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	if !info.IsDir() || info.Mode().Perm()&0400 == 0 {
		return false, fmt.Errorf("Cannot read Selfishell backups: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	if len(entries) > 0 {
		if _, err := os.ReadDir(paths.State); err != nil {
			return false, err
		}
	}
	return len(entries) > 0, nil
}

func purgeStateDirectory(paths managed.Paths) (os.FileInfo, error) {
	info, present, err := managed.Exists(paths.State)
	if err != nil || !present {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Selfishell state path is not a directory; preserving it: %s", paths.State)
	}
	return info, nil
}
