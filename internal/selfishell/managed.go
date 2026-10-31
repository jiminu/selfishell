package selfishell

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

type lifecycle struct {
	c                CLI
	paths            managed.Paths
	dry, yes         bool
	unchanged        int
	actions          map[string]string
	removeState      func(string) error
	beforeBackupMove func(string, string)
	createLink       func(string, string) error
	atomicWrite      func(string, []byte, os.FileMode) error
}

func (m *lifecycle) write(path string, data []byte, mode os.FileMode) error {
	if m.atomicWrite != nil {
		return m.atomicWrite(path, data, mode)
	}
	return managed.WriteAtomic(path, data, mode)
}

// Report, DryRun, Unchanged, Backup, ResolveModified and Write let the
// Windows Terminal fragment code drive this lifecycle (windowsLifecycle).
func (m *lifecycle) Report(section string, tone report.Tone, format string, args ...any) {
	m.c.report(section, tone, format, args...)
}
func (m *lifecycle) DryRun() bool                       { return m.dry }
func (m *lifecycle) Unchanged()                         { m.unchanged++ }
func (m *lifecycle) Backup(base string) (string, error) { return m.backup(base) }
func (m *lifecycle) ResolveModified(r managed.Resource, kind, question string, preflight bool) (bool, error) {
	return m.resolveModified(r, kind, question, preflight)
}
func (m *lifecycle) Write(path string, data []byte, mode os.FileMode) error {
	return m.write(path, data, mode)
}

func blockConflictError(r managed.Resource) error {
	return fmt.Errorf("Cannot manage the Selfishell %s block in: %s\nselfishell: Preserving the file. Remove conflicting Selfishell markers and retry.", r.Name, r.Target)
}

func (m *lifecycle) statePath(r managed.Resource) string {
	return m.paths.Resources + "/" + r.Name + ".state"
}
func (m *lifecycle) state(r managed.Resource) (managed.State, bool, error) {
	s, e := managed.ReadState(m.statePath(r))
	if errors.Is(e, fs.ErrNotExist) {
		return managed.State{}, false, nil
	}
	return s, e == nil, e
}
func (m *lifecycle) save(r managed.Resource, s managed.State) error {
	return managed.WriteState(m.statePath(r), s)
}
func (m *lifecycle) say(tone report.Tone, format string, args ...any) {
	m.c.report("Configuration", tone, format, args...)
}
func (m *lifecycle) backup(path string) (string, error) {
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
func (m *lifecycle) installResource(r managed.Resource, preflight bool) error {
	switch r.Kind {
	case "file":
		return m.installFile(r, preflight)
	case "link":
		return m.installLink(r, preflight)
	case "block":
		return m.installBlock(r, preflight)
	}
	return fmt.Errorf("invalid resource kind: %s", r.Kind)
}
func (m *lifecycle) installFile(r managed.Resource, preflight bool) error {
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
		backup, err = m.backup(r.Target)
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
			if m.dry {
				if !preflight {
					m.say(report.Warning, "Conflict: modified managed file: %s", r.Target)
					m.say(report.Preview, "Would require an overwrite or skip decision.")
				}
				return nil
			}
			if overwrite, err := m.resolveModified(r, "file", "Overwrite with default config?", preflight); err != nil || !overwrite {
				return err
			}
		}
	}
	if preflight {
		return nil
	}
	if current == sourceChecksum && !preserveOriginal {
		if !m.dry {
			if err = m.save(r, managed.State{Kind: "file", Status: "active", Target: r.Target, Reference: "-", Backup: backup, Checksum: sourceChecksum}); err != nil {
				return err
			}
		}
		m.unchanged++
		return nil
	}
	active := has && s.Status == "active" && present
	verb := "Installed"
	if active {
		verb = "Updated"
	}
	if m.dry {
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
			if m.beforeBackupMove != nil {
				m.beforeBackupMove(r.Target, backup)
			}
			if err = moveBackupNoReplace(r.Target, backup); err != nil {
				return err
			}
		}
	}
	if err = m.write(r.Target, source, 0644); err != nil {
		return err
	}
	if err = m.save(r, managed.State{Kind: "file", Status: "active", Target: r.Target, Reference: "-", Backup: backup, Checksum: sourceChecksum}); err != nil {
		return err
	}
	m.say(report.Success, "%s managed file: %s", verb, r.Target)
	return nil
}

// resolveModified asks once per resource whether to overwrite a locally
// modified managed file or block, backing it up first. False leaves it as is.
func (m *lifecycle) resolveModified(r managed.Resource, kind, question string, preflight bool) (bool, error) {
	action := m.actions[r.Name]
	if action == "" {
		if m.yes || !m.c.interactive() {
			return false, fmt.Errorf("Managed %s was modified; preserving it: %s", kind, r.Target)
		}
		m.c.progress.Pause()
		fmt.Fprintf(m.c.Out, "Managed %s was modified: %s. %s [y/N] ", kind, r.Target, question)
		answer, _ := m.c.readAnswer()
		if report.Affirmative(answer) {
			action = "overwrite"
		} else {
			action = "skip"
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
	conflict, err := m.backup(m.paths.State + "/backups/" + r.Name)
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
func (m *lifecycle) installLink(r managed.Resource, preflight bool) error {
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
		backup, err = m.backup(r.Target)
		if err != nil {
			return err
		}
	}
	if has && present && info.Mode()&os.ModeSymlink != 0 {
		dest, _ := os.Readlink(r.Target)
		if dest == r.Source {
			if !preflight {
				if !m.dry {
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
				m.unchanged++
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
	if m.dry {
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
			if m.beforeBackupMove != nil {
				m.beforeBackupMove(r.Target, backup)
			}
			if err = moveBackupNoReplace(r.Target, backup); err != nil {
				return err
			}
			moved = true
		}
	}
	_, present, _ = managed.Exists(r.Target)
	if !present {
		create := m.createLink
		if create == nil {
			create = os.Symlink
		}
		if err = create(r.Source, r.Target); err != nil {
			if moved {
				if restoreErr := moveBackupNoReplace(backup, r.Target); restoreErr == nil {
					remove := m.removeState
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
func (m *lifecycle) installBlock(r managed.Resource, preflight bool) error {
	content, err := managed.BlockContent(r.Name, m.paths.Config)
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
			if !m.dry {
				if err = m.save(r, managed.State{Kind: "block", Status: "active", Target: r.Target, Reference: "-", Backup: "-", Checksum: expected}); err != nil {
					return err
				}
			}
			m.unchanged++
		}
		return nil
	}
	if has && view.Status == "intact" && view.Checksum != s.Checksum {
		if m.dry {
			if !preflight {
				m.say(report.Preview, "Would preserve modified managed block: %s", r.Target)
			}
			return nil
		}
		if overwrite, err := m.resolveModified(r, "block", "Overwrite the Selfishell block?", preflight); err != nil || !overwrite {
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
		if m.dry {
			m.say(report.Preview, "Would update Selfishell block: %s", r.Target)
			return nil
		}
		if err = m.save(r, managed.State{Kind: "block", Status: "pending", Target: r.Target, Reference: "-", Backup: "-", Checksum: view.Checksum}); err != nil {
			return err
		}
		data = managed.SpliceBlock(data, view, content)
	} else {
		if m.dry {
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
	if err = m.write(r.Target, data, mode); err != nil {
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
