package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
	"github.com/jiminu/selfishell/internal/terminal"
)

func (o *PackageOperation) miseCommand(paths managed.Paths) (string, error) {
	if path, err := o.Process.LookPath("mise"); err == nil {
		return path, nil
	}
	home := o.Process.Environment().Get("HOME")
	if home == "" {
		return "", fmt.Errorf("HOME must be set")
	}
	path := home + "/.local/bin/mise"
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
		return path, nil
	}
	return "", fmt.Errorf("mise is unavailable")
}

func (o *PackageOperation) miseProcess(root string, offline bool) process.Process {
	shared := root + "/config/shared"
	set := map[string]string{"MISE_GLOBAL_CONFIG_FILE": shared + "/mise.toml"}
	if offline {
		set["MISE_OFFLINE"] = "1"
	}
	p := process.WithEnvironment(o.Process, set, "MISE_OFFLINE")
	p.Dir = shared
	return p
}

// trustManagedMiseConfig trusts the managed conf.d entry when it is a file or
// link. Callers choose the mise binary and the child's environment.
func trustManagedMiseConfig(ctx context.Context, p process.Process, mise string, paths managed.Paths) {
	link := strings.TrimSuffix(paths.Config, "/selfishell") + "/mise/conf.d/selfishell.toml"
	if info, err := os.Lstat(link); err == nil && (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		_, _ = p.Run(ctx, mise, "trust", link)
	}
}

// InstallMise synchronizes one required or optional group against release pins.
func (o *PackageOperation) InstallMise(ctx context.Context, root string, paths managed.Paths, requirement string, dryRun bool, names ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	if err := validRequirement(requirement); err != nil {
		return err
	}
	if dryRun {
		o.report(report.Preview, "Would sync %s mise tools: %s", requirement, strings.Join(names, " "))
		return nil
	}
	mise, err := o.miseCommand(paths)
	if err != nil {
		return o.optionalFailure(requirement, "mise is required to install mise-managed tools.", names)
	}
	trust := o.miseProcess(root, false)
	trust.Out, trust.Err = io.Discard, io.Discard
	trustManagedMiseConfig(ctx, trust, mise, paths)
	if err := ctx.Err(); err != nil {
		return err
	}
	shared := root + "/config/shared"
	check := o.miseProcess(root, true)
	check.Out, check.Err = io.Discard, io.Discard
	code, err := check.Run(ctx, mise, append([]string{"-C", shared, "-q", "install", "--dry-run-code"}, names...)...)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err == nil && code == 0 {
		return nil
	}
	install := o.miseProcess(root, false)
	code, err = install.Run(ctx, mise, append([]string{"-C", shared, "install"}, names...)...)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err != nil || code != 0 {
		return o.optionalFailure(requirement, fmt.Sprintf("Could not install %s mise tools: %s", requirement, strings.Join(names, " ")), names)
	}
	if progressOf(o.Process) != nil {
		o.report(report.Success, "Synchronized mise tools: %d\n  %s", len(names),
			strings.ReplaceAll(wrapWords(names, terminal.Width(o.Process.Out)-4), "\n", "\n  "))
	}
	return nil
}

func (o *PackageOperation) miseOutput(ctx context.Context, p process.Process, mise string, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	p.Out, p.Err = &out, &stderr
	code, err := p.Run(ctx, mise, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("mise exited %d: %s", code, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
