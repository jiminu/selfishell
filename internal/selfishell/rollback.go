package selfishell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// rollback activates a retained release, the previous one when requested is
// empty. confirm runs after every check and before any link changes; its error
// is returned unchanged.
func (o releaseOperation) rollback(requested string, confirm func(version string) error) error {
	l, err := installedReleaseLayout(o.Root)
	if err != nil {
		return err
	}
	current, err := os.Readlink(l.current)
	if err != nil {
		return err
	}
	if requested == "" {
		link, err := os.Readlink(l.previous)
		if os.IsNotExist(err) {
			return errors.New("No previous release is retained.")
		}
		if err != nil {
			// A non-link at previous is user data; name it instead of echoing EINVAL.
			if info, e := os.Lstat(l.previous); e == nil && info.Mode()&os.ModeSymlink == 0 {
				return errors.New("Retained previous release is invalid: " + l.previous + " is not a link")
			}
			return err
		}
		if requested, err = retainedRelease(l.releases, link); err != nil {
			return errors.New("Retained previous release is invalid: " + filepath.Base(link))
		}
	} else if _, err := validReleaseDirectory(l.releases, requested); err != nil {
		return errors.New("Retained release not found: " + requested)
	}
	target := "releases/" + requested
	currentInfo, currentErr := os.Stat(l.current)
	targetInfo, targetErr := os.Stat(l.releases + "/" + requested)
	if current == target || currentErr == nil && targetErr == nil && os.SameFile(currentInfo, targetInfo) {
		fmt.Fprintf(o.Process.Out, "Release is already active: %s\n", requested)
		return nil
	}
	if err := releaseLinkReplaceable(l.current); err != nil {
		return err
	}
	if err := releaseLinkReplaceable(l.previous); err != nil {
		return err
	}
	previous, previousErr := os.Readlink(l.previous)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return previousErr
	}
	if err := confirm(requested); err != nil {
		return err
	}
	if err := atomicReleaseLink(current, l.previous); err != nil {
		return fmt.Errorf("Failed to retain current Selfishell release: %w", err)
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
			fmt.Fprintln(o.Process.Err, "selfishell: warning: Failed to restore the previous release link: "+restoreErr.Error())
		}
		return errors.New("Failed to roll back to " + requested + ".")
	}
	fmt.Fprintf(o.Process.Out, "Selfishell CLI rolled back to %s.\n", requested)
	return nil
}
