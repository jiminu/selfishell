package selfishell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const rollbackHelp = "Usage: selfishell rollback [VERSION] [--yes]\n"

func (c CLI) rollback(args []string) int {
	requested, yes := "", false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			yes = true
		case arg == "help" || arg == "--help" || arg == "-h":
			fmt.Fprint(c.Out, rollbackHelp)
			return 0
		case strings.HasPrefix(arg, "-"):
			c.error("Unknown rollback option: " + arg)
			return 2
		case requested != "":
			c.error("rollback accepts only one version")
			return 2
		default:
			requested = strings.TrimPrefix(arg, "v")
			if !ValidReleaseVersion(requested) {
				c.error("Invalid semantic version: " + arg)
				return 2
			}
		}
	}
	l, err := installedReleaseLayout(c.Root)
	if err != nil {
		c.error(err.Error())
		return 1
	}
	current, err := os.Readlink(l.current)
	if err != nil {
		c.error(err.Error())
		return 1
	}
	if requested == "" {
		link, err := os.Readlink(l.previous)
		if os.IsNotExist(err) {
			c.error("No previous release is retained.")
			return 1
		}
		if err != nil {
			// A non-link at previous is user data; name it instead of echoing EINVAL.
			if info, e := os.Lstat(l.previous); e == nil && info.Mode()&os.ModeSymlink == 0 {
				c.error("Retained previous release is invalid: " + l.previous + " is not a link")
				return 1
			}
			c.error(err.Error())
			return 1
		}
		if requested, err = retainedRelease(l.releases, link); err != nil {
			c.error("Retained previous release is invalid: " + filepath.Base(link))
			return 1
		}
	} else if _, err := validReleaseDirectory(l.releases, requested); err != nil {
		c.error("Retained release not found: " + requested)
		return 1
	}
	target := "releases/" + requested
	currentInfo, currentErr := os.Stat(l.current)
	targetInfo, targetErr := os.Stat(l.releases + "/" + requested)
	if current == target || currentErr == nil && targetErr == nil && os.SameFile(currentInfo, targetInfo) {
		fmt.Fprintf(c.Out, "Release is already active: %s\n", requested)
		return 0
	}
	if err := releaseLinkReplaceable(l.current); err != nil {
		c.error(err.Error())
		return 1
	}
	if err := releaseLinkReplaceable(l.previous); err != nil {
		c.error(err.Error())
		return 1
	}
	previous, previousErr := os.Readlink(l.previous)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		c.error(previousErr.Error())
		return 1
	}
	if code := c.confirmRelease("Roll back Selfishell CLI to "+requested+"?", yes, false); code != 0 {
		return code
	}
	if err := atomicReleaseLink(current, l.previous); err != nil {
		c.error("Failed to retain current Selfishell release: " + err.Error())
		return 1
	}
	if err := atomicReleaseLink(target, l.current); err != nil {
		var restoreErr error
		if link, readErr := os.Readlink(l.previous); readErr != nil || link != current {
			restoreErr = fmt.Errorf("previous release link changed during rollback")
		} else if previousErr == nil {
			restoreErr = atomicReleaseLink(previous, l.previous)
		} else {
			restoreErr = os.Remove(l.previous)
		}
		if restoreErr != nil {
			fmt.Fprintln(c.Err, "selfishell: warning: Failed to restore the previous release link: "+restoreErr.Error())
		}
		c.error("Failed to roll back to " + requested + ".")
		return 1
	}
	fmt.Fprintf(c.Out, "Selfishell CLI rolled back to %s.\n", requested)
	return 0
}
