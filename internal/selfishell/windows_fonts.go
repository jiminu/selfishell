package selfishell

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type windowsFontStatus struct {
	FontInstalled bool              `json:"fontInstalled"`
	Registrations map[string]string `json:"registrations"`
}

// Windows fonts are optional direct packages. Like other packages, installed
// fonts remain available to other applications after configuration uninstall.
func (o *PackageOperation) installWindowsFont(ctx context.Context, paths Paths, dep Dependency) error {
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
	type pendingFont struct{ Target, Version, Checksum string }
	data, err := readStateFile(journal)
	if err == nil {
		var pending pendingFont
		if json.Unmarshal(data, &pending) != nil || pending.Target != target || pending.Version == "" || len(pending.Checksum) != 64 {
			return fmt.Errorf("invalid pending Windows font state: %s", journal)
		}
		if _, err := readStateFile(paths.State + "/dependencies/" + dep.Name); errors.Is(err, os.ErrNotExist) {
			if contents, err := os.ReadFile(target); err == nil {
				if fmt.Sprintf("%x", sha256.Sum256(contents)) != pending.Checksum {
					return fmt.Errorf("interrupted Windows font was modified: %s", target)
				}
				if err := writeAtomic(paths.State+"/dependencies/"+dep.Name, []byte(pending.Version+"\n"), 0600); err != nil {
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
	data, err = json.Marshal(pendingFont{target, dep.Version, dep.Checksum})
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
	_, err = o.Process.windowsScript(ctx, map[string]string{"operation": "font-register", "path": windowsPath, "checksum": dep.Checksum, "name": dep.Name})
	if err != nil {
		return err
	}
	return os.Remove(journal)
}
