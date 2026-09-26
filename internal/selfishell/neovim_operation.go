package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var gitHashPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func (o *PackageOperation) gitHead(ctx context.Context, dir string) (string, error) {
	git := dir + "/.git"
	data, err := os.ReadFile(git + "/HEAD")
	if err == nil {
		head, _, _ := strings.Cut(string(data), "\n")
		if gitHashPattern.MatchString(head) {
			return head, nil
		}
		if strings.HasPrefix(head, "ref: refs/") {
			ref := strings.TrimPrefix(head, "ref: ")
			if filepath.IsLocal(ref) {
				if data, err := os.ReadFile(git + "/" + ref); err == nil {
					sha, _, _ := strings.Cut(string(data), "\n")
					if gitHashPattern.MatchString(sha) {
						return sha, nil
					}
				} else if os.IsNotExist(err) {
					if data, err := os.ReadFile(git + "/packed-refs"); err == nil {
						for _, line := range strings.Split(string(data), "\n") {
							fields := strings.Fields(line)
							if len(fields) == 2 && fields[1] == ref && gitHashPattern.MatchString(fields[0]) {
								return fields[0], nil
							}
						}
					}
				}
			}
		}
	}
	return o.commandOutput(ctx, "git", "-C", dir, "rev-parse", "HEAD")
}

func (o *PackageOperation) gitTrackedChanges(ctx context.Context, dir string, excludeTags bool) (string, error) {
	return o.gitChanges(ctx, dir, excludeTags, false)
}

func (o *PackageOperation) gitChanges(ctx context.Context, dir string, excludeTags, includeUntracked bool) (string, error) {
	p := withEnvironment(o.Process, map[string]string{"GIT_DIR": ".git", "GIT_WORK_TREE": ".", "GIT_OPTIONAL_LOCKS": "0"})
	var out, stderr bytes.Buffer
	p.Out, p.Err = &out, &stderr
	args := []string{"-C", dir, "status", "--porcelain"}
	if !includeUntracked {
		args = append(args, "--untracked-files=no")
	}
	args = append(args, "--")
	if excludeTags {
		args = append(args, ":(exclude)doc/tags")
	}
	code, err := p.Run(ctx, "git", args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git status exited %d: %s", code, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func nvimPluginPath(paths Paths, dep Dependency) (string, error) {
	data := strings.TrimSuffix(paths.Data, "/selfishell")
	if dep.Name == "folke/lazy.nvim" {
		return paths.Data + "/nvim/lazy/lazy.nvim", nil
	}
	name := strings.TrimSuffix(filepath.Base(dep.Source), ".git")
	if name == "." || name == "" || !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid Neovim plugin source: %s", dep.Source)
	}
	return data + "/nvim/lazy/" + name, nil
}

func (o *PackageOperation) nvimCommand(ctx context.Context, root string, paths Paths) (string, string, error) {
	mise, _ := o.miseCommand(paths)
	if mise != "" {
		resolved, err := o.miseOutput(ctx, o.miseProcess(root, false), mise, "-C", root+"/config/shared", "which", "nvim")
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if err == nil {
			if info, err := os.Stat(resolved); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
				return resolved, mise, nil
			}
		}
	}
	if path, err := o.Process.lookPath("nvim"); err == nil {
		return path, mise, nil
	}
	home := envValue(o.Process.environment(), "HOME")
	path := home + "/.local/bin/nvim"
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
		return path, mise, nil
	}
	return "", mise, fmt.Errorf("Could not locate Neovim after installing the development environment.")
}

func (o *PackageOperation) runNvim(ctx context.Context, root, nvim, mise string, args ...string) (string, error) {
	p := o.Process
	name := nvim
	if mise != "" {
		p = o.miseProcess(root, false)
		name = mise
		args = append([]string{"-C", root + "/config/shared", "exec", "--", nvim}, args...)
	}
	var log bytes.Buffer
	p.Out, p.Err = &log, &log
	code, err := p.Run(ctx, name, args...)
	if err := ctx.Err(); err != nil {
		return log.String(), err
	}
	if err != nil || code != 0 {
		if log.Len() != 0 {
			fmt.Fprint(o.Process.Err, log.String())
		}
		if err != nil {
			return log.String(), err
		}
		return log.String(), fmt.Errorf("Neovim exited %d", code)
	}
	return log.String(), nil
}

func (o *PackageOperation) installLazy(ctx context.Context, paths Paths, dep Dependency) error {
	if !gitHashPattern.MatchString(dep.Version) {
		return fmt.Errorf("Invalid approved lazy.nvim revision: %s", dep.Version)
	}
	target, _ := nvimPluginPath(paths, dep)
	previously := false
	if exists, err := present(target); err != nil && !errors.Is(err, syscall.ENOTDIR) {
		return err
	} else if exists {
		info, err := os.Lstat(target)
		if err != nil {
			return err
		}
		git, err := os.Lstat(target + "/.git")
		if !info.IsDir() || err != nil || !git.IsDir() {
			return fmt.Errorf("lazy.nvim path is not an approved Git checkout; preserving it: %s", target)
		}
		changes, err := o.gitChanges(ctx, target, false, true)
		if err != nil {
			return err
		}
		if changes != "" {
			return fmt.Errorf("lazy.nvim checkout was modified; preserving it: %s", target)
		}
		head, err := o.gitHead(ctx, target)
		if err != nil {
			return err
		}
		if head == dep.Version {
			return nil
		}
		previously = true
	}
	if err := makeRawDir(rawParent(target)); err != nil {
		return fmt.Errorf("Could not create Neovim plugin directory: %s: %w", rawParent(target), err)
	}
	stage, err := siblingName(target, ".tmp.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if _, err := o.commandOutput(ctx, "git", "clone", "--quiet", "--filter=blob:none", "--", dep.Source, stage); err != nil {
		return err
	}
	if _, err := o.commandOutput(ctx, "git", "-C", stage, "checkout", "--quiet", "--detach", dep.Version); err != nil {
		return err
	}
	if head, err := o.gitHead(ctx, stage); err != nil || head != dep.Version {
		return fmt.Errorf("lazy.nvim revision does not match approved pin: %s", dep.Version)
	}
	old, moved, err := moveAside(target)
	if err != nil {
		return err
	}
	if occupied, err := present(target); err != nil || occupied {
		if moved {
			_ = restoreEmpty(old, target)
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("lazy.nvim target occupied: %s", target)
	}
	if err := os.Rename(stage, target); err != nil {
		if moved {
			_ = restoreEmpty(old, target)
		}
		return err
	}
	if moved {
		os.RemoveAll(old)
	}
	verb := "Installed"
	if previously {
		verb = "Updated"
	}
	fmt.Fprintf(o.Process.Out, "%s approved lazy.nvim revision: %s\n", verb, dep.Version)
	return nil
}

// InstallNeovimPlugins syncs declared pins and parser updates with the release environment.
func (o *PackageOperation) InstallNeovimPlugins(ctx context.Context, root string, paths Paths, manifest string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintln(o.Process.Out, "Would sync declared Neovim plugins.\nWould sync lazy.nvim bootstrap repository.\nWould update installed Tree-sitter parsers.")
		return nil
	}
	if err := o.loadDependencies(manifest); err != nil {
		return err
	}
	var deps []Dependency
	var lazy *Dependency
	for _, dep := range o.dependencies {
		if dep.Kind != "nvim-plugin" {
			continue
		}
		deps = append(deps, dep)
		if dep.Name == "folke/lazy.nvim" {
			d := dep
			lazy = &d
		}
	}
	if lazy == nil {
		return fmt.Errorf("No approved lazy.nvim revision is declared.")
	}
	nvim, mise, err := o.nvimCommand(ctx, root, paths)
	if err != nil {
		return err
	}
	if err := o.installLazy(ctx, paths, *lazy); err != nil {
		return err
	}
	declared := map[string]bool{}
	synced := true
	for _, dep := range deps {
		path, err := nvimPluginPath(paths, dep)
		if err != nil {
			return err
		}
		declared[filepath.Base(path)] = true
		if dep.Name == "folke/lazy.nvim" {
			continue
		}
		if exists, err := present(path); err != nil {
			return err
		} else if exists {
			entry, err := os.Lstat(path)
			if err != nil {
				return err
			}
			git, err := os.Lstat(path + "/.git")
			if !entry.IsDir() || err != nil || !git.IsDir() {
				return fmt.Errorf("Neovim plugin path is not an approved Git checkout; preserving it: %s", path)
			}
		}
		info, err := os.Stat(path + "/.git")
		if err == nil && info.IsDir() {
			changes, err := o.gitTrackedChanges(ctx, path, true)
			if err != nil {
				return fmt.Errorf("Could not inspect Neovim plugin checkout: %s: %w", path, err)
			}
			if changes != "" {
				return fmt.Errorf("Neovim plugin checkout was modified; preserving it: %s. Remove it, then retry to restore the approved revision.", path)
			}
		}
		if err != nil || !info.IsDir() {
			synced = false
			continue
		}
		head, err := o.gitHead(ctx, path)
		if err != nil || head != dep.Version {
			synced = false
		}
	}
	lazyDir := strings.TrimSuffix(paths.Data, "/selfishell") + "/nvim/lazy"
	entries, err := os.ReadDir(lazyDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if !declared[entry.Name()] {
			synced = false
		}
	}
	if !synced {
		syncLog, err := o.runNvim(ctx, root, nvim, mise, "--headless", `+lua local ok, message = pcall(vim.cmd, "Lazy! sync"); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end`, "+qa")
		if err != nil {
			return fmt.Errorf("Could not install Neovim plugins: %w", err)
		}
		verificationError := func(err error) error {
			if syncLog != "" {
				fmt.Fprint(o.Process.Err, syncLog)
			}
			return err
		}
		for _, dep := range deps {
			path, err := nvimPluginPath(paths, dep)
			if err != nil {
				return err
			}
			git, err := os.Stat(path + "/.git")
			if err != nil || !git.IsDir() {
				return verificationError(fmt.Errorf("Neovim plugin checkout is missing after sync: %s", dep.Name))
			}
			head, err := o.gitHead(ctx, path)
			if err != nil {
				return verificationError(fmt.Errorf("Could not inspect Neovim plugin after sync: %s: %w", dep.Name, err))
			}
			if head != dep.Version {
				return verificationError(fmt.Errorf("Neovim plugin revision does not match after sync: %s", dep.Name))
			}
		}
	}
	if _, err := o.runNvim(ctx, root, nvim, mise, "--headless", `+lua local ok, done = pcall(function() return require("nvim-treesitter").update():wait(300000) end); if not (ok and done) then vim.cmd("cquit") end`, "+qa"); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		o.warn("Could not update Tree-sitter parsers; run :TSUpdate in Neovim to retry.")
	}
	return nil
}
