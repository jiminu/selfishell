package selfishell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jiminu/selfishell/internal/lifecycle"
	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
	"github.com/jiminu/selfishell/internal/terminal"
)

const installHelp = `Usage:
  selfishell install [--skip-packages] [--ghostty|--windows-terminal] [--dry-run] [--yes]

Options:
  --skip-packages Skip package and tool installation and apply managed configuration only
  --ghostty  On macOS, install and manage Ghostty even if it was declined before
  --windows-terminal On WSL, configure the existing Windows Terminal profile even if previously declined
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
`
const uninstallHelp = `Usage:
  selfishell uninstall [--restore] [--purge] [--dry-run] [--yes]

Options:
  --restore  Restore configuration files backed up during installation
  --purge    Also remove the Selfishell CLI, releases, cache, and configuration state
  --dry-run  Show changes without modifying files
  --yes      Skip interactive confirmation
  --help     Show this help
`

func (c CLI) install(args []string) (result int) {
	skip, dry, yes, ghostty, windowsTerminal := false, false, false, false, false
	for _, arg := range args {
		switch arg {
		case "--skip-packages":
			skip = true
		case "--ghostty":
			ghostty = true
		case "--windows-terminal":
			windowsTerminal = true
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "help", "--help", "-h":
			fmt.Fprint(c.Out, installHelp)
			return 0
		default:
			c.error("Unknown install option: " + arg)
			return 2
		}
	}
	packages, err := ReadPackages(filepath.Join(c.Root, "packages.conf"))
	if err != nil {
		return c.diagnosticError(err)
	}
	if _, err := ReadDependencies(c.Env.Default("SELFISHELL_DEPENDENCIES_FILE", filepath.Join(c.Root, "dependencies.conf"))); err != nil {
		c.error(err.Error())
		return 1
	}
	platform := detectPlatform(c.Env).Name
	if !platformSupported(platform) {
		c.error("Managed installation is unavailable on " + platform + ".")
		return 1
	}
	if ghostty && platform != "macos" {
		c.error("--ghostty is available only on macOS.")
		return 2
	}
	if windowsTerminal && platform != "ubuntu-wsl" {
		c.error("--windows-terminal is available only on Ubuntu on WSL.")
		return 2
	}
	if !skip {
		if err := checkMisePins(c.Root, packages, platform); err != nil {
			c.error(err.Error())
			return 1
		}
	}
	if !yes && !dry {
		if !c.interactive() {
			c.error("Confirmation requires an interactive terminal; use --yes.")
			return 2
		}
		fmt.Fprint(c.Out, "Install Selfishell configuration? [y/N] ")
		answer, _ := c.readAnswer()
		if !report.Affirmative(answer) {
			fmt.Fprintln(c.Out, "Cancelled.")
			return 1
		}
	}
	defer func() {
		if result != 0 && !dry {
			command := "selfishell install"
			if ghostty {
				command += " --ghostty"
			}
			if windowsTerminal {
				command += " --windows-terminal"
			}
			c.retryHint(command, skip)
		}
	}()
	prepared, err := c.prepareConfig(platform, dry, yes, false, ghostty, windowsTerminal)
	if err != nil {
		c.error(err.Error())
		return 1
	}
	if err := c.saveWindowsTerminalChoice(prepared); err != nil {
		c.error(err.Error())
		return 1
	}
	if !dry {
		c.progress = newProgress(c.Out, c.Err, prepared.paths, c.Env)
		prepared.m.Reporter, prepared.m.Ask = report.ReportFunc(c.report), c.askModified
		defer c.progress.finish()
	}
	var operation *PackageOperation
	if !skip {
		operation = &PackageOperation{windowsTerminal: prepared.windowsTerminal, Process: process.Process{In: c.In, Out: c.Out, Err: c.Err, Inherit: c.Env, Progress: c.progress}}
		err = func() error {
			ctx, stop := signal.NotifyContext(c.invocationContext(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := c.installPackages(ctx, operation, prepared.paths, packages, platform, detectPlatform(c.Env).Arch, prepared.ghostty, dry); err != nil {
				return err
			}
			operation.reportSkippedOptional()
			return nil
		}()
		if err != nil {
			c.error(err.Error())
			return 1
		}
	}
	if err := c.applyConfig(prepared, dry, yes, skip, operation); err != nil {
		c.error(err.Error())
		return 1
	}
	return 0
}

func (c CLI) retryHint(command string, skipPackages bool) {
	if skipPackages {
		command += " --skip-packages"
	}
	fmt.Fprintf(c.Err, "After resolving the error, retry with: %s\n", command)
}

func (c CLI) uninstall(args []string) int {
	restore, purge, dry, yes := false, false, false, false
	for _, arg := range args {
		switch arg {
		case "--restore":
			restore = true
		case "--purge":
			purge = true
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "help", "--help", "-h":
			fmt.Fprint(c.Out, uninstallHelp)
			return 0
		default:
			c.error("Unknown uninstall option: " + arg)
			return 2
		}
	}
	if !yes && !dry {
		if !c.interactive() {
			c.error("Confirmation requires an interactive terminal; use --yes.")
			return 2
		}
		prompt := "Uninstall Selfishell configuration? [y/N] "
		if purge {
			prompt = "Uninstall and purge Selfishell? [y/N] "
		}
		fmt.Fprint(c.Out, prompt)
		answer, _ := c.readAnswer()
		if !report.Affirmative(answer) {
			fmt.Fprintln(c.Out, "Cancelled.")
			return 1
		}
	}
	if err := c.uninstallConfig(restore, purge, dry); err != nil {
		c.error(err.Error())
		return 1
	}
	return 0
}

type preparedConfig struct {
	paths           managed.Paths
	resources       []managed.Resource
	m               lifecycle.Operation
	miseGlobal      string
	ghostty         bool
	windowsTerminal *terminal.WindowsChoice
}

// The Windows Terminal fragment code drives the managed-file lifecycle.
var _ terminal.Lifecycle = (*lifecycle.Operation)(nil)

// managedLifecycle reports through the CLI and asks only when it is interactive.
func (c CLI) managedLifecycle(paths managed.Paths, dry, yes bool) lifecycle.Operation {
	return lifecycle.Operation{Paths: paths, Reporter: report.ReportFunc(c.report), Ask: c.askModified, Dry: dry, Yes: yes}
}

// askModified pauses the spinner before asking about a modified resource.
func (c CLI) askModified(question string) (string, bool) {
	if !c.interactive() {
		return "", false
	}
	c.progress.Pause()
	fmt.Fprint(c.Out, question)
	answer, _ := c.readAnswer()
	return answer, true
}

// windowsSetup reports through the CLI and asks only when it is interactive.
func (c CLI) windowsSetup() terminal.Setup {
	return terminal.Setup{
		Ctx:     c.invocationContext(),
		Process: process.Process{Out: c.Out, Err: c.Err, Inherit: c.Env},
		Distro:  c.Env.Get("WSL_DISTRO_NAME"),
		Report:  report.ReportFunc(c.report),
		Ask: func(question string) (string, bool) {
			if !c.interactive() {
				return "", false
			}
			fmt.Fprint(c.Out, question)
			answer, _ := c.readAnswer()
			return answer, true
		},
	}
}

func (c CLI) addWindowsTerminal(p *preparedConfig, dry, yes, update, enable bool) error {
	choice, err := c.windowsSetup().Prepare(p.paths, dry, yes, update, enable)
	if err != nil {
		return err
	}
	p.windowsTerminal = choice
	if choice == nil || !choice.Enabled {
		return nil
	}
	return terminal.InstallFragment(&p.m, p.paths, choice, true)
}

func (c CLI) saveWindowsTerminalChoice(p preparedConfig) error {
	return terminal.SaveChoice(p.paths, p.windowsTerminal, p.m.Dry)
}

// prepareConfig is read-only, including all user-owned and managed resource preflights.
func (c CLI) prepareConfig(platform string, dry, yes, update, enableGhostty, enableWindowsTerminal bool) (preparedConfig, error) {
	paths, err := managed.UserPaths(c.Env.Get)
	if err != nil {
		return preparedConfig{}, err
	}
	ghostty := false
	if platform == "macos" {
		data, e := managed.ReadFile(paths.State + "/ghostty")
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return preparedConfig{}, fmt.Errorf("could not read Ghostty choice %s: %w", paths.State+"/ghostty", e)
		}
		if enableGhostty {
			ghostty = true
		} else if e == nil {
			ghostty = string(data) == "1\n"
		} else if !update {
			ghostty = yes || dry
			if !ghostty && c.interactive() {
				question := "Install Ghostty terminal and managed configuration (recommended)? [Y/n] "
				if terminal.GhosttyInstalled(process.Process{Inherit: c.Env}) {
					question = "Use Selfishell configuration for your existing Ghostty terminal (recommended)? [Y/n] "
				}
				fmt.Fprint(c.Out, question)
				answer, _ := c.readAnswer()
				ghostty = !report.Negative(answer)
			}
		}
	}
	if !platformSupported(platform) {
		return preparedConfig{}, fmt.Errorf("managed installation is unavailable on %s", platform)
	}
	resources, err := lifecycle.ResourcesForPlatform(c.Root, platform, ghostty, c.Env)
	if err != nil {
		return preparedConfig{}, err
	}
	m := c.managedLifecycle(paths, dry, yes)
	for _, name := range []string{"user-zshrc", "user-zprofile", "user-zshenv", "user-vimrc", "user-ghostty"} {
		for _, r := range resources {
			if r.Name == name {
				if err = m.InstallResource(r, true); err != nil {
					return preparedConfig{}, err
				}
			}
		}
	}
	miseGlobal := c.Env.Default("XDG_CONFIG_HOME", c.Env.Get("HOME")+"/.config") + "/mise/config.toml"
	if !update {
		if info, present, e := managed.Exists(miseGlobal); e != nil {
			return preparedConfig{}, e
		} else if present && !(info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return preparedConfig{}, fmt.Errorf("user mise config path is not a regular file or symlink: %s", miseGlobal)
		}
	}
	for _, r := range resources {
		if r.Kind != "block" {
			if err = m.InstallResource(r, true); err != nil {
				return preparedConfig{}, err
			}
		}
	}
	prepared := preparedConfig{paths: paths, resources: resources, m: m, miseGlobal: miseGlobal, ghostty: ghostty}
	if platform == "ubuntu-wsl" {
		if err := c.addWindowsTerminal(&prepared, dry, yes, update, enableWindowsTerminal); err != nil {
			return preparedConfig{}, err
		}
	}
	return prepared, nil
}

// applyConfig also serves the later tools-only update without install-only finalization.
func (c CLI) applyManagedResources(p *preparedConfig) error {
	parent := c.invocationContext()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	c.Context = ctx
	defer func() { stop() }()
	defer c.progress.Pause()
	if c.progress != nil {
		// Conflict prompts keep normal terminal interrupt handling. Resume the
		// configuration context after input, including a changed target after preflight.
		c.progress.suspendSignals = func() func() {
			stop()
			return func() {
				ctx, stop = signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
				c.Context = ctx
				c.progress.stage("Applying configuration")
			}
		}
		defer func() { c.progress.suspendSignals = nil }()
	}
	c.progress.stage("Applying configuration")
	m, paths, resources := &p.m, p.paths, p.resources
	for _, r := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.Name == "zsh-interactive" && !m.Dry {
			same := false
			source, e := os.ReadFile(r.Source)
			if e == nil {
				target, e := os.ReadFile(r.Target)
				same = e == nil && string(target) == string(source)
			}
			if !same {
				for _, name := range []string{"zoxide-init.zsh", "fzf-init.zsh", "starship-init.zsh"} {
					os.Remove(paths.Cache + "/" + name)
				}
			}
		}
		if err := m.InstallResource(r, false); err != nil {
			return err
		}
	}
	if err := terminal.InstallFragment(m, m.Paths, p.windowsTerminal, false); err != nil {
		return err
	}
	if !m.Dry {
		c.trustMise(paths)
	}
	return ctx.Err()
}

func (c CLI) applyConfig(p preparedConfig, dry, yes, skip bool, operation *PackageOperation) error {
	if skip {
		c.report("Notes", report.Warning, "Skipping package and tool installation.")
	}
	if err := c.applyManagedResources(&p); err != nil {
		return err
	}
	m, paths, miseGlobal, ghostty := &p.m, p.paths, p.miseGlobal, p.ghostty
	if ghostty {
		if entrypoint, found := terminal.GhosttyOverrides(p.resources); len(found) > 0 {
			c.report("Notes", report.Warning, "Selfishell's Ghostty defaults override these settings in %s: %s. Move them to user.ghostty to keep them.", entrypoint, strings.Join(found, ", "))
		}
	}
	if operation != nil {
		m.UnchangedCount += operation.UnchangedCount
	}
	if dry {
		if !skip {
			if err := c.installNeovim(operation, paths, true); err != nil {
				return err
			}
		}
		if _, present, _ := managed.Exists(miseGlobal); present {
			fmt.Fprintf(c.Out, "user mise config exists; preserving it: %s\n", miseGlobal)
		} else {
			fmt.Fprintf(c.Out, "Would create user mise config: %s\n", miseGlobal)
		}
		c.defaultShell(dry, yes)
		if m.UnchangedCount > 0 {
			fmt.Fprintf(c.Out, "%d items unchanged.\n", m.UnchangedCount)
		}
		fmt.Fprintln(c.Out, "Dry run complete; no files were changed.")
		return nil
	}
	if _, present, _ := managed.Exists(miseGlobal); !present {
		if err := managed.WriteOnce(miseGlobal, nil); err != nil {
			return err
		}
		c.report("Configuration", report.Success, "Created user mise config: %s", miseGlobal)
	}
	if !skip {
		if err := c.installNeovim(operation, paths, dry); err != nil {
			return err
		}
	}
	if err := managed.WriteAtomic(paths.State+"/configured", []byte("1\n"), 0600); err != nil {
		return err
	}
	value := "0\n"
	if ghostty {
		value = "1\n"
	}
	if err := managed.WriteAtomic(paths.State+"/ghostty", []byte(value), 0600); err != nil {
		return err
	}
	c.defaultShell(dry, yes)
	if m.UnchangedCount > 0 {
		c.report("Notes", report.Info, "%d items unchanged.", m.UnchangedCount)
	}
	c.complete("Selfishell configuration installed.")
	return nil
}

func (c CLI) installNeovim(operation *PackageOperation, paths managed.Paths, dry bool) error {
	c.progress.stage("Synchronizing Neovim plugins")
	ctx, stop := signal.NotifyContext(c.invocationContext(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return operation.InstallNeovimPlugins(ctx, c.Root, paths, c.Env.Default("SELFISHELL_DEPENDENCIES_FILE", c.Root+"/dependencies.conf"), dry)
}

func (c CLI) interactive() bool {
	return terminal.IsTerminal(c.In) || c.Env.Get("SELFISHELL_TEST_TTY") != ""
}
func (c CLI) readAnswer() (string, error) {
	resume := c.progress.prompt()
	defer resume()
	if c.In == nil {
		return "", io.EOF
	}
	var result []byte
	var b [1]byte
	for {
		n, e := c.In.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				return strings.TrimSuffix(string(result), "\r"), nil
			}
			result = append(result, b[0])
		}
		if e != nil {
			return string(result), e
		}
	}
}
