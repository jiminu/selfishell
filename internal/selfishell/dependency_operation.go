package selfishell

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InstallDirect synchronizes one declared direct package. One operation caches
// its manifest so zinit and its plugins use the same approved records.
func (o *PackageOperation) InstallDirect(ctx context.Context, paths Paths, manifest, requirement, name, platform, arch string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validRequirement(requirement); err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(o.Process.Out, "Would sync %s direct package: %s\n", requirement, name)
		return nil
	}
	if err := o.loadDependencies(manifest); err != nil {
		return err
	}
	depPlatform := platform
	if platform == "ubuntu" || platform == "ubuntu-wsl" {
		depPlatform = "linux"
	}
	var dep *Dependency
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
	if err := o.installDependency(ctx, paths, *dep); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return o.optionalFailure(requirement, err.Error(), []string{name})
	}
	if name == "zinit" {
		if err := o.InstallZinitPlugins(ctx, paths, manifest, false); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return o.optionalFailure(requirement, err.Error(), []string{name})
		}
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

func dependencyTarget(dep Dependency, paths Paths) (string, error) {
	if !filepath.IsLocal(dep.Target) || dep.Target == "." || strings.HasPrefix(dep.Target, "-") {
		return "", fmt.Errorf("invalid dependency target: %s", dep.Target)
	}
	if strings.HasPrefix(dep.Target, ".local/share/") {
		return strings.TrimSuffix(paths.Data, "/selfishell") + "/" + strings.TrimPrefix(dep.Target, ".local/share/"), nil
	}
	return os.Getenv("HOME") + "/" + dep.Target, nil
}

func present(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (o *PackageOperation) installDependency(ctx context.Context, paths Paths, dep Dependency) error {
	if dep.Name == "." || filepath.Base(dep.Name) != dep.Name {
		return fmt.Errorf("invalid dependency name: %s", dep.Name)
	}
	target, err := dependencyTarget(dep, paths)
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
		exists, err := present(target)
		if err != nil {
			return err
		}
		if exists {
			if o.validDirect(ctx, dep, target, false) {
				fmt.Fprintf(o.Process.Out, "Externally installed; preserving: %s\n", target)
				return nil
			}
			return fmt.Errorf("An existing %s is not a usable %s installation; leaving it in place.", target, dep.Name)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Staging on the destination filesystem makes final rename atomic.
	if err := makeRawDir(rawParent(target)); err != nil {
		return err
	}
	if o.dependencyFault != nil {
		if err := o.dependencyFault("stage"); err != nil {
			return err
		}
	}
	stageDir, err := os.MkdirTemp(rawParent(target), ".selfishell-dependency.*")
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
	old, moved, err := moveAside(target)
	if err != nil {
		return err
	}
	if o.dependencyFault != nil {
		if err := o.dependencyFault("activate"); err != nil {
			if moved {
				if restoreErr := restoreEmpty(old, target); restoreErr != nil {
					return fmt.Errorf("%v; restore: %w", err, restoreErr)
				}
			}
			return err
		}
	}
	if occupied, err := present(target); err != nil || occupied {
		if moved {
			if restoreErr := restoreEmpty(old, target); restoreErr != nil {
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
			if restoreErr := restoreEmpty(old, target); restoreErr != nil {
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
		err = replaceRaw(state, []byte(dep.Version+"\n"), 0600)
	}
	if err != nil {
		// Remove only the inode this transaction installed. A concurrent replacement
		// is user data and cannot be removed to make restoration possible.
		if current, e := os.Lstat(target); e == nil && os.SameFile(installed, current) {
			os.RemoveAll(target)
		}
		if moved {
			if restoreErr := restoreEmpty(old, target); restoreErr != nil {
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
	fmt.Fprintf(o.Process.Out, "%s approved dependency: %s %s\n", verb, dep.Name, dep.Version)
	return nil
}

func siblingName(target, suffix string) (string, error) {
	for i := 0; i < 20; i++ {
		var salt [8]byte
		if _, err := rand.Read(salt[:]); err != nil {
			return "", err
		}
		name := target + suffix + hex.EncodeToString(salt[:])
		if exists, err := present(name); err != nil {
			return "", err
		} else if !exists {
			return name, nil
		}
	}
	return "", fmt.Errorf("no unused sibling for %s", target)
}
func moveAside(target string) (string, bool, error) {
	exists, err := present(target)
	if err != nil || !exists {
		return "", false, err
	}
	old, err := siblingName(target, ".previous.")
	if err != nil {
		return "", false, err
	}
	if err := os.Rename(target, old); err != nil {
		return "", false, err
	}
	return old, true, nil
}
func restoreEmpty(old, target string) error {
	exists, err := present(target)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("cannot restore over occupied target: %s", target)
	}
	return os.Rename(old, target)
}

func (o *PackageOperation) stageDownload(ctx context.Context, dep Dependency, stage string) error {
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
	if dep.Marker == "raw" {
		if err := os.Rename(archive, stage); err != nil {
			return err
		}
		return os.Chmod(stage, 0755)
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
func (o *PackageOperation) stageGit(ctx context.Context, dep Dependency, stage string) error {
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
func (o *PackageOperation) validDirect(ctx context.Context, dep Dependency, target string, managed bool) bool {
	info, err := os.Stat(target)
	if err != nil {
		return false
	}
	if managed {
		link, err := os.Lstat(target)
		if err != nil || link.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	switch dep.Kind {
	case "download":
		return info.Mode().IsRegular() && info.Mode()&0111 != 0
	case "git":
		if !info.IsDir() || !filepath.IsLocal(dep.Marker) {
			return false
		}
		if _, err := os.Stat(target + "/" + dep.Marker); err != nil {
			return false
		}
		if !managed {
			return true
		}
		if _, err := os.Stat(target + "/.git"); err != nil {
			return false
		}
		if dep.Checksum != "-" {
			head, err := o.commandOutput(ctx, "git", "-C", target, "rev-parse", "HEAD")
			if err != nil || head != dep.Checksum {
				return false
			}
		}
		status, err := o.commandOutput(ctx, "git", "-C", target, "status", "--porcelain", "--untracked-files=no")
		return err == nil && status == ""
	}
	return false
}
