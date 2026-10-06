package selfishell

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type windowsFontStatus struct {
	FontInstalled bool              `json:"fontInstalled"`
	Registrations map[string]string `json:"registrations"`
}

type retainedWindowsFont struct {
	Target   string `json:"target"`
	Checksum string `json:"checksum"`
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
		Target        string   `json:"target"`
		Version       string   `json:"version"`
		Checksum      string   `json:"checksum"`
		PreviousPaths []string `json:"previousPaths,omitempty"`
		// Read only: journals from 1.6.3-1.6.6 also kept candidates here.
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
	reuse := false
	next := pendingFont{Target: target, Version: dep.Version, Checksum: dep.Checksum}
	approve := func(path string) {
		if path != "" && !slices.Contains(next.PreviousPaths, path) {
			next.PreviousPaths = append(next.PreviousPaths, path)
		}
	}
	// Registration may still point to any earlier interrupted pin. Keep every
	// ownership-approved candidate until registration succeeds.
	for _, path := range append(append([]string(nil), pending.PreviousPaths...), pending.PreviousPath, pending.AlternatePreviousPath) {
		approve(path)
	}
	if oldVersion != "" && oldVersion != dep.Version {
		previous := dep
		previous.Version = oldVersion
		oldTarget, err := dependencyTarget(previous, paths)
		if err != nil {
			return err
		}
		oldPath, err := o.Process.windowsPath(ctx, "-w", oldTarget)
		if err != nil {
			return err
		}
		approve(oldPath)
		// Reuse only a recorded, intact old pin. Never replace a retained file:
		// Windows applications may still have it loaded.
		if exists, err := present(target); err != nil {
			return err
		} else if exists {
			record := paths.State + "/retained-fonts/" + dep.Name + "/" + dep.Version + ".json"
			data, err := readStateFile(record)
			if err == nil {
				var retained retainedWindowsFont
				if json.Unmarshal(data, &retained) != nil || retained.Target != target || retained.Checksum != dep.Checksum || !o.validDirect(ctx, dep, target, true) {
					return fmt.Errorf("retained Windows font was modified; preserving: %s", target)
				}
				reuse = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			} else if !hasPending || pending.Target != target {
				return fmt.Errorf("unrecorded Windows font path is occupied; preserving: %s", target)
			}
		}
		// Preserve the current marker's ownership evidence before changing it.
		// This also handles installations made before per-version records existed.
		if contents, err := readStateFile(oldTarget); err == nil {
			retained := retainedWindowsFont{Target: oldTarget, Checksum: fmt.Sprintf("%x", sha256.Sum256(contents))}
			data, err := json.Marshal(retained)
			if err != nil {
				return err
			}
			if err := writeAtomic(paths.State+"/retained-fonts/"+dep.Name+"/"+oldVersion+".json", data, 0600); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	data, err = json.Marshal(next)
	if err != nil {
		return err
	}
	if err := writeAtomic(journal, data, 0600); err != nil {
		return err
	}
	if reuse {
		if err := writeAtomic(state, []byte(dep.Version+"\n"), 0600); err != nil {
			return err
		}
		o.UnchangedCount++
	} else if err := o.installDependency(ctx, paths, dep); err != nil {
		return err
	}
	windowsPath, err := o.Process.windowsPath(ctx, "-w", target)
	if err != nil {
		return err
	}
	registeredPreviousPaths := next.PreviousPaths
	if len(next.PreviousPaths) > 1 {
		// Keep the full history in the journal, but do not put a growing list
		// on Windows' bounded command line. Only its current registry value
		// can be needed by the registration ownership guard.
		registeredPreviousPaths = nil
		data, err := o.Process.windowsScript(ctx, map[string]string{"operation": "font-status"})
		if err != nil {
			return err
		}
		var status windowsFontStatus
		if err := json.Unmarshal(data, &status); err != nil {
			return err
		}
		registered := status.Registrations["Selfishell "+dep.Name+" (TrueType)"]
		for _, candidate := range next.PreviousPaths {
			if registered != "" && strings.EqualFold(candidate, registered) {
				registeredPreviousPaths = []string{candidate}
				break
			}
		}
	}
	_, err = o.Process.windowsScript(ctx, map[string]any{"operation": "font-register", "path": windowsPath, "checksum": dep.Checksum, "name": dep.Name, "previousPaths": registeredPreviousPaths})
	if err != nil {
		return err
	}
	return os.Remove(journal)
}
