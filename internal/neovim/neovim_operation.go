// Package neovim synchronizes the pinned Neovim plugins, the lazy.nvim
// bootstrap checkout and the default LSP servers.
package neovim

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

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
)

// Operation synchronizes the pinned Neovim plugins and LSP servers.
// Process.Progress, when set, is the active spinner: a nil spinner must be
// stored as a nil interface. Report and Warn render its notes. Mise and
// MiseProcess resolve the release's mise, and Cache keeps the last mise
// resolution of nvim across operations.
type Operation struct {
	Process     process.Process
	Report      report.Reporter
	Warn        func(message string)
	Mise        func(paths managed.Paths) (string, error)
	MiseProcess func(root string) process.Process
	Cache       *Cache
}

// Cache holds the last successful "mise which nvim" and its mise/root.
type Cache struct{ path, key string }

var gitHashPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func gitRepositoryProcess(base process.Process) process.Process {
	p := process.WithEnvironment(base, map[string]string{"GIT_DIR": ".git", "GIT_WORK_TREE": ".", "GIT_OPTIONAL_LOCKS": "0"})
	p.RepoScopedGit = true
	return p
}

// GitHead returns a checkout's HEAD commit from its .git files, else from git
// scoped to that repository.
func GitHead(ctx context.Context, base process.Process, dir string) (string, error) {
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
	p := gitRepositoryProcess(base)
	var out, stderr bytes.Buffer
	p.In, p.Out, p.Err = nil, &out, &stderr // may run as a concurrent probe
	code, err := p.Run(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git rev-parse exited %d: %s", code, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func gitTrackedChanges(ctx context.Context, base process.Process, dir string, excludeTags bool) (string, error) {
	return gitChanges(ctx, base, dir, excludeTags, false)
}

func gitChanges(ctx context.Context, base process.Process, dir string, excludeTags, includeUntracked bool) (string, error) {
	p := gitRepositoryProcess(base)
	var out, stderr bytes.Buffer
	p.In, p.Out, p.Err = nil, &out, &stderr // may run as a concurrent probe
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

// PluginPath is where lazy.nvim keeps dep's checkout; lazy.nvim itself lives
// under Selfishell's data directory.
func PluginPath(paths managed.Paths, dep managed.Dependency) (string, error) {
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

// Locate resolves Neovim through the release's mise, then PATH, then
// ~/.local/bin/nvim, and returns the mise it used, if any.
func (o Operation) Locate(ctx context.Context, root string, paths managed.Paths) (string, string, error) {
	mise, _ := o.Mise(paths)
	if mise != "" {
		resolved, err := o.miseWhichNvim(ctx, root, mise)
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if err == nil && executableFile(resolved) {
			return resolved, mise, nil
		}
	}
	if path, err := o.Process.LookPath("nvim"); err == nil {
		return path, mise, nil
	}
	home := o.Process.Environment().Get("HOME")
	path := home + "/.local/bin/nvim"
	if executableFile(path) {
		return path, mise, nil
	}
	return "", mise, fmt.Errorf("Could not locate Neovim after installing the development environment.")
}

// miseWhichNvim reuses this operation's last resolution only while it is
// still an executable file; a failed resolution is never reused.
func (o Operation) miseWhichNvim(ctx context.Context, root, mise string) (string, error) {
	key := mise + "\x00" + root
	if o.Cache.path != "" && o.Cache.key == key && executableFile(o.Cache.path) {
		return o.Cache.path, nil
	}
	resolved, err := process.Output(ctx, o.MiseProcess(root), mise, "-C", root+"/config/shared", "which", "nvim")
	*o.Cache = Cache{}
	if err == nil && executableFile(resolved) {
		*o.Cache = Cache{resolved, key}
	}
	return resolved, err
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

// Run runs nvim, through mise when set, and prints its log on failure.
func (o Operation) Run(ctx context.Context, root, nvim, mise string, args ...string) (string, error) {
	p := o.Process
	name := nvim
	if mise != "" {
		p = o.MiseProcess(root)
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
			if o.Process.Progress != nil {
				o.Process.Progress.Pause()
			}
			fmt.Fprint(o.Process.Err, log.String())
		}
		if err != nil {
			return log.String(), err
		}
		return log.String(), fmt.Errorf("Neovim exited %d", code)
	}
	return log.String(), nil
}

// gitStatus holds a dirty check that ran in a concurrent batch ahead of the
// sequential scan consuming it.
type gitStatus struct {
	changes string
	err     error
}

// gitCheckout reports whether dir and its .git are real directories.
func gitCheckout(dir string) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	git, err := os.Lstat(dir + "/.git")
	return err == nil && git.IsDir()
}

// pluginPathError preserves an existing plugin path that is not a real
// directory holding a real .git directory.
func pluginPathError(path string) error {
	if exists, err := managed.Present(path); err != nil || !exists {
		return err
	}
	entry, err := os.Lstat(path)
	if err != nil {
		return err
	}
	git, err := os.Lstat(path + "/.git")
	if !entry.IsDir() || err != nil || !git.IsDir() {
		return fmt.Errorf("Neovim plugin path is not an approved Git checkout; preserving it: %s", path)
	}
	return nil
}

// installLazy uses inspected, when non-nil, as the checkout's dirty check.
func (o Operation) installLazy(ctx context.Context, paths managed.Paths, dep managed.Dependency, inspected *gitStatus) error {
	if !gitHashPattern.MatchString(dep.Version) {
		return fmt.Errorf("Invalid approved lazy.nvim revision: %s", dep.Version)
	}
	target, _ := PluginPath(paths, dep)
	previously := false
	if exists, err := managed.Present(target); err != nil && !errors.Is(err, syscall.ENOTDIR) {
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
		var changes string
		if inspected != nil {
			changes, err = inspected.changes, inspected.err
		} else {
			changes, err = gitChanges(ctx, o.Process, target, false, true)
		}
		if err != nil {
			return err
		}
		if changes != "" {
			return fmt.Errorf("lazy.nvim checkout was modified; preserving it: %s", target)
		}
		head, err := GitHead(ctx, o.Process, target)
		if err != nil {
			return err
		}
		if head == dep.Version {
			return nil
		}
		previously = true
	}
	if err := managed.MakeRawDir(managed.RawParent(target)); err != nil {
		return fmt.Errorf("Could not create Neovim plugin directory: %s: %w", managed.RawParent(target), err)
	}
	stage, err := managed.SiblingName(target, ".tmp.")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil {
			o.Warn(fmt.Sprintf("Could not clean up lazy.nvim staging path %s: %v", stage, err))
		}
	}()
	if _, err := process.Output(ctx, o.Process, "git", "clone", "--quiet", "--filter=blob:none", "--", dep.Source, stage); err != nil {
		return err
	}
	if _, err := process.Output(ctx, o.Process, "git", "-C", stage, "checkout", "--quiet", "--detach", dep.Version); err != nil {
		return err
	}
	if head, err := GitHead(ctx, o.Process, stage); err != nil || head != dep.Version {
		return fmt.Errorf("lazy.nvim revision does not match approved pin: %s", dep.Version)
	}
	old, moved, err := managed.MoveAside(target)
	if err != nil {
		return err
	}
	if occupied, err := managed.Present(target); err != nil || occupied {
		if err == nil {
			err = fmt.Errorf("lazy.nvim target occupied: %s", target)
		}
		if moved {
			return restoreLazyOnFailure(old, target, err)
		}
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		if moved {
			return restoreLazyOnFailure(old, target, err)
		}
		return err
	}
	if moved {
		if err := os.RemoveAll(old); err != nil {
			o.Warn(fmt.Sprintf("Could not clean up previous lazy.nvim checkout %s: %v", old, err))
		}
	}
	verb := "Installed"
	if previously {
		verb = "Updated"
	}
	o.Report.Report("Tools", report.Success, "%s approved lazy.nvim revision: %s", verb, dep.Version)
	return nil
}

func restoreLazyOnFailure(old, target string, primary error) error {
	if err := managed.RestoreEmpty(old, target); err != nil {
		return errors.Join(primary, fmt.Errorf("Could not restore previous lazy.nvim checkout %s: %w", old, err))
	}
	return primary
}

// InstallPlugins syncs the declared plugin pins and parser updates with the
// release environment; dependencies may be nil for a dry run.
func (o Operation) InstallPlugins(ctx context.Context, root string, paths managed.Paths, dependencies []managed.Dependency, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dryRun {
		for _, line := range []string{"Would sync declared Neovim plugins.", "Would sync lazy.nvim bootstrap repository.", "Would update installed Tree-sitter parsers."} {
			o.Report.Report("Tools", report.Preview, "%s", line)
		}
		return nil
	}
	var deps []managed.Dependency
	var lazy *managed.Dependency
	for _, dep := range dependencies {
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
	nvim, mise, err := o.Locate(ctx, root, paths)
	if err != nil {
		return err
	}
	// Validate plugin paths in declaration order up to the first failure, run
	// the dirty checks (and lazy.nvim's) concurrently, then report exactly as
	// the sequential scan did: lazy.nvim, each plugin, then the path failure.
	type pluginCheck struct {
		path, version string
		git           bool
		status        gitStatus
	}
	declared := map[string]bool{}
	var checks []pluginCheck
	var invalid error
	for _, dep := range deps {
		path, err := PluginPath(paths, dep)
		if err != nil {
			invalid = err
			break
		}
		if dep.Name == "folke/lazy.nvim" {
			continue
		}
		declared[filepath.Base(path)] = true
		if invalid = pluginPathError(path); invalid != nil {
			break
		}
		info, err := os.Stat(path + "/.git")
		checks = append(checks, pluginCheck{path: path, version: dep.Version, git: err == nil && info.IsDir()})
	}
	lazyPath, _ := PluginPath(paths, *lazy)
	var lazyStatus *gitStatus
	if gitCheckout(lazyPath) {
		lazyStatus = &gitStatus{}
	}
	process.ProbeParallel(len(checks)+1, func(i int) {
		if i < len(checks) {
			if check := &checks[i]; check.git {
				check.status.changes, check.status.err = gitTrackedChanges(ctx, o.Process, check.path, true)
			}
		} else if lazyStatus != nil {
			lazyStatus.changes, lazyStatus.err = gitChanges(ctx, o.Process, lazyPath, false, true)
		}
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := o.installLazy(ctx, paths, *lazy, lazyStatus); err != nil {
		return err
	}
	synced := true
	for _, check := range checks {
		if !check.git {
			synced = false
			continue
		}
		if check.status.err != nil {
			return fmt.Errorf("Could not inspect Neovim plugin checkout: %s: %w", check.path, check.status.err)
		}
		if check.status.changes != "" {
			return fmt.Errorf("Neovim plugin checkout was modified; preserving it: %s. Remove it, then retry to restore the approved revision.", check.path)
		}
		head, err := GitHead(ctx, o.Process, check.path)
		if err != nil || head != check.version {
			synced = false
		}
	}
	if invalid != nil {
		return invalid
	}
	lazyDir := strings.TrimSuffix(paths.Data, "/selfishell") + "/nvim/lazy"
	entries, err := os.ReadDir(lazyDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	probe := gitRepositoryProcess(o.Process)
	for _, entry := range entries {
		// lazy.nvim cleans only real directories, excluding its legacy readme
		// cache. It leaves files and symlinks alone.
		if declared[entry.Name()] || entry.Name() == "readme" || !entry.IsDir() {
			continue
		}
		path := lazyDir + "/" + entry.Name()
		if err := pluginPathError(path); err != nil {
			return err
		}
		// Cleanup removes the whole checkout, including ignored and untracked
		// files that ordinary pin updates deliberately leave untouched. Lazy's
		// helptags generates doc/tags in nearly every checkout.
		changes, err := process.Output(ctx, probe, "git", "-C", path, "status", "--porcelain", "--untracked-files=all", "--ignored", "--", ":(exclude)doc/tags")
		if err != nil {
			return fmt.Errorf("Could not inspect Neovim plugin checkout: %s: %w", path, err)
		}
		if changes != "" {
			return fmt.Errorf("Neovim plugin checkout contains local changes or extra files; preserving it: %s. Move it out of the plugin directory, then retry.", path)
		}
		synced = false
	}
	if !synced {
		syncLog, err := o.Run(ctx, root, nvim, mise, "--headless", `+lua local ok, message = pcall(vim.cmd, "Lazy! sync"); if not ok then vim.api.nvim_err_writeln(message); vim.cmd("cquit") end`, "+qa")
		if err != nil {
			return fmt.Errorf("Could not install Neovim plugins: %w", err)
		}
		verificationError := func(err error) error {
			if syncLog != "" {
				if o.Process.Progress != nil {
					o.Process.Progress.Pause()
				}
				fmt.Fprint(o.Process.Err, syncLog)
			}
			return err
		}
		for _, dep := range deps {
			path, err := PluginPath(paths, dep)
			if err != nil {
				return err
			}
			git, err := os.Stat(path + "/.git")
			if err != nil || !git.IsDir() {
				return verificationError(fmt.Errorf("Neovim plugin checkout is missing after sync: %s", dep.Name))
			}
			head, err := GitHead(ctx, o.Process, path)
			if err != nil {
				return verificationError(fmt.Errorf("Could not inspect Neovim plugin after sync: %s: %w", dep.Name, err))
			}
			if head != dep.Version {
				return verificationError(fmt.Errorf("Neovim plugin revision does not match after sync: %s", dep.Name))
			}
		}
	}
	if !synced && o.Process.Progress != nil {
		o.Report.Report("Tools", report.Success, "Synchronized Neovim plugins: approved revisions")
	}
	if _, err := o.Run(ctx, root, nvim, mise, "--headless", `+lua local ok, done = pcall(function() return require("nvim-treesitter").update():wait(300000) end); if not (ok and done) then vim.cmd("cquit") end`, "+qa"); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		o.Warn("Could not update Tree-sitter parsers; run :TSUpdate in Neovim to retry.")
	}
	return nil
}
