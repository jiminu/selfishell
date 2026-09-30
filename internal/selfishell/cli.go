// Package selfishell implements the Selfishell CLI.
package selfishell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// CLI keeps output and release location explicit for isolated invocation tests.
type CLI struct {
	Root     string
	In       io.Reader
	Out, Err io.Writer
	Context  context.Context
	progress *operationProgress
}

func (c CLI) invocationContext() context.Context {
	if c.Context != nil {
		return c.Context
	}
	return context.Background()
}

func (c CLI) latestReleaseVersion() (string, error) {
	ctx, stop := signal.NotifyContext(c.invocationContext(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return (releaseOperation{Root: c.Root, Process: Process{In: c.In, Out: c.Out, Err: c.Err}}).latest(ctx)
}

func (c CLI) error(message string) {
	c.progress.pause()
	prefix := "selfishell:"
	if progressColor(c.Err) {
		prefix = "\x1b[31mselfishell:\x1b[0m"
	}
	fmt.Fprintln(c.Err, prefix+" "+message)
}

// Run dispatches the CLI commands.
func (c CLI) Run(args []string) int {
	command := "help"
	if len(args) > 0 {
		if args[0] != "" {
			command = args[0]
		}
		args = args[1:]
	}
	switch command {
	case "help", "--help", "-h":
		if len(args) > 0 {
			c.error("help does not accept arguments")
			return 2
		}
		fmt.Fprint(c.Out, helpText)
		return 0
	case "version", "--version", "-v":
		if len(args) > 0 {
			switch args[0] {
			case "":
			case "help", "--help", "-h":
				fmt.Fprintln(c.Out, "Usage: selfishell version [--available]")
				return 0
			case "--available":
				if len(args) == 1 {
					version, err := c.latestReleaseVersion()
					if err != nil {
						c.error("Unable to determine the latest Selfishell release.")
						return 1
					}
					fmt.Fprintln(c.Out, version)
					return 0
				}
				fallthrough
			default:
				c.error("Usage: selfishell version [--available]")
				return 2
			}
		}
		file := filepath.Join(c.Root, "VERSION")
		data, err := os.ReadFile(file)
		version := ""
		if err == nil {
			// Version text ignores NULs and trailing LF bytes, preserving spaces/CR.
			version = strings.TrimRight(strings.ReplaceAll(string(data), "\x00", ""), "\n")
		} else if _, err := os.Stat(filepath.Join(c.Root, ".git")); err == nil {
			version = "development"
		} else {
			c.error("Version file not found: " + file)
			return 1
		}
		fmt.Fprintln(c.Out, "selfishell "+version)
		return 0
	case "install":
		return c.install(args)
	case "uninstall":
		return c.uninstall(args)
	case "status", "doctor": // doctor is a hidden alias from before the two merged.
		return c.status(args)
	case "update":
		return c.update(args)
	case "rollback":
		return c.rollback(args)
	default:
		c.error("Unknown command: " + command)
		c.error("Run 'selfishell help' to see available commands.")
		return 2
	}
}

// ReleaseRoot resolves both bin/selfishell and the development .build/selfishell.
// Caller working directory and SELFISHELL_ROOT never override the executable.
func ReleaseRoot(executable string) (string, error) {
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(resolved)), nil
}

const helpText = `Selfishell manages a consistent Zsh development environment.

Usage:
  selfishell <command>

Commands:
  install    Install managed shell configuration
  status     Check the system, tools, and managed configuration
  uninstall  Remove managed configuration
  update     Update the CLI, approved tools, and managed configuration
  rollback   Switch back to a retained CLI release
  version    Print the Selfishell version
  help       Show this help

Exit codes:
  0  Command completed successfully
  1  Environment or operation error
  2  Invalid command usage

The optional 'sfs' command is a shorthand for 'selfishell'.
`
