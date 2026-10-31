package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/report"
)

// ErrInterruptedInstall reports a resource an interrupted install left pending.
var ErrInterruptedInstall = errors.New("An interrupted install left this unfinished; run 'selfishell install', then uninstall again.")

// PreflightUninstall checks that record can be removed, and restored when
// restore is set, without changing anything.
func (m *Operation) PreflightUninstall(record ResourceState, restore bool) error {
	s := record.State
	r := record.Resource
	if s.Status == "restoring" {
		return preflightRestore(s, restore)
	}
	info, present, err := managed.Exists(s.Target)
	if err != nil {
		return err
	}
	willRemove := false
	switch s.Kind {
	case "file":
		if present {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("Managed file path changed type; preserving it: %s", s.Target)
			}
			actual, e := managed.Checksum(context.Background(), s.Target)
			if e != nil {
				return e
			}
			if actual != s.Checksum {
				return fmt.Errorf("Managed file was modified; preserving it: %s", s.Target)
			}
			willRemove = true
		}
	case "link":
		if present {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("Managed link was replaced; preserving it: %s", s.Target)
			}
			dest, e := os.Readlink(s.Target)
			if e != nil {
				return e
			}
			if dest != s.Reference {
				return fmt.Errorf("Managed link was replaced; preserving it: %s", s.Target)
			}
			willRemove = true
		}
	case "block":
		if present && !info.Mode().IsRegular() {
			return fmt.Errorf("Managed block path changed type; preserving it: %s", s.Target)
		}
		if present {
			data, e := os.ReadFile(s.Target)
			if e != nil {
				return e
			}
			view, e := managed.InspectBlock(r.Name, data)
			if e != nil {
				return e
			}
			if view.Status != "absent" && (view.Status != "intact" || view.Checksum != s.Checksum) {
				return blockConflictError(r)
			}
		}
	}
	if restore && s.Backup != "-" {
		_, hasBackup, e := managed.Exists(s.Backup)
		if e != nil {
			return e
		}
		if !hasBackup {
			return missingRestoreBackup(s.Backup)
		}
		if present && !willRemove {
			return fmt.Errorf("Restore target is occupied; preserving backup: %s", s.Backup)
		}
	}
	if s.Status == "pending" {
		return ErrInterruptedInstall
	}
	return nil
}

// RemoveResource removes record's managed path and state, restoring its
// backup when restore is set; the restore is journaled before the move.
func (m *Operation) RemoveResource(record ResourceState, restore bool) error {
	r, s := record.Resource, record.State
	if err := m.PreflightUninstall(record, restore); err != nil {
		return err
	}
	if s.Status == "restoring" {
		return m.finishRestore(r, s)
	}
	info, present, err := managed.Exists(s.Target)
	if err != nil {
		return err
	}
	if s.Kind == "block" {
		if m.Dry {
			m.say(report.Preview, "Would remove Selfishell block: %s", s.Target)
		} else if present {
			data, e := os.ReadFile(s.Target)
			if e != nil {
				return e
			}
			view, e := managed.InspectBlock(r.Name, data)
			if e != nil {
				return e
			}
			if view.Status == "intact" {
				pending := s
				pending.Status = "pending"
				if err = m.save(r, pending); err != nil {
					return err
				}
				if err = m.Write(s.Target, managed.SpliceBlock(data, view, nil), info.Mode().Perm()); err != nil {
					return err
				}
			}
		}
	} else if present {
		if m.Dry {
			if s.Kind == "link" {
				m.say(report.Preview, "Would remove managed link: %s", s.Target)
			} else {
				m.say(report.Preview, "Would remove managed file: %s", s.Target)
			}
		} else if err = os.Remove(s.Target); err != nil {
			return err
		}
	}
	if restore && s.Backup != "-" {
		_, backupPresent, e := managed.Exists(s.Backup)
		if e != nil {
			return e
		}
		if !backupPresent {
			return missingRestoreBackup(s.Backup)
		}
		if m.Dry {
			m.say(report.Preview, "Would restore: %s -> %s", s.Backup, s.Target)
		} else {
			if _, occupied, _ := managed.Exists(s.Target); occupied {
				return fmt.Errorf("Restore target is occupied; preserving backup: %s", s.Backup)
			}
			// Journal the restore before moving the backup. A retry must never
			// mistake the restored user path for a managed path to remove.
			s.Status = "restoring"
			if err = m.save(r, s); err != nil {
				return err
			}
			return m.finishRestore(r, s)
		}
	}
	if !restore && s.Backup != "-" {
		m.say(report.Info, "Original backup location: %s", s.Backup)
	}
	return m.forgetResource(r)
}

func interruptedRestore(target string) error {
	return fmt.Errorf("An interrupted restore is unfinished for: %s. Run 'selfishell uninstall --restore' before installing again.", target)
}

func preflightRestore(s managed.State, restore bool) error {
	_, targetPresent, err := managed.Exists(s.Target)
	if err != nil {
		return err
	}
	_, backupPresent, err := managed.Exists(s.Backup)
	if err != nil {
		return err
	}
	if backupPresent {
		if targetPresent {
			return fmt.Errorf("Restore target is occupied; preserving backup: %s", s.Backup)
		}
		if !restore {
			return interruptedRestore(s.Target)
		}
	} else if !targetPresent {
		return fmt.Errorf("Restore target and recorded backup are both missing: %s; %s. Return the backup and retry 'selfishell uninstall --restore'.", s.Target, s.Backup)
	}
	return nil
}

func (m *Operation) finishRestore(r managed.Resource, s managed.State) error {
	if err := preflightRestore(s, true); err != nil {
		return err
	}
	_, backupPresent, err := managed.Exists(s.Backup)
	if err != nil {
		return err
	}
	if backupPresent {
		if m.Dry {
			m.say(report.Preview, "Would restore: %s -> %s", s.Backup, s.Target)
		} else {
			if err = managed.MakeRawDir(managed.RawParent(s.Target)); err != nil {
				return err
			}
			if err = moveBackupNoReplace(s.Backup, s.Target); err != nil {
				return err
			}
		}
	} else if m.Dry {
		m.say(report.Preview, "Would clear completed restore record: %s", s.Target)
	}
	return m.forgetResource(r)
}

func (m *Operation) forgetResource(r managed.Resource) error {
	if !m.Dry {
		if m.Hooks.RemoveState != nil {
			return m.Hooks.RemoveState(m.statePath(r))
		}
		return os.Remove(m.statePath(r))
	}
	return nil
}
func missingRestoreBackup(path string) error {
	return fmt.Errorf("Recorded backup is missing: %s. Return it and retry, or uninstall without --restore.", path)
}
