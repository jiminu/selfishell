package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

func (p Process) environment() []string {
	if p.Env != nil {
		return append([]string(nil), p.Env...)
	}
	return os.Environ()
}

func envValue(env []string, key string) string {
	value := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			value = strings.TrimPrefix(entry, key+"=")
		}
	}
	return value
}

func withEnvironment(p Process, set map[string]string, unset ...string) Process {
	remove := map[string]bool{}
	for key := range set {
		remove[key] = true
	}
	for _, key := range unset {
		remove[key] = true
	}
	env := p.environment()
	p.Env = make([]string, 0, len(env)+len(set))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !remove[key] {
			p.Env = append(p.Env, entry)
		}
	}
	for key, value := range set {
		p.Env = append(p.Env, key+"="+value)
	}
	return p
}

func (o *PackageOperation) miseCommand(paths Paths) (string, error) {
	if path, err := o.Process.lookPath("mise"); err == nil {
		return path, nil
	}
	home := envValue(o.Process.environment(), "HOME")
	if home == "" {
		return "", fmt.Errorf("HOME must be set")
	}
	path := home + "/.local/bin/mise"
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
		return path, nil
	}
	return "", fmt.Errorf("mise is unavailable")
}

func (o *PackageOperation) miseProcess(root string, offline bool) Process {
	shared := root + "/config/shared"
	set := map[string]string{"MISE_GLOBAL_CONFIG_FILE": shared + "/mise.toml"}
	if offline {
		set["MISE_OFFLINE"] = "1"
	}
	p := withEnvironment(o.Process, set, "MISE_OFFLINE")
	p.Dir = shared
	return p
}

// InstallMise synchronizes one required or optional group against release pins.
func (o *PackageOperation) InstallMise(ctx context.Context, root string, paths Paths, requirement string, dryRun bool, names ...string) error {
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
		fmt.Fprintf(o.Process.Out, "Would sync %s mise tools: %s\n", requirement, strings.Join(names, " "))
		return nil
	}
	mise, err := o.miseCommand(paths)
	if err != nil {
		return o.optionalFailure(requirement, "mise is required to install mise-managed tools.", names)
	}
	link := strings.TrimSuffix(paths.Config, "/selfishell") + "/mise/conf.d/selfishell.toml"
	if info, err := os.Lstat(link); err == nil && (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		p := o.miseProcess(root, false)
		p.Out, p.Err = io.Discard, io.Discard
		_, _ = p.Run(ctx, mise, "trust", link)
		if err := ctx.Err(); err != nil {
			return err
		}
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
	return nil
}

func (o *PackageOperation) miseOutput(ctx context.Context, p Process, mise string, args ...string) (string, error) {
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
