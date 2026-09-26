package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		old := ""
		for _, entry := range o.Process.Env {
			if strings.HasPrefix(entry, "PATH=") {
				old = strings.TrimPrefix(entry, "PATH=")
			}
		}
		if o.Process.Env == nil {
			old = os.Getenv("PATH")
			o.Process.Env = os.Environ()
		}
		updated := false
		for i, entry := range o.Process.Env {
			if strings.HasPrefix(entry, "PATH=") {
				o.Process.Env[i] = "PATH=" + bin + string(os.PathListSeparator) + old
				updated = true
				break
			}
		}
		if !updated {
			o.Process.Env = append(o.Process.Env, "PATH="+bin)
		}
		return true
	}
	return false
}

func (o *PackageOperation) ensureBrew(ctx context.Context) error {
	if o.brewPath() != "" {
		return nil
	}
	color, reset := o.color(o.Process.Out, "\x1b[36m")
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
	if err := o.run(ctx, "/bin/bash", "-c", script.String()); err != nil {
		return err
	}
	if o.brewPath() == "" {
		o.activateBrew()
	}
	if o.brewPath() == "" {
		return fmt.Errorf("Homebrew installation failed.")
	}
	return nil
}

func (o *PackageOperation) brewInventory(ctx context.Context, manager string) (map[string]bool, error) {
	ready := &o.brewFormulaeReady
	inventory := &o.brewFormulae
	if manager == "cask" {
		ready = &o.brewCasksReady
		inventory = &o.brewCasks
	}
	if *ready {
		return *inventory, nil
	}
	var out bytes.Buffer
	p := o.Process
	p.Out = &out
	p.Err = io.Discard
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
	*inventory = make(map[string]bool)
	for _, name := range strings.Split(out.String(), "\n") {
		if name != "" {
			(*inventory)[name] = true
		}
	}
	*ready = true
	return *inventory, nil
}

// InstallHomebrew inventories each package kind once per operation. Missing
// required Homebrew is bootstrapped with the official installer script.
func (o *PackageOperation) InstallHomebrew(ctx context.Context, requirement, manager string, dryRun bool, names ...string) error {
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
	return nil
}
