// Package selfishell implements the development Go CLI candidate.
package selfishell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CLI keeps output and release location explicit for isolated invocation tests.
type CLI struct {
	Root     string
	In       io.Reader
	Out, Err io.Writer
}

func (c CLI) error(message string) {
	prefix := "selfishell:"
	if os.Getenv("NO_COLOR") == "" && IsTerminal(c.Err) {
		prefix = "\x1b[31mselfishell:\x1b[0m"
	}
	fmt.Fprintln(c.Err, prefix+" "+message)
}

// Run implements only the foundation commands. Unported operations must fail.
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
					return c.incomplete("version --available")
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
			// Bash command substitution drops NULs and trailing LF bytes, not spaces/CR.
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
	case "doctor", "status", "update", "rollback":
		return c.incomplete(command)
	default:
		c.error("Unknown command: " + command)
		c.error("Run 'selfishell help' to see available commands.")
		return 2
	}
}

func (c CLI) incomplete(command string) int {
	c.error(command + " is not implemented in the Go candidate.")
	return 1
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
  status     Show managed configuration status
  uninstall  Remove managed configuration
  update     Update the CLI, approved tools, and managed configuration
  rollback   Switch back to a retained CLI release
  doctor     Diagnose platform and required dependencies
  version    Print the Selfishell version
  help       Show this help

Exit codes:
  0  Command completed successfully
  1  Environment or operation error
  2  Invalid command usage

The optional 'sfs' command is a shorthand for 'selfishell'.
`
