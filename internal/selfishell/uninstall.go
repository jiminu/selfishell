package selfishell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func (c CLI) uninstallConfig(restore, purge, dry bool) error {
	paths, err := UserPaths()
	if err != nil {
		return err
	}
	if purge {
		if err = preflightPurge(c.Root); err != nil {
			return err
		}
	}
	resources, err := ManagedResources(c.Root)
	if err != nil {
		return err
	}
	records, err := LoadResourceStates(paths.Resources, resources)
	if err != nil {
		return fmt.Errorf("Uninstall cancelled because the managed resources could not be listed: %w", err)
	}
	m := managed{c: c, paths: paths, dry: dry}
	failed := false
	for _, record := range records {
		if e := m.preflightUninstall(record, restore); e != nil {
			c.error(e.Error())
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("Uninstall cancelled because managed resources were changed.")
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
	for _, dir := range []string{paths.Config + "/ghostty", paths.Config + "/nvim/after/lsp", paths.Config + "/nvim/after", paths.Config + "/nvim/lua/config", paths.Config + "/nvim/lua/plugins", paths.Config + "/nvim/lua", paths.Config + "/nvim", paths.Config + "/vim", paths.Config + "/mise", paths.Config + "/zsh", paths.Config, paths.Resources, paths.State} {
		syscall.Rmdir(dir)
	}
	if purge {
		return purgeFiles(c, paths)
	}
	fmt.Fprintln(c.Out, "Selfishell configuration uninstalled.")
	fmt.Fprintln(c.Out, "The Selfishell CLI is still installed.")
	fmt.Fprintln(c.Out, "Run 'selfishell uninstall --purge' to also remove the CLI, releases, cache, and state.")
	return nil
}
func (m *managed) preflightUninstall(record ResourceState, restore bool) error {
	s := record.State
	r := record.Resource
	info, present, err := exists(s.Target)
	if err != nil {
		return err
	}
	willRemove := false
	switch s.Kind {
	case "file":
		if present {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("Managed file path changed type; preserving it: %s", s.Target)
			}
			actual, e := Checksum(context.Background(), s.Target)
			if e != nil {
				return e
			}
			if actual != s.Checksum {
				return fmt.Errorf("Managed file was modified; preserving it: %s", s.Target)
			}
			willRemove = true
		}
	case "link":
		if present {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("Managed link was replaced; preserving it: %s", s.Target)
			}
			dest, e := os.Readlink(s.Target)
			if e != nil {
				return e
			}
			if dest != s.Reference {
				return fmt.Errorf("Managed link was replaced; preserving it: %s", s.Target)
			}
			willRemove = true
		}
	case "block":
		if !present || !info.Mode().IsRegular() {
			return fmt.Errorf("Managed block path changed type; preserving it: %s", s.Target)
		}
		data, e := os.ReadFile(s.Target)
		if e != nil {
			return e
		}
		view, e := inspectBlock(r.Name, data)
		if e != nil {
			return e
		}
		if view.status != "intact" || view.checksum != s.Checksum {
			return fmt.Errorf("Cannot manage the Selfishell %s block in: %s", r.Name, s.Target)
		}
	}
	if restore && s.Backup != "-" {
		_, hasBackup, e := exists(s.Backup)
		if e != nil {
			return e
		}
		if hasBackup && present && !willRemove {
			return fmt.Errorf("Restore target is occupied; preserving backup: %s", s.Backup)
		}
	}
	if s.Status == "pending" {
		return fmt.Errorf("An interrupted install left this unfinished; run 'selfishell install', then uninstall again.")
	}
	return nil
}
func (m *managed) removeResource(record ResourceState, restore bool) error {
	r, s := record.Resource, record.State
	if err := m.preflightUninstall(record, restore); err != nil {
		return err
	}
	info, present, err := exists(s.Target)
	if err != nil {
		return err
	}
	if s.Kind == "block" {
		if m.dry {
			m.say("Would remove Selfishell block: %s", s.Target)
		} else {
			data, e := os.ReadFile(s.Target)
			if e != nil {
				return e
			}
			view, e := inspectBlock(r.Name, data)
			if e != nil {
				return e
			}
			if err = writeAtomic(s.Target, spliceBlock(data, view, nil), info.Mode().Perm()); err != nil {
				return err
			}
		}
	} else if present {
		if m.dry {
			if s.Kind == "link" {
				m.say("Would remove managed link: %s", s.Target)
			} else {
				m.say("Would remove managed file: %s", s.Target)
			}
		} else if err = os.Remove(s.Target); err != nil {
			return err
		}
	}
	if restore && s.Backup != "-" {
		_, backupPresent, e := exists(s.Backup)
		if e != nil {
			return e
		}
		if backupPresent {
			if m.dry {
				m.say("Would restore: %s -> %s", s.Backup, s.Target)
			} else {
				if _, occupied, _ := exists(s.Target); occupied {
					return fmt.Errorf("Restore target is occupied; preserving backup: %s", s.Backup)
				}
				if err = makeRawDir(rawParent(s.Target)); err != nil {
					return err
				}
				if err = os.Rename(s.Backup, s.Target); err != nil {
					return err
				}
			}
		}
	}
	if !m.dry {
		return os.Remove(m.statePath(r))
	}
	return nil
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
	info, present, err := exists(bin)
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
func purgeFiles(c CLI, paths Paths) error {
	if err := preflightPurge(c.Root); err != nil {
		return err
	}
	share := filepath.Dir(filepath.Dir(c.Root))
	bin := filepath.Dir(filepath.Dir(share)) + "/bin"
	ownedSfs := false
	if _, present, e := exists(bin + "/sfs"); e != nil {
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
	entries, readErr := os.ReadDir(backups)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if len(entries) == 0 {
		if err := os.RemoveAll(paths.State); err != nil {
			return err
		}
	} else {
		entries, readErr = os.ReadDir(paths.State)
		if readErr != nil {
			return readErr
		}
		for _, e := range entries {
			if e.Name() != "backups" {
				if err := os.RemoveAll(paths.State + "/" + e.Name()); err != nil {
					return err
				}
			}
		}
	}
	fmt.Fprintln(c.Out, "Selfishell configuration, CLI, releases, cache, and state removed.")
	if len(entries) > 0 {
		fmt.Fprintf(c.Out, "Kept backups of modified files: %s\n", backups)
	}
	fmt.Fprintln(c.Out, "User-owned files it created once and never manages, such as the mise config.toml, are left in place.")
	return nil
}

func resolvedLinkTarget(link, target string) (string, error) {
	if !filepath.IsAbs(target) {
		target = rawParent(link) + "/" + target
	}
	parent, err := filepath.EvalSymlinks(rawParent(target))
	if err != nil {
		return "", err
	}
	return parent + "/" + filepath.Base(target), nil
}
func isOwnedSfs(path string) bool {
	info, present, e := exists(path)
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
func purgeDry(c CLI, paths Paths) {
	share := filepath.Dir(filepath.Dir(c.Root))
	bin := filepath.Dir(filepath.Dir(share)) + "/bin"
	if _, present, _ := exists(bin + "/sfs"); present {
		if isOwnedSfs(bin + "/sfs") {
			fmt.Fprintf(c.Out, "Would remove Selfishell CLI link: %s\n", bin+"/sfs")
		} else {
			fmt.Fprintf(c.Out, "Leaving %s in place; it is not the Selfishell sfs link.\n", bin+"/sfs")
		}
	}
	fmt.Fprintf(c.Out, "Would remove Selfishell CLI link: %s\n", bin+"/selfishell")
	fmt.Fprintf(c.Out, "Would remove Selfishell releases: %s\n", share)
	fmt.Fprintf(c.Out, "Would remove Selfishell cache: %s\n", paths.Cache)
	fmt.Fprintf(c.Out, "Would remove Selfishell state: %s\n", paths.State)
	entries, _ := os.ReadDir(paths.State + "/backups")
	if len(entries) > 0 {
		fmt.Fprintf(c.Out, "Would keep backups of modified files: %s\n", paths.State+"/backups")
	}
}
