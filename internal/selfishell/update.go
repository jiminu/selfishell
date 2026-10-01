package selfishell

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const updateHelp = `Usage:
  selfishell update [--cli-only | --tools-only] [--version VERSION]
                     [--skip-packages] [--dry-run] [--yes]

By default, update to the latest Selfishell release and synchronize that
release's packages, approved tools, and managed configuration. If the
target release is already installed, no changes are made. Use --tools-only to
explicitly resynchronize the current release's tools and configuration, or
--cli-only to limit the scope to the CLI release itself.
--version selects an exact CLI release and cannot be used with --tools-only.
When the tools/configuration phase runs, --skip-packages skips package and
tool installation and applies managed configuration only. A default update
whose target release is already installed exits before that phase; use
--tools-only --skip-packages to reapply the current release's configuration.

Already installed apt/Homebrew packages are left at their current version
(mise-managed and Selfishell direct tools are synced to their pinned
versions). Selfishell update does not perform a general Apt/Homebrew
upgrade.

After successful tool/configuration synchronization, unused versions of
Selfishell's mise tools are pruned automatically. Current versions and those
needed by mise's tracked project configurations are retained; versions needed
only by the rollback release are not.
--skip-packages also skips cleanup; --dry-run only describes the cleanup scope.
`

type updateOptions struct {
	mode, version                string
	yes, dry, skip, continuation bool
}

func (c CLI) parseUpdate(args []string) (updateOptions, int) {
	o := updateOptions{mode: "all"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cli-only":
			if o.continuation {
				c.error("--cli-only and --continue-after-cli-update cannot be used together")
				return o, 2
			}
			if o.mode == "tools" {
				c.error("--cli-only and --tools-only cannot be used together")
				return o, 2
			}
			o.mode = "cli"
		case "--tools-only":
			if o.mode == "cli" {
				c.error("--cli-only and --tools-only cannot be used together")
				return o, 2
			}
			o.mode = "tools"
		case "--version":
			i++
			if i == len(args) {
				c.error("--version requires a value")
				return o, 2
			}
			o.version = strings.TrimPrefix(args[i], "v")
			if !ValidReleaseVersion(o.version) {
				c.error("Invalid semantic version: " + args[i])
				return o, 2
			}
		case "--skip-packages":
			o.skip = true
		case "--dry-run":
			o.dry = true
		case "--yes":
			o.yes = true
		case "--continue-after-cli-update":
			if o.mode == "cli" {
				c.error("--cli-only and --continue-after-cli-update cannot be used together")
				return o, 2
			}
			o.continuation = true
			o.mode = "tools"
		case "help", "--help", "-h":
			fmt.Fprint(c.Out, updateHelp)
			return o, -1
		default:
			c.error("Unknown update option: " + args[i])
			return o, 2
		}
	}
	if o.mode == "tools" && o.version != "" {
		c.error("--version cannot be used with --tools-only")
		return o, 2
	}
	return o, 0
}

func (c CLI) confirmRelease(prompt string, yes, dry bool) int {
	if yes || dry {
		return 0
	}
	if !c.interactive() {
		c.error("Confirmation requires an interactive terminal; use --yes.")
		return 2
	}
	fmt.Fprint(c.Out, prompt+" [y/N] ")
	answer, _ := c.readAnswer()
	if !affirmative(answer) {
		fmt.Fprintln(c.Out, "Cancelled.")
		return 1
	}
	return 0
}

func (c CLI) update(args []string) int {
	o, code := c.parseUpdate(args)
	if code < 0 {
		return 0
	}
	if code != 0 {
		return code
	}
	ctx := c.invocationContext()
	if o.mode != "tools" {
		version := o.version
		discovered := version == ""
		if discovered {
			var err error
			version, err = c.latestReleaseVersion()
			if err != nil {
				c.error("Unable to determine the latest Selfishell release. Use --version VERSION to select one.")
				return 1
			}
		}
		active := ""
		if data, err := os.ReadFile(filepath.Join(c.Root, "VERSION")); err == nil {
			candidate := strings.TrimSuffix(string(data), "\n")
			if ValidReleaseVersion(candidate) {
				active = candidate
			}
		}
		if active == version || (discovered && compareReleaseVersions(active, version) > 0) {
			if discovered && active != version {
				fmt.Fprintf(c.Out, "Selfishell %s is newer than the latest release %s.\n", active, version)
			}
			if o.mode == "all" {
				fmt.Fprintf(c.Out, "Selfishell is up to date (%s).\n", active)
			} else {
				fmt.Fprintf(c.Out, "Selfishell CLI is already at %s; skipping CLI update.\n", active)
			}
			return 0
		}
		if o.dry {
			fmt.Fprintf(c.Out, "Would update Selfishell CLI to %s.\n", version)
			if o.mode == "all" {
				running := active
				if running == "" {
					running = "development"
				}
				fmt.Fprintf(c.Out, "Tools and configuration preview uses the running release (%s); target release contents are not downloaded for dry-run.\n", running)
			}
		} else {
			if _, err := installedReleaseLayout(c.Root); err != nil {
				c.error(err.Error())
				return 1
			}
			if code := c.confirmRelease("Update Selfishell CLI to "+version+"?", o.yes, false); code != 0 {
				return code
			}
			paths, err := UserPaths()
			if err != nil {
				c.error(err.Error())
				return 1
			}
			c.progress = newProgress(c.Out, c.Err, paths)
			defer c.progress.finish()
			c.progress.stage("Downloading and verifying Selfishell " + version)
			// Keep default signal handling while confirmation reads from the terminal.
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			target, err := (releaseOperation{Root: c.Root, Process: Process{In: c.In, Out: c.Out, Err: c.Err, progress: c.progress}}).install(ctx, version)
			if err != nil {
				c.error(err.Error())
				return 1
			}
			if o.mode == "all" {
				c.progress.pause()
				argv := []string{"update", "--continue-after-cli-update"}
				if o.yes {
					argv = append(argv, "--yes")
				}
				if o.skip {
					argv = append(argv, "--skip-packages")
				}
				code, err := (Process{In: c.In, Out: c.Out, Err: c.Err}).runCLI(ctx, target+"/bin/selfishell", argv...)
				if code != 0 || err != nil {
					c.report("Tools", reportSuccess, "Activated Selfishell CLI: %s -> %s", active, version)
				}
				if err != nil {
					c.error(err.Error())
					return code
				}
				return code
			}
			c.complete(fmt.Sprintf("Selfishell updated: %s -> %s", active, version))
			return 0
		}
	}
	if o.mode != "cli" {
		if code := c.updateTools(o); code != 0 {
			return code
		}
	}

	return 0
}

func (c CLI) completeUpdate() {
	layout, err := installedReleaseLayout(c.Root)
	if err != nil {
		return
	}
	previous, _ := os.Readlink(layout.previous)
	from, to := filepath.Base(previous), filepath.Base(c.Root)
	if previous == "" || from == to {
		c.complete("Selfishell updated to " + to + ".")
	} else {
		c.complete("Selfishell updated: " + from + " -> " + to)
	}
}

func (c CLI) updateTools(o updateOptions) (result int) {
	paths, err := UserPaths()
	if err != nil {
		c.error(err.Error())
		return 1
	}
	_, err = os.ReadFile(paths.State + "/configured")
	if os.IsNotExist(err) {
		if o.mode == "tools" && !o.continuation {
			c.error("Selfishell configuration is not installed.")
			return 1
		}
		fmt.Fprintln(c.Out, "Selfishell configuration is not installed; skipping tools and configuration.")
		if o.continuation {
			c.completeUpdate()
		}
		return 0
	}
	if err != nil {
		c.error("Could not read Selfishell configured marker: " + err.Error())
		return 1
	}
	defer func() {
		if result != 0 && !o.dry {
			c.retryHint("selfishell update --tools-only", o.skip)
		}
	}()
	packages, err := ReadPackages(c.Root + "/packages.conf")
	if err != nil {
		return c.diagnosticError(err)
	}
	if _, err := ReadDependencies(envDefault("SELFISHELL_DEPENDENCIES_FILE", c.Root+"/dependencies.conf")); err != nil {
		c.error(err.Error())
		return 1
	}
	platform := DetectPlatform().Name
	if !platformSupported(platform) {
		c.error("Managed installation is unavailable on " + platform + ".")
		return 1
	}
	if !o.skip {
		if err := checkMisePins(c.Root, packages, platform); err != nil {
			c.error(err.Error())
			return 1
		}
	}
	if err := c.invocationContext().Err(); err != nil {
		c.error(err.Error())
		return 1
	}
	prepared, err := c.prepareConfig(platform, o.dry, o.yes, true, false)
	if err != nil {
		c.error(err.Error())
		return 1
	}
	if err := c.invocationContext().Err(); err != nil {
		c.error(err.Error())
		return 1
	}
	if !o.dry {
		c.progress = newProgress(c.Out, c.Err, prepared.paths)
		prepared.m.c.progress = c.progress
		defer c.progress.finish()
	}
	operation := &PackageOperation{Process: Process{In: c.In, Out: c.Out, Err: c.Err, progress: c.progress}}
	if !o.skip {
		phaseCtx, stop := signal.NotifyContext(c.invocationContext(), os.Interrupt, syscall.SIGTERM)
		err = c.installPackages(phaseCtx, operation, prepared.paths, packages, platform, DetectPlatform().Arch, prepared.ghostty, o.dry)
		canceled := phaseCtx.Err()
		stop()
		if err != nil {
			c.error(err.Error())
			return 1
		}
		if canceled != nil {
			c.error(canceled.Error())
			return 1
		}
		operation.reportSkippedOptional()
	}
	if o.skip {
		c.report("Notes", reportWarning, "Skipping package and tool installation.")
	}
	if err := c.invocationContext().Err(); err != nil {
		c.error(err.Error())
		return 1
	}
	if err := c.applyManagedResources(&prepared); err != nil {
		c.error(err.Error())
		return 1
	}
	if err := c.invocationContext().Err(); err != nil {
		c.error(err.Error())
		return 1
	}
	if !o.skip {
		if err := c.installNeovim(operation, prepared.paths, o.dry); err != nil {
			c.error(err.Error())
			return 1
		}
		if err := c.invocationContext().Err(); err != nil {
			c.error(err.Error())
			return 1
		}
		toolsCtx, stop := signal.NotifyContext(c.invocationContext(), os.Interrupt, syscall.SIGTERM)
		c.progress.stage("Synchronizing Neovim language servers")
		if err := operation.UpdateDefaultLSP(toolsCtx, c.Root, prepared.paths, o.dry); err != nil {
			stop()
			c.error(err.Error())
			return 1
		}
		c.progress.stage("Cleaning up unused tool versions")
		pruneErr := operation.PruneMise(toolsCtx, c.Root, prepared.paths, packages, platform, o.dry)
		canceled := toolsCtx.Err()
		stop()
		if pruneErr != nil {
			if canceled != nil {
				c.error(canceled.Error())
				return 1
			}
			operation.warn("Could not prune unused mise versions; tools and configuration were synchronized.")
		}
		if canceled != nil {
			c.error(canceled.Error())
			return 1
		}
	}
	if err := c.invocationContext().Err(); err != nil {
		c.error(err.Error())
		return 1
	}
	if o.dry {
		fmt.Fprintln(c.Out, "Tool/configuration dry run complete.")
	} else if !o.continuation {
		c.complete("Selfishell tools and configuration synchronized.")
	} else {
		c.completeUpdate()
	}
	if !o.dry && prepared.m.unchanged > 0 && c.progress.compact {
		c.report("Notes", reportInfo, "%d configuration items unchanged.", prepared.m.unchanged)
	}
	return 0
}
