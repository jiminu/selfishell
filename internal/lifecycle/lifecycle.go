// Package lifecycle installs, updates and removes Selfishell's managed files,
// links and blocks. It owns their invariants: pending state before a change,
// atomic writes, backups that are never overwritten and restores that never
// replace an occupied target.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/report"
)

// Operation installs and removes managed resources and owns their pending
// state, backups and restores. Reporter receives its notes. Ask prompts about a
// modified resource and returns false when nobody can answer; a nil Ask cannot.
// Dry previews without writing, Yes keeps modified resources instead of asking,
// and UnchangedCount counts resources already in their managed state.
type Operation struct {
	Paths          managed.Paths
	Reporter       report.Reporter
	Ask            func(question string) (answer string, ok bool)
	Dry, Yes       bool
	UnchangedCount int
	Hooks          Hooks
	actions        map[string]string // overwrite or skip, asked once per resource
}

// Hooks exist for failure tests and are nil in production.
type Hooks struct {
	CreateLink       func(oldname, newname string) error
	AtomicWrite      func(path string, data []byte, mode os.FileMode) error
	BeforeBackupMove func(target, backup string)
	RemoveState      func(path string) error
}

// Write replaces path atomically.
func (m *Operation) Write(path string, data []byte, mode os.FileMode) error {
	if m.Hooks.AtomicWrite != nil {
		return m.Hooks.AtomicWrite(path, data, mode)
	}
	return managed.WriteAtomic(path, data, mode)
}

// Report forwards a note to Reporter.
func (m *Operation) Report(section string, tone report.Tone, format string, args ...any) {
	m.Reporter.Report(section, tone, format, args...)
}

// DryRun reports whether the lifecycle only previews.
func (m *Operation) DryRun() bool { return m.Dry }

// Unchanged counts one resource already in its managed state.
func (m *Operation) Unchanged() { m.UnchangedCount++ }

func blockConflictError(r managed.Resource) error {
	return fmt.Errorf("Cannot manage the Selfishell %s block in: %s\nselfishell: Preserving the file. Remove conflicting Selfishell markers and retry.", r.Name, r.Target)
}

func (m *Operation) statePath(r managed.Resource) string {
	return m.Paths.Resources + "/" + r.Name + ".state"
}
func (m *Operation) state(r managed.Resource) (managed.State, bool, error) {
	s, e := managed.ReadState(m.statePath(r))
	if errors.Is(e, fs.ErrNotExist) {
		return managed.State{}, false, nil
	}
	return s, e == nil, e
}
func (m *Operation) save(r managed.Resource, s managed.State) error {
	return managed.WriteState(m.statePath(r), s)
}
func (m *Operation) say(tone report.Tone, format string, args ...any) {
	m.Reporter.Report("Configuration", tone, format, args...)
}

// Backup returns the first unused timestamped backup name for path.
func (m *Operation) Backup(path string) (string, error) {
	base := path + ".backup." + time.Now().Format("20060102150405")
	candidate := base
	for n := 1; ; n++ {
		_, present, e := managed.Exists(candidate)
		if e != nil {
			return "", e
		}
		if !present {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s.%d", base, n)
	}
}

// InstallResource installs or updates r by kind; preflight only checks it.
func (m *Operation) InstallResource(r managed.Resource, preflight bool) error {
	switch r.Kind {
	case "file":
		return m.InstallFile(r, preflight)
	case "link":
		return m.InstallLink(r, preflight)
	case "block":
		return m.InstallBlock(r, preflight)
	}
	return fmt.Errorf("invalid resource kind: %s", r.Kind)
}

// InstallFile copies r.Source to r.Target, backing up a user file first.
func (m *Operation) InstallFile(r managed.Resource, preflight bool) error {
	source, err := os.ReadFile(r.Source)
	if err != nil {
		return err
	}
	sourceChecksum, err := managed.ChecksumBytes(source)
	if err != nil {
		return err
	}
	s, has, err := m.state(r)
	if err != nil {
		return fmt.Errorf("Managed resource state is malformed: %s", m.statePath(r))
	}
	if has && (s.Kind != "file" || s.Target != r.Target) {
		return fmt.Errorf("State conflict for managed file: %s", r.Name)
	}
	if has && s.Status == "restoring" {
		return interruptedRestore(s.Target)
	}
	info, present, err := managed.Exists(r.Target)
	if err != nil {
		return err
	}
	if has && present && !info.Mode().IsRegular() {
		return fmt.Errorf("Managed file path changed type; preserving it: %s", r.Target)
	}
	backup := "-"
	if has {
		backup = s.Backup
	} else if present {
		backup, err = m.Backup(r.Target)
		if err != nil {
			return err
		}
	}
	backupPresent := false
	if backup != "-" {
		_, backupPresent, err = managed.Exists(backup)
		if err != nil {
			return err
		}
	}
	preserveOriginal := backup != "-" && present && (!has || !backupPresent)
	current := ""
	if present && info.Mode().IsRegular() {
		current, err = managed.Checksum(context.Background(), r.Target)
		if err != nil {
			return err
		}
	}
	if has && current != "" && current != s.Checksum && current != sourceChecksum {
		if s.Status == "active" || backup == "-" || backupPresent {
			if m.Dry {
				if !preflight {
					m.say(report.Warning, "Conflict: modified managed file: %s", r.Target)
					m.say(report.Preview, "Would require an overwrite or skip decision.")
				}
				return nil
			}
			if overwrite, err := m.ResolveModified(r, "file", "Overwrite with default config?", preflight); err != nil || !overwrite {
				return err
			}
		}
	}
	if preflight {
		return nil
	}
	if current == sourceChecksum && !preserveOriginal {
		if !m.Dry {
			if err = m.save(r, managed.State{Kind: "file", Status: "active", Target: r.Target, Reference: "-", Backup: backup, Checksum: sourceChecksum}); err != nil {
				return err
			}
		}
		m.UnchangedCount++
		return nil
	}
	active := has && s.Status == "active" && present
	verb := "Installed"
	if active {
		verb = "Updated"
	}
	if m.Dry {
		m.say(report.Preview, "Would %s managed file: %s", map[bool]string{true: "update", false: "install"}[active], r.Target)
		return nil
	}
	// Keep the accepted pre-write bytes recognizable if the atomic write fails.
	// A completed write is recognized separately by sourceChecksum on retry.
	pendingChecksum := sourceChecksum
	if has && current != "" {
		pendingChecksum = current
	}
	if err = m.save(r, managed.State{Kind: "file", Status: "pending", Target: r.Target, Reference: "-", Backup: backup, Checksum: pendingChecksum}); err != nil {
		return err
	}
	if backup != "-" {
		if preserveOriginal {
			if err = managed.MakeRawDir(managed.RawParent(backup)); err != nil {
				return err
			}
			if m.Hooks.BeforeBackupMove != nil {
				m.Hooks.BeforeBackupMove(r.Target, backup)
			}
			if err = moveBackupNoReplace(r.Target, backup); err != nil {
				return err
			}
		}
	}
	if err = m.Write(r.Target, source, 0644); err != nil {
		return err
	}
	if err = m.save(r, managed.State{Kind: "file", Status: "active", Target: r.Target, Reference: "-", Backup: backup, Checksum: sourceChecksum}); err != nil {
		return err
	}
	m.say(report.Success, "%s managed file: %s", verb, r.Target)
	return nil
}

// ResolveModified asks once per resource whether to overwrite a locally
// modified managed file or block, backing it up first. False leaves it as is.
func (m *Operation) ResolveModified(r managed.Resource, kind, question string, preflight bool) (bool, error) {
	action := m.actions[r.Name]
	if action == "" {
		var answer string
		ok := !m.Yes && m.Ask != nil
		if ok {
			answer, ok = m.Ask(fmt.Sprintf("Managed %s was modified: %s. %s [y/N] ", kind, r.Target, question))
		}
		if !ok {
			return false, fmt.Errorf("Managed %s was modified; preserving it: %s", kind, r.Target)
		}
		if report.Affirmative(answer) {
			action = "overwrite"
		} else {
			action = "skip"
		}
		if m.actions == nil {
			m.actions = map[string]string{}
		}
		m.actions[r.Name] = action
	}
	if preflight {
		return false, nil
	}
	if action == "skip" {
		m.say(report.Warning, "Skipped modified managed %s: %s", kind, r.Target)
		return false, nil
	}
	conflict, err := m.Backup(m.Paths.State + "/backups/" + r.Name)
	if err != nil {
		return false, err
	}
	// copyPreserve creates the backups directory.
	if err = copyPreserve(r.Target, conflict); err != nil {
		return false, err
	}
	m.say(report.Success, "Backed up modified managed %s: %s -> %s", kind, r.Target, conflict)
	return true, nil
}
func copyPreserve(source, target string) error {
	data, e := os.ReadFile(source)
	if e != nil {
		return e
	}
	i, e := os.Stat(source)
	if e != nil {
		return e
	}
	return managed.CreateRawExclusive(target, data, i.Mode().Perm())
}

// InstallLink links r.Target to r.Source, backing up a user path first.
func (m *Operation) InstallLink(r managed.Resource, preflight bool) error {
	s, has, err := m.state(r)
	if err != nil {
		return fmt.Errorf("Managed resource state is malformed: %s", m.statePath(r))
	}
	if has && (s.Kind != "link" || s.Target != r.Target) {
		return fmt.Errorf("State conflict for managed link: %s", r.Name)
	}
	if has && s.Status == "restoring" {
		return interruptedRestore(s.Target)
	}
	info, present, err := managed.Exists(r.Target)
	if err != nil {
		return err
	}
	backup := "-"
	if has {
		backup = s.Backup
	} else if present {
		backup, err = m.Backup(r.Target)
		if err != nil {
			return err
		}
	}
	if has && present && info.Mode()&os.ModeSymlink != 0 {
		dest, _ := os.Readlink(r.Target)
		if dest == r.Source {
			if !preflight {
				if !m.Dry {
					info, present, err = managed.Exists(r.Target)
					if err != nil {
						return err
					}
					if !present || info.Mode()&os.ModeSymlink == 0 {
						return fmt.Errorf("Failed to install managed link: %s", r.Target)
					}
					dest, err := os.Readlink(r.Target)
					if err != nil || dest != r.Source {
						return fmt.Errorf("Failed to install managed link: %s", r.Target)
					}
					if err = m.save(r, managed.State{Kind: "link", Status: "active", Target: r.Target, Reference: r.Source, Backup: backup, Checksum: "-"}); err != nil {
						return err
					}
				}
				m.UnchangedCount++
			}
			return nil
		}
	}
	if has && s.Status == "active" && present {
		return fmt.Errorf("Managed link was replaced; preserving it: %s", r.Target)
	}
	if preflight {
		return nil
	}
	if m.Dry {
		m.say(report.Preview, "Would link: %s -> %s", r.Target, r.Source)
		return nil
	}
	if err = m.save(r, managed.State{Kind: "link", Status: "pending", Target: r.Target, Reference: r.Source, Backup: backup, Checksum: "-"}); err != nil {
		return err
	}
	if err = managed.MakeRawDir(managed.RawParent(r.Target)); err != nil {
		return err
	}
	moved := false
	if backup != "-" && present {
		_, b, _ := managed.Exists(backup)
		if !b {
			if m.Hooks.BeforeBackupMove != nil {
				m.Hooks.BeforeBackupMove(r.Target, backup)
			}
			if err = moveBackupNoReplace(r.Target, backup); err != nil {
				return err
			}
			moved = true
		}
	}
	_, present, _ = managed.Exists(r.Target)
	if !present {
		create := m.Hooks.CreateLink
		if create == nil {
			create = os.Symlink
		}
		if err = create(r.Source, r.Target); err != nil {
			if moved {
				if restoreErr := moveBackupNoReplace(backup, r.Target); restoreErr == nil {
					remove := m.Hooks.RemoveState
					if remove == nil {
						remove = os.Remove
					}
					if removeErr := remove(m.statePath(r)); removeErr != nil {
						return removeErr
					}
				}
			}
			return err
		}
	}
	info, present, err = managed.Exists(r.Target)
	if err != nil {
		return err
	}
	if !present || info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("Failed to install managed link: %s", r.Target)
	}
	dest, err := os.Readlink(r.Target)
	if err != nil || dest != r.Source {
		return fmt.Errorf("Failed to install managed link: %s", r.Target)
	}
	if err = m.save(r, managed.State{Kind: "link", Status: "active", Target: r.Target, Reference: r.Source, Backup: backup, Checksum: "-"}); err != nil {
		return err
	}
	m.say(report.Success, "Linked: %s -> %s", r.Target, r.Source)
	return nil
}

// InstallBlock adds or updates the Selfishell block in r.Target.
func (m *Operation) InstallBlock(r managed.Resource, preflight bool) error {
	content, err := managed.BlockContent(r.Name, m.Paths.Config)
	if err != nil {
		return err
	}
	expected, err := managed.ChecksumBytes(content)
	if err != nil {
		return err
	}
	info, present, err := managed.Exists(r.Target)
	if err != nil {
		return err
	}
	if present && !info.Mode().IsRegular() {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Refusing to modify symbolic link: %s", r.Target)
		}
		return fmt.Errorf("Refusing to modify non-regular block path: %s", r.Target)
	}
	var data []byte
	if present {
		data, err = os.ReadFile(r.Target)
		if err != nil {
			return err
		}
	}
	view, err := managed.InspectBlock(r.Name, data)
	if err != nil {
		return err
	}
	s, has, err := m.state(r)
	if err != nil {
		return fmt.Errorf("Managed resource state is malformed: %s", m.statePath(r))
	}
	if has && (s.Kind != "block" || s.Target != r.Target) {
		return fmt.Errorf("State conflict for managed block: %s", r.Name)
	}
	if !has && view.Status != "absent" {
		return blockConflictError(r)
	}
	if has && view.Status == "intact" && view.Checksum == expected {
		if !preflight {
			if !m.Dry {
				if err = m.save(r, managed.State{Kind: "block", Status: "active", Target: r.Target, Reference: "-", Backup: "-", Checksum: expected}); err != nil {
					return err
				}
			}
			m.UnchangedCount++
		}
		return nil
	}
	if has && view.Status == "intact" && view.Checksum != s.Checksum {
		if m.Dry {
			if !preflight {
				m.say(report.Preview, "Would preserve modified managed block: %s", r.Target)
			}
			return nil
		}
		if overwrite, err := m.ResolveModified(r, "block", "Overwrite the Selfishell block?", preflight); err != nil || !overwrite {
			return err
		}
	}
	if has && view.Status != "intact" && view.Status != "absent" {
		return blockConflictError(r)
	}
	if preflight {
		return nil
	}
	if view.Status == "intact" {
		if m.Dry {
			m.say(report.Preview, "Would update Selfishell block: %s", r.Target)
			return nil
		}
		if err = m.save(r, managed.State{Kind: "block", Status: "pending", Target: r.Target, Reference: "-", Backup: "-", Checksum: view.Checksum}); err != nil {
			return err
		}
		data = managed.SpliceBlock(data, view, content)
	} else {
		if m.Dry {
			m.say(report.Preview, "Would add Selfishell block: %s", r.Target)
			return nil
		}
		if err = m.save(r, managed.State{Kind: "block", Status: "pending", Target: r.Target, Reference: "-", Backup: "-", Checksum: expected}); err != nil {
			return err
		}
		data = append(content, data...)
	}
	mode := os.FileMode(0644)
	if present {
		mode = info.Mode().Perm()
	}
	if err = m.Write(r.Target, data, mode); err != nil {
		return err
	}
	if err = m.save(r, managed.State{Kind: "block", Status: "active", Target: r.Target, Reference: "-", Backup: "-", Checksum: expected}); err != nil {
		return err
	}
	if view.Status == "intact" {
		m.say(report.Success, "Updated Selfishell block: %s", r.Target)
	} else {
		m.say(report.Success, "Added Selfishell block: %s", r.Target)
	}
	return nil
}
