package selfishell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// PackageOperation owns state for one tools/configuration synchronization.
// Reuse it across required and optional Apt, Homebrew, and later direct/mise
// phases. SkippedOptional records names that could not be installed; callers
// can report them and skip cleanup while still completing optional groups.
// Use a fresh value for a new synchronization, including retries and rollback.
type PackageOperation struct {
	Process                           Process
	SkippedOptional                   []string
	UnchangedCount                    int
	dependencyManifest                string
	dependencies                      []Dependency
	dependencyFault                   func(string) error // focused filesystem-failure test seam
	aptUpdated                        bool
	brewFormulae, brewCasks           map[string]bool
	brewFormulaeReady, brewCasksReady bool
	uid                               func() int // package-private test seam; nil uses os.Geteuid.
	brewLocations                     []string   // package-private test seam; nil uses standard macOS locations.
}

func (o *PackageOperation) warn(message string) {
	color, reset := o.color(o.Process.Err, "\x1b[33m")
	fmt.Fprintf(o.Process.Err, "%sselfishell: warning:%s %s\n", color, reset, message)
}
func (o *PackageOperation) color(stream io.Writer, code string) (string, string) {
	noColor := os.Getenv("NO_COLOR")
	if o.Process.Env != nil {
		noColor = ""
		for _, entry := range o.Process.Env {
			if strings.HasPrefix(entry, "NO_COLOR=") {
				noColor = strings.TrimPrefix(entry, "NO_COLOR=")
			}
		}
	}
	if noColor != "" || !IsTerminal(stream) {
		return "", ""
	}
	return code, "\x1b[0m"
}
func (o *PackageOperation) fail(message string) error { return errors.New(message) }
func (o *PackageOperation) optionalFailure(requirement, message string, names []string) error {
	if requirement == "optional" {
		o.warn(message)
		o.SkippedOptional = append(o.SkippedOptional, names...)
		return nil
	}
	return o.fail(message)
}
func (o *PackageOperation) run(ctx context.Context, name string, args ...string) error {
	code, err := o.Process.Run(ctx, name, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s exited with status %d", name, code)
	}
	return nil
}

func validRequirement(requirement string) error {
	if requirement != "required" && requirement != "optional" {
		return fmt.Errorf("invalid package requirement: %s", requirement)
	}
	return nil
}

// InstallApt preserves argument boundaries and checks dpkg's actual status.
// A failed index update leaves aptUpdated false, so a later group can retry.
func (o *PackageOperation) InstallApt(ctx context.Context, requirement string, dryRun bool, names ...string) error {
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
		color, reset := o.color(o.Process.Out, "\x1b[36m")
		fmt.Fprintf(o.Process.Out, "%sWould install %s apt packages:%s %s\n", color, requirement, reset, strings.Join(names, " "))
		return nil
	}
	if _, err := o.Process.lookPath("apt-get"); err != nil {
		return o.optionalFailure(requirement, "apt-get is required to install packages.", names)
	}
	var missing []string
	for _, name := range names {
		var out strings.Builder
		p := o.Process
		p.Out = &out
		p.Err = io.Discard
		code, err := p.Run(ctx, "dpkg-query", "-W", "-f=${Status}\\n", name)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || code != 0 || !strings.HasSuffix(strings.TrimSpace(out.String()), " ok installed") {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	uid := os.Geteuid()
	if o.uid != nil {
		uid = o.uid()
	}
	privileged := uid != 0
	if privileged {
		if _, err := o.Process.lookPath("sudo"); err != nil {
			return o.optionalFailure(requirement, "sudo is required to install apt packages as a non-root user.", missing)
		}
	}
	apt := func(args ...string) error {
		if privileged {
			return o.run(ctx, "sudo", append([]string{"apt-get"}, args...)...)
		}
		return o.run(ctx, "apt-get", args...)
	}
	if !o.aptUpdated {
		if err := apt("update"); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return o.optionalFailure(requirement, "Could not update apt package indexes.", missing)
		}
		o.aptUpdated = true
	}
	var available, unavailable []string
	for _, name := range missing {
		p := o.Process
		p.Out = io.Discard
		p.Err = io.Discard
		code, err := p.Run(ctx, "apt-cache", "show", name)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil && code == 0 {
			available = append(available, name)
		} else {
			unavailable = append(unavailable, name)
		}
	}
	if len(unavailable) > 0 {
		if err := o.optionalFailure(requirement, fmt.Sprintf("Unavailable %s apt packages: %s", requirement, strings.Join(unavailable, " ")), unavailable); err != nil {
			return err
		}
	}
	if len(available) == 0 {
		return nil
	}
	if err := apt(append([]string{"install", "-y"}, available...)...); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return o.optionalFailure(requirement, fmt.Sprintf("Could not install %s apt packages: %s", requirement, strings.Join(available, " ")), available)
	}
	return nil
}
