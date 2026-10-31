package selfishell

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
	"github.com/jiminu/selfishell/internal/terminal"
)

// InstallDirect synchronizes one declared direct package. One operation caches
// its manifest so every package it installs uses the same approved records.
func (o *PackageOperation) InstallDirect(ctx context.Context, paths managed.Paths, manifest, requirement, name, platform, arch string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validRequirement(requirement); err != nil {
		return err
	}
	if dryRun {
		o.report(report.Preview, "Would sync %s direct package: %s", requirement, name)
		return nil
	}
	if err := o.loadDependencies(manifest); err != nil {
		return err
	}
	depPlatform := dependencyPlatform(platform)
	var dep *managed.Dependency
	for i := range o.dependencies {
		d := &o.dependencies[i]
		if d.Name == name && (d.Platform == "all" || d.Platform == depPlatform) && (d.Arch == "all" || d.Arch == arch) && (d.Kind == "download" || d.Kind == "git") {
			dep = d
			break
		}
	}
	if dep == nil {
		return o.optionalFailure(requirement, fmt.Sprintf("No approved dependency entry for %s (%s/%s).", name, depPlatform, arch), []string{name})
	}
	install := o.installDependency
	if dep.Marker == "font" {
		install = o.installWindowsFont
	}
	if err := install(ctx, paths, *dep); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return o.optionalFailure(requirement, err.Error(), []string{name})
	}
	return nil
}

func (o *PackageOperation) loadDependencies(manifest string) error {
	if o.dependencyManifest == manifest && o.dependencies != nil {
		return nil
	}
	deps, err := ReadDependencies(manifest)
	if err != nil {
		return err
	}
	o.dependencyManifest, o.dependencies = manifest, deps
	return nil
}

func dependencyTarget(dep managed.Dependency, paths managed.Paths, env process.Environment) (string, error) {
	if !filepath.IsLocal(dep.Target) || dep.Target == "." || strings.HasPrefix(dep.Target, "-") {
		return "", fmt.Errorf("invalid dependency target: %s", dep.Target)
	}
	if dep.Marker == "font" {
		choice, err := terminal.ReadWindowsChoice(paths)
		if err != nil {
			return "", err
		}
		return terminal.FontTarget(choice, dep)
	}
	if strings.HasPrefix(dep.Target, ".local/share/") {
		return strings.TrimSuffix(paths.Data, "/selfishell") + "/" + strings.TrimPrefix(dep.Target, ".local/share/"), nil
	}
	return env.Get("HOME") + "/" + dep.Target, nil
}

func (o *PackageOperation) installDependency(ctx context.Context, paths managed.Paths, dep managed.Dependency) error {
	if dep.Name == "." || filepath.Base(dep.Name) != dep.Name {
		return fmt.Errorf("invalid dependency name: %s", dep.Name)
	}
	target, err := dependencyTarget(dep, paths, o.Process.Environment())
	if err != nil {
		return err
	}
	state := paths.State + "/dependencies/" + dep.Name
	if info, err := os.Lstat(state); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("invalid dependency state: %s", state)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, readErr := os.ReadFile(state)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	oldVersion := strings.TrimSpace(string(data))
	owned := oldVersion != ""
	if owned && oldVersion == dep.Version && o.validDirect(ctx, dep, target, true) {
		o.UnchangedCount++
		return nil
	}
	if !owned {
		exists, err := managed.Present(target)
		if err != nil {
			return err
		}
		if exists {
			if o.validDirect(ctx, dep, target, false) {
				o.report(report.Info, "Externally installed; preserving: %s", target)
				return nil
			}
			return fmt.Errorf("An existing %s is not a usable %s installation; leaving it in place.", target, dep.Name)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Staging on the destination filesystem makes final rename atomic.
	if err := managed.MakeRawDir(managed.RawParent(target)); err != nil {
		return err
	}
	stageDir, err := os.MkdirTemp(managed.RawParent(target), ".selfishell-dependency.*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)
	stage := stageDir + "/target"
	switch dep.Kind {
	case "download":
		err = o.stageDownload(ctx, dep, stage)
	case "git":
		err = o.stageGit(ctx, dep, stage)
	default:
		err = fmt.Errorf("Unknown dependency type: %s", dep.Kind)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	old, moved, err := managed.MoveAside(target)
	if err != nil {
		return err
	}
	if o.dependencyFault != nil {
		if err := o.dependencyFault("activate"); err != nil {
			if moved {
				if restoreErr := managed.RestoreEmpty(old, target); restoreErr != nil {
					return fmt.Errorf("%v; restore: %w", err, restoreErr)
				}
			}
			return err
		}
	}
	if occupied, err := managed.Present(target); err != nil || occupied {
		if moved {
			if restoreErr := managed.RestoreEmpty(old, target); restoreErr != nil {
				return fmt.Errorf("target occupied before activation: %s; restore: %w", target, restoreErr)
			}
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("target occupied before activation: %s", target)
	}
	if err := os.Rename(stage, target); err != nil {
		if moved {
			if restoreErr := managed.RestoreEmpty(old, target); restoreErr != nil {
				return fmt.Errorf("%v; restore: %w", err, restoreErr)
			}
		}
		return err
	}
	installed, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if o.dependencyFault != nil {
		err = o.dependencyFault("state")
	}
	if err == nil {
		err = managed.WriteAtomic(state, []byte(dep.Version+"\n"), 0600)
	}
	if err != nil {
		// Remove only the inode this transaction installed. A concurrent replacement
		// is user data and cannot be removed to make restoration possible.
		if current, e := os.Lstat(target); e == nil && os.SameFile(installed, current) {
			os.RemoveAll(target)
		}
		if moved {
			if restoreErr := managed.RestoreEmpty(old, target); restoreErr != nil {
				return fmt.Errorf("%v; restore: %w", err, restoreErr)
			}
		}
		return err
	}
	if moved {
		os.RemoveAll(old)
	}
	verb := "Installed"
	if owned && oldVersion != dep.Version {
		verb = "Updated"
	}
	o.report(report.Success, "%s approved dependency: %s %s", verb, dep.Name, dep.Version)
	return nil
}

func (o *PackageOperation) stageDownload(ctx context.Context, dep managed.Dependency, stage string) error {
	archive := stage + ".archive"
	code, err := o.Process.Curl(ctx, "transfer", dep.Source, "-o", archive)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("download %s: curl exited %d", dep.Name, code)
	}
	input, err := os.Open(archive)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, input)
	input.Close()
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != dep.Checksum {
		return fmt.Errorf("Checksum mismatch for %s %s.", dep.Name, dep.Version)
	}
	if dep.Marker == "raw" || dep.Marker == "font" {
		if err := os.Rename(archive, stage); err != nil {
			return err
		}
		mode := os.FileMode(0755)
		if dep.Marker == "font" {
			mode = 0644
		}
		return os.Chmod(stage, mode)
	}
	if !filepath.IsLocal(dep.Marker) {
		return fmt.Errorf("unsafe archive marker: %s", dep.Marker)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var payload []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !filepath.IsLocal(hdr.Name) || hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA && hdr.Typeflag != tar.TypeDir {
			return fmt.Errorf("unsafe archive entry: %s", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Name == dep.Marker {
			if payload != nil {
				return fmt.Errorf("duplicate archive marker: %s", dep.Marker)
			}
			if hdr.Size > 100<<20 {
				return fmt.Errorf("archive marker too large: %s", dep.Marker)
			}
			payload, err = io.ReadAll(tr)
			if err != nil {
				return err
			}
		}
	}
	if payload == nil {
		return fmt.Errorf("Expected executable missing from %s archive.", dep.Name)
	}
	return os.WriteFile(stage, payload, 0755)
}

func (o *PackageOperation) commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	p := o.Process
	p.Out, p.Err = &out, &stderr
	code, err := p.Run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("%s exited %d: %s", name, code, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
func (o *PackageOperation) stageGit(ctx context.Context, dep managed.Dependency, stage string) error {
	if _, err := o.commandOutput(ctx, "git", "clone", "--quiet", "--", dep.Source, stage); err != nil {
		return err
	}
	if _, err := o.commandOutput(ctx, "git", "-C", stage, "checkout", "--quiet", "--detach", dep.Version); err != nil {
		return err
	}
	head, err := o.commandOutput(ctx, "git", "-C", stage, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if dep.Checksum != "-" && head != dep.Checksum {
		return fmt.Errorf("%s %s no longer points to its approved commit %s.", dep.Name, dep.Version, dep.Checksum)
	}
	if !filepath.IsLocal(dep.Marker) {
		return fmt.Errorf("unsafe dependency marker: %s", dep.Marker)
	}
	if _, err := os.Stat(stage + "/" + dep.Marker); err != nil {
		return fmt.Errorf("Expected marker missing from %s checkout: %w", dep.Name, err)
	}
	return nil
}

// directDownloadIntact is shared by install and diagnostics: a recorded raw
// download must still match its approved SHA-256, not merely be executable.
func directDownloadIntact(dep managed.Dependency, target string, info os.FileInfo, owned bool) bool {
	if !info.Mode().IsRegular() || (dep.Marker != "font" && info.Mode()&0111 == 0) {
		return false
	}
	if !owned || (dep.Marker != "font" && dep.Marker != "raw") {
		return true
	}
	file, err := os.Open(target)
	if err != nil {
		return false
	}
	h := sha256.New()
	_, err = io.Copy(h, file)
	closeErr := file.Close()
	return err == nil && closeErr == nil && hex.EncodeToString(h.Sum(nil)) == dep.Checksum
}

// directGitMarkerPresent requires the approved marker inside the checkout. The
// raw join keeps symlink/.. components that filepath.Join would clean.
func directGitMarkerPresent(dep managed.Dependency, target string, info os.FileInfo) bool {
	if !info.IsDir() || !filepath.IsLocal(dep.Marker) {
		return false
	}
	_, err := os.Stat(target + "/" + dep.Marker)
	return err == nil
}

func (o *PackageOperation) validDirect(ctx context.Context, dep managed.Dependency, target string, owned bool) bool {
	info, err := os.Stat(target)
	if err != nil {
		return false
	}
	if owned {
		link, err := os.Lstat(target)
		if err != nil || link.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	switch dep.Kind {
	case "download":
		return directDownloadIntact(dep, target, info, owned)
	case "git":
		if !directGitMarkerPresent(dep, target, info) {
			return false
		}
		if !owned {
			return true
		}
		if _, err := os.Stat(target + "/.git"); err != nil {
			return false
		}
		if dep.Checksum != "-" {
			head, err := o.gitHead(ctx, target)
			if err != nil || head != dep.Checksum {
				return false
			}
		}
		status, err := o.commandOutput(ctx, "git", "-C", target, "status", "--porcelain", "--untracked-files=no")
		return err == nil && status == ""
	}
	return false
}
