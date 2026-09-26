package selfishell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallZinitPlugins provisions the declared plugin set through Zinit's
// cloneonly protocol. It never loads plugin code into the installing process.
func (o *PackageOperation) InstallZinitPlugins(ctx context.Context, paths Paths, manifest string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintln(o.Process.Out, "Would sync declared Zsh plugins.")
		return nil
	}
	if err := o.loadDependencies(manifest); err != nil {
		return err
	}
	dataHome := strings.TrimSuffix(paths.Data, "/selfishell")
	script := dataHome + "/zinit/zinit.git/zinit.zsh"
	info, err := os.Stat(script)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Zinit is not installed: %s", script)
	}
	for _, dep := range o.dependencies {
		if dep.Kind != "zsh-plugin" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validPluginName(dep.Name) {
			return fmt.Errorf("invalid Zinit plugin name: %s", dep.Name)
		}
		target := dataHome + "/zinit/plugins/" + strings.ReplaceAll(dep.Name, "/", "---")
		if o.validZinitPlugin(ctx, target, dep.Version) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := makeRawDir(rawParent(target)); err != nil {
			return err
		}
		original, originalErr := os.Lstat(target)
		if originalErr != nil && !os.IsNotExist(originalErr) {
			return originalErr
		}
		// Zinit's PLUGINS_DIR setting controls the entire checkout, including
		// its ._zinit bookkeeping. Keep the child in a private sibling until
		// the approved checkout has been verified.
		stageRoot, err := os.MkdirTemp(rawParent(target), ".selfishell-zinit.*")
		if err != nil {
			return err
		}
		stageTarget := stageRoot + "/" + strings.ReplaceAll(dep.Name, "/", "---")
		defer os.RemoveAll(stageRoot)
		p := o.Process
		p.In = strings.NewReader("")
		code, runErr := p.Run(ctx, "zsh", "-f", "-c", `typeset -A ZINIT
ZINIT[PLUGINS_DIR]="$4"
source "$1" || exit 1
zinit ice cloneonly "ver${3}" || exit 1
zinit light "$2"`, "zsh", script, dep.Name, dep.Version, stageRoot)
		if runErr == nil && code == 0 && !o.validZinitPlugin(ctx, stageTarget, dep.Version) {
			runErr = fmt.Errorf("Zinit plugin revision does not match after provisioning: %s", dep.Name)
		}
		if runErr == nil && code != 0 {
			runErr = fmt.Errorf("Could not provision Zinit plugin: %s", dep.Name)
		}
		if runErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return runErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		current, currentErr := os.Lstat(target)
		if original == nil && currentErr == nil || original != nil && (currentErr != nil || !os.SameFile(original, current)) {
			return fmt.Errorf("Zinit plugin target changed during provisioning: %s", target)
		}
		if currentErr != nil && !os.IsNotExist(currentErr) {
			return currentErr
		}
		var previous string
		moved := false
		if original != nil {
			previous, moved, err = moveAside(target)
			if err != nil {
				return err
			}
		}
		if occupied, err := present(target); err != nil || occupied {
			if moved {
				if restoreErr := restoreEmpty(previous, target); restoreErr != nil {
					return fmt.Errorf("Zinit plugin target occupied; restore: %w", restoreErr)
				}
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("Zinit plugin target occupied: %s", target)
		}
		if err := os.Rename(stageTarget, target); err != nil {
			if moved {
				if restoreErr := restoreEmpty(previous, target); restoreErr != nil {
					return fmt.Errorf("%v; restore: %w", err, restoreErr)
				}
			}
			return err
		}
		if moved {
			os.RemoveAll(previous)
			fmt.Fprintf(o.Process.Out, "Updated Zsh plugin: %s\n", dep.Name)
		}
	}
	return nil
}

func validPluginName(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && filepath.IsLocal(parts[0]) && filepath.IsLocal(parts[1])
}
func (o *PackageOperation) validZinitPlugin(ctx context.Context, target, version string) bool {
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() {
		return false
	}
	if git, err := os.Stat(target + "/.git"); err != nil || !git.IsDir() {
		return false
	}
	head, err := o.commandOutput(ctx, "git", "-C", target, "rev-parse", "HEAD")
	if err != nil || head != version {
		return false
	}
	status, err := o.commandOutput(ctx, "git", "-C", target, "status", "--porcelain")
	return err == nil && status == ""
}
