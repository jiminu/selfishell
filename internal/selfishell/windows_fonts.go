package selfishell

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type windowsFontStatus struct {
	FontInstalled bool              `json:"fontInstalled"`
	Registrations map[string]string `json:"registrations"`
}

// Windows fonts are optional direct packages. Like other packages, installed
// fonts remain available to other applications after configuration uninstall.
func (o *PackageOperation) installWindowsFont(ctx context.Context, paths Paths, dep Dependency) error {
	if dep.Name == "." || filepath.Base(dep.Name) != dep.Name {
		return fmt.Errorf("invalid font dependency name: %s", dep.Name)
	}
	if !o.windowsFontsChecked {
		owned := false
		for _, d := range o.dependencies {
			if d.Marker != "font" {
				continue
			}
			for _, file := range []string{paths.State + "/dependencies/" + d.Name, paths.State + "/pending-fonts/" + d.Name} {
				if _, err := readStateFile(file); err == nil {
					owned = true
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
		if !owned {
			data, err := o.Process.windowsScript(ctx, map[string]string{"operation": "font-status"})
			if err != nil {
				return err
			}
			var status struct {
				FontInstalled bool `json:"fontInstalled"`
			}
			if err := json.Unmarshal(data, &status); err != nil {
				return err
			}
			o.windowsFontsExternal = status.FontInstalled
		}
		o.windowsFontsChecked = true
	}
	if o.windowsFontsExternal {
		o.report(reportInfo, "Externally installed; preserving Windows font: %s", terminalFont)
		return nil
	}
	target, err := dependencyTarget(dep, paths)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("Windows font path is not a regular file: %s", target)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	journal := paths.State + "/pending-fonts/" + dep.Name
	state := paths.State + "/dependencies/" + dep.Name
	type pendingFont struct {
		Target                string `json:"target"`
		Version               string `json:"version"`
		Checksum              string `json:"checksum"`
		PreviousPath          string `json:"previousPath,omitempty"`
		AlternatePreviousPath string `json:"alternatePreviousPath,omitempty"`
	}
	var pending pendingFont
	data, err := readStateFile(journal)
	hasPending := err == nil
	if hasPending {
		if json.Unmarshal(data, &pending) != nil || pending.Version == "" || len(pending.Checksum) != 64 {
			return fmt.Errorf("invalid pending Windows font state: %s", journal)
		}
		previous := dep
		previous.Version = pending.Version
		expected, err := dependencyTarget(previous, paths)
		if err != nil || expected != pending.Target {
			return fmt.Errorf("invalid pending Windows font target: %s", journal)
		}
		if _, err := readStateFile(state); errors.Is(err, os.ErrNotExist) {
			if contents, err := os.ReadFile(pending.Target); err == nil {
				if info, err := os.Lstat(pending.Target); err != nil || !info.Mode().IsRegular() || fmt.Sprintf("%x", sha256.Sum256(contents)) != pending.Checksum {
					return fmt.Errorf("interrupted Windows font was modified: %s", pending.Target)
				}
				if err := writeAtomic(state, []byte(pending.Version+"\n"), 0600); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err = readStateFile(state)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	oldVersion := strings.TrimSpace(string(data))
	next := pendingFont{Target: target, Version: dep.Version, Checksum: dep.Checksum}
	if hasPending && pending.Version == dep.Version {
		next.PreviousPath, next.AlternatePreviousPath = pending.PreviousPath, pending.AlternatePreviousPath
	}
	if oldVersion != "" && oldVersion != dep.Version {
		previous := dep
		previous.Version = oldVersion
		oldTarget, err := dependencyTarget(previous, paths)
		if err != nil {
			return err
		}
		next.PreviousPath, err = o.Process.windowsPath(ctx, "-w", oldTarget)
		if err != nil {
			return err
		}
		if hasPending {
			next.AlternatePreviousPath = pending.PreviousPath
		}
		// A version marker owns only that version's path. A new occupied path is user data.
		if exists, err := present(target); err != nil {
			return err
		} else if exists && (!hasPending || pending.Target != target) {
			return fmt.Errorf("unrecorded Windows font path is occupied; preserving: %s", target)
		}
	}
	data, err = json.Marshal(next)
	if err != nil {
		return err
	}
	if err := writeAtomic(journal, data, 0600); err != nil {
		return err
	}
	if err := o.installDependency(ctx, paths, dep); err != nil {
		return err
	}
	windowsPath, err := o.Process.windowsPath(ctx, "-w", target)
	if err != nil {
		return err
	}
	_, err = o.Process.windowsScript(ctx, map[string]string{"operation": "font-register", "path": windowsPath, "checksum": dep.Checksum, "name": dep.Name, "previousPath": next.PreviousPath, "alternatePreviousPath": next.AlternatePreviousPath})
	if err != nil {
		return err
	}
	return os.Remove(journal)
}
