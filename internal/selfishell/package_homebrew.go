package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func (o *PackageOperation) brewPath() string {
	if path, err := o.Process.lookPath("brew"); err == nil {
		return path
	}
	return ""
}

func (o *PackageOperation) activateBrew() bool {
	locations := o.brewLocations
	if locations == nil {
		locations = []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"}
	}
	for _, candidate := range locations {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		bin := filepath.Dir(candidate)
		old := envValue(o.Process.environment(), "PATH")
		o.Process = withEnvironment(o.Process, map[string]string{"PATH": bin + string(os.PathListSeparator) + old})
		return true
	}
	return false
}

func (o *PackageOperation) ensureBrew(ctx context.Context) error {
	if o.brewPath() != "" {
		return nil
	}
	if o.activateBrew() && o.brewPath() != "" {
		return nil
	}
	color, reset := o.color(o.Process.Out, "\x1b[36m")
	o.Process.progress.pause()
	fmt.Fprintf(o.Process.Out, "%sInstalling Homebrew%s\n", color, reset)
	var script bytes.Buffer
	p := o.Process
	p.Out = &script
	code, err := p.Curl(ctx, "transfer", "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh")
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("Homebrew installer download exited with status %d", code)
	}
	p = o.Process
	p.foreground = true
	if code, err := p.Run(ctx, "/bin/bash", "-c", script.String()); err != nil || code != 0 {
		return fmt.Errorf("Homebrew installer failed (exit %d): %v", code, err)
	}
	if o.brewPath() == "" {
		o.activateBrew()
	}
	if o.brewPath() == "" {
		return fmt.Errorf("Homebrew installation failed.")
	}
	return nil
}

func (o *PackageOperation) brewCache(manager string) (*map[string]bool, *bool) {
	if manager == "cask" {
		return &o.brewCasks, &o.brewCasksReady
	}
	return &o.brewFormulae, &o.brewFormulaeReady
}

// brewInventory lists each kind once per operation. With brewBothKinds, the
// first request lists the other kind concurrently; a failed concurrent
// listing is not cached, so that kind's own request runs it again.
func (o *PackageOperation) brewInventory(ctx context.Context, manager string) (map[string]bool, error) {
	inventory, ready := o.brewCache(manager)
	if *ready {
		return *inventory, nil
	}
	other := "cask"
	if manager == "cask" {
		other = "formula"
	}
	otherInventory, otherReady := o.brewCache(other)
	var otherNames map[string]bool
	var otherErr error
	var wg sync.WaitGroup
	listOther := o.brewBothKinds && !*otherReady
	if listOther {
		wg.Go(func() { otherNames, otherErr = o.brewList(ctx, other) })
	}
	names, err := o.brewList(ctx, manager)
	wg.Wait()
	if listOther && otherErr == nil {
		*otherInventory, *otherReady = otherNames, true
	}
	if err != nil {
		return nil, err
	}
	*inventory, *ready = names, true
	return names, nil
}

func (o *PackageOperation) brewList(ctx context.Context, manager string) (map[string]bool, error) {
	var out bytes.Buffer
	p := o.Process
	p.In, p.Out, p.Err = nil, &out, io.Discard // may run concurrently with the other kind
	code, err := p.Run(ctx, "brew", "list", "--"+manager)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if code != 0 {
		out.Reset()
	}
	names := make(map[string]bool)
	for _, name := range strings.Split(out.String(), "\n") {
		if name != "" {
			names[name] = true
		}
	}
	return names, nil
}

// InstallHomebrew inventories each package kind once per operation. Missing
// required Homebrew is bootstrapped with the official installer script.
func (o *PackageOperation) InstallHomebrew(ctx context.Context, requirement, manager string, dryRun bool, names ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	if err := validRequirement(requirement); err != nil {
		return err
	}
	if manager != "formula" && manager != "cask" {
		return fmt.Errorf("invalid Homebrew manager: %s", manager)
	}
	if dryRun {
		color, reset := o.color(o.Process.Out, "\x1b[36m")
		fmt.Fprintf(o.Process.Out, "%sWould install %s Homebrew %s:%s %s\n", color, requirement, manager, reset, strings.Join(names, " "))
		return nil
	}
	if o.brewPath() == "" && requirement == "optional" {
		o.activateBrew()
	}
	if o.brewPath() == "" && requirement == "required" {
		if err := o.ensureBrew(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Homebrew installation failed: %w", err)
		}
	}
	if o.brewPath() == "" {
		return o.optionalFailure(requirement, "Homebrew is required to install packages.", names)
	}
	installed, err := o.brewInventory(ctx, manager)
	if err != nil {
		return err
	}
	var missing []string
	for _, name := range names {
		if !installed[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	args := []string{"install"}
	if manager == "cask" {
		args = append(args, "--cask")
	}
	args = append(args, missing...)
	p := o.Process
	if p.Env == nil {
		p.Env = os.Environ()
	}
	p.Env = append(append([]string{}, p.Env...), "HOMEBREW_NO_ASK=1")
	p.foreground = manager == "cask"
	code, err := p.Run(ctx, "brew", args...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || code != 0 {
		kind := "formulae"
		if manager == "cask" {
			kind = "casks"
		}
		return o.optionalFailure(requirement, fmt.Sprintf("Could not install %s Homebrew %s: %s", requirement, kind, strings.Join(missing, " ")), missing)
	}
	for _, name := range missing {
		installed[name] = true
	}
	if o.Process.progress != nil {
		o.report(reportSuccess, "Installed Homebrew %s: %s", manager, strings.Join(missing, " "))
	}
	return nil
}
