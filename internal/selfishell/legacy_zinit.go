package selfishell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Releases before direct Zsh plugins installed Zinit with exactly these plugins.
var legacyZinitPlugins = map[string]bool{
	"_local---zinit":                               true,
	"zsh-users---zsh-completions":                  true,
	"Aloxaf---fzf-tab":                             true,
	"zsh-users---zsh-autosuggestions":              true,
	"zdharma-continuum---fast-syntax-highlighting": true,
}

// Entries Zinit itself creates in its home directory; anything else is user data.
var legacyZinitEntries = map[string]bool{"zinit.git": true, "plugins": true, "polaris": true, "completions": true, "snippets": true, "services": true}

func legacyZinitDir(paths Paths) string {
	return strings.TrimSuffix(paths.Data, "/selfishell") + "/zinit"
}

// legacyZinitRemovable reports whether dir (present: it exists) is a Zinit
// checkout Selfishell installed and nobody extended. It never follows links.
func legacyZinitRemovable(paths Paths) (present, removable bool, err error) {
	dir := legacyZinitDir(paths)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	if _, err := os.Lstat(paths.State + "/dependencies/zinit"); err != nil || !info.IsDir() {
		return true, false, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, false, err
	}
	for _, entry := range entries {
		if !legacyZinitEntries[entry.Name()] {
			return true, false, nil
		}
	}
	plugins, err := realDirEntries(dir + "/plugins")
	if err != nil || plugins == nil {
		return true, false, err
	}
	for _, entry := range plugins {
		if !legacyZinitPlugins[entry.Name()] || !entry.IsDir() {
			return true, false, nil
		}
	}
	defaults, err := onlyZinitPolarisDefaults(dir+"/polaris", "")
	return true, defaults, err
}

// realDirEntries lists a real directory, returns an empty list when it is
// absent, and nil without error when the path is a link or not a directory.
func realDirEntries(path string) ([]os.DirEntry, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []os.DirEntry{}, nil
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(path)
	if entries == nil && err == nil {
		entries = []os.DirEntry{}
	}
	return entries, err
}

// Zinit copies its own manual into polaris on every load (zinit.zsh installs
// doc/zinit.1 under $ZPFX/man/man1), so a default polaris is that one regular
// file in a tree of otherwise empty directories.
const zinitPolarisManual = "man/man1/zinit.1"

// onlyZinitPolarisDefaults accepts an absent path or a tree of real directories
// holding nothing but Zinit's own manual; any other file or link is user data.
func onlyZinitPolarisDefaults(path, relative string) (bool, error) {
	entries, err := realDirEntries(path)
	if err != nil || entries == nil {
		return false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if relative != "" {
			name = relative + "/" + name
		}
		if entry.Type().IsRegular() && name == zinitPolarisManual {
			continue
		}
		if !entry.IsDir() {
			return false, nil
		}
		if ok, err := onlyZinitPolarisDefaults(path+"/"+entry.Name(), name); err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// RemoveLegacyZinit deletes a Selfishell-installed Zinit checkout once direct
// plugins replace it. A failure only warns, so the next run retries.
func (o *PackageOperation) RemoveLegacyZinit(ctx context.Context, paths Paths, dry bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := legacyZinitDir(paths)
	present, removable, err := legacyZinitRemovable(paths)
	if err != nil {
		o.warn(fmt.Sprintf("Could not inspect the legacy Zinit checkout %s: %v", dir, err))
		return nil
	}
	state := paths.State + "/dependencies/zinit"
	if !present {
		// An earlier run removed the checkout but not its record.
		if _, err := os.Lstat(state); err == nil && !dry {
			if err := os.Remove(state); err != nil {
				o.warn(fmt.Sprintf("Could not remove the legacy Zinit record %s: %v", state, err))
			}
		}
		return nil
	}
	if !removable {
		return nil
	}
	if dry {
		o.report(reportPreview, "Would remove the legacy Zinit checkout: %s", dir)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.dependencyFault != nil {
		err = o.dependencyFault("legacy-zinit")
	}
	if err == nil {
		err = os.RemoveAll(dir)
	}
	if err == nil {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = os.Remove(state)
	}
	if err != nil {
		o.warn(fmt.Sprintf("Could not remove the legacy Zinit checkout %s: %v", dir, err))
		return nil
	}
	o.report(reportSuccess, "Removed the legacy Zinit checkout: %s", dir)
	return nil
}

// legacyZinitNote points at a Zinit checkout that install keeps because it may
// be in use; a removable one needs no note, since the next update deletes it.
func (c CLI) legacyZinitNote(paths Paths) {
	if present, removable, _ := legacyZinitRemovable(paths); present && !removable {
		c.sayDiagnostic("36", "INFO", "Legacy Zinit checkout remains: "+legacyZinitDir(paths)+"; delete it by hand if you do not use Zinit directly.")
	}
}
