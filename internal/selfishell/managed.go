package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type managed struct {
	c         CLI
	paths     Paths
	dry, yes  bool
	unchanged int
	actions   map[string]string
}

func exists(path string) (os.FileInfo, bool, error) {
	i, e := os.Lstat(path)
	if errors.Is(e, fs.ErrNotExist) {
		return nil, false, nil
	}
	return i, e == nil, e
}
func (m *managed) statePath(r Resource) string { return m.paths.Resources + "/" + r.Name + ".state" }
func (m *managed) state(r Resource) (State, bool, error) {
	s, e := ReadState(m.statePath(r))
	if errors.Is(e, fs.ErrNotExist) {
		return State{}, false, nil
	}
	return s, e == nil, e
}
func (m *managed) save(r Resource, s State) error { return WriteState(m.statePath(r), s) }
func (m *managed) say(format string, args ...any) { fmt.Fprintf(m.c.Out, format+"\n", args...) }
func (m *managed) backup(path string) (string, error) {
	var out bytes.Buffer
	code, err := (Process{Out: &out, Err: m.c.Err}).Run(context.Background(), "date", "+%Y%m%d%H%M%S")
	if err != nil || code != 0 {
		return "", fmt.Errorf("could not generate backup timestamp")
	}
	base := path + ".backup." + strings.TrimSpace(out.String())
	candidate := base
	for n := 1; ; n++ {
		_, present, e := exists(candidate)
		if e != nil {
			return "", e
		}
		if !present {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s.%d", base, n)
	}
}
func (m *managed) installResource(r Resource, preflight bool) error {
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
func (m *managed) installFile(r Resource, preflight bool) error {
	source, err := os.ReadFile(r.Source)
	if err != nil {
		return err
	}
	sourceChecksum, err := checksumBytes(source)
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
	info, present, err := exists(r.Target)
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
	current := ""
	if present && info.Mode().IsRegular() {
		current, err = Checksum(context.Background(), r.Target)
		if err != nil {
			return err
		}
	}
	if has && current != "" && current != s.Checksum && current != sourceChecksum {
		backupExists := false
		if backup != "-" {
			_, backupExists, _ = exists(backup)
		}
		if s.Status == "active" || backup == "-" || backupExists {
			if m.dry {
				if !preflight {
					m.say("Conflict: modified managed file: %s", r.Target)
					m.say("Would require an overwrite or skip decision.")
				}
				return nil
			}
			action := m.actions[r.Name]
			if action == "" {
				if m.yes || !m.c.interactive() {
					return fmt.Errorf("Managed file was modified; preserving it: %s", r.Target)
				}
				fmt.Fprintf(m.c.Out, "Managed file was modified: %s. Overwrite with default config? [y/N] ", r.Target)
				answer, _ := m.c.readAnswer()
				if affirmative(answer) {
					action = "overwrite"
				} else {
					action = "skip"
				}
				m.actions[r.Name] = action
			}
			if preflight {
				return nil
			}
			if action == "skip" {
				m.say("Skipped modified managed file: %s", r.Target)
				return nil
			}
			conflict, err := m.backup(m.paths.State + "/backups/" + r.Name)
			if err != nil {
				return err
			}
			if err = makeRawDir(rawParent(conflict)); err != nil {
				return err
			}
			if err = copyPreserve(r.Target, conflict); err != nil {
				return err
			}
			m.say("Backed up modified managed file: %s -> %s", r.Target, conflict)
		}
	}
	if preflight {
		return nil
	}
	if current == sourceChecksum {
		if !m.dry {
			if err = m.save(r, State{"file", "active", r.Target, "-", backup, sourceChecksum}); err != nil {
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
		m.say("Would %s managed file: %s", map[bool]string{true: "update", false: "install"}[active], r.Target)
		return nil
	}
	if err = m.save(r, State{"file", "pending", r.Target, "-", backup, sourceChecksum}); err != nil {
		return err
	}
	if backup != "-" {
		_, backupPresent, _ := exists(backup)
		if !backupPresent && present {
			if err = makeRawDir(rawParent(backup)); err != nil {
				return err
			}
			if err = os.Rename(r.Target, backup); err != nil {
				return err
			}
		}
	}
	if err = writeAtomic(r.Target, source, 0644); err != nil {
		return err
	}
	if err = m.save(r, State{"file", "active", r.Target, "-", backup, sourceChecksum}); err != nil {
		return err
	}
	m.say("%s managed file: %s", verb, r.Target)
	return nil
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
	return writeAtomic(target, data, i.Mode().Perm())
}
func (m *managed) installLink(r Resource, preflight bool) error {
	s, has, err := m.state(r)
	if err != nil {
		return fmt.Errorf("Managed resource state is malformed: %s", m.statePath(r))
	}
	if has && (s.Kind != "link" || s.Target != r.Target) {
		return fmt.Errorf("State conflict for managed link: %s", r.Name)
	}
	info, present, err := exists(r.Target)
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
					if err = m.save(r, State{"link", "active", r.Target, r.Source, backup, "-"}); err != nil {
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
		m.say("Would link: %s -> %s", r.Target, r.Source)
		return nil
	}
	if err = m.save(r, State{"link", "pending", r.Target, r.Source, backup, "-"}); err != nil {
		return err
	}
	if err = makeRawDir(rawParent(r.Target)); err != nil {
		return err
	}
	moved := false
	if backup != "-" && present {
		_, b, _ := exists(backup)
		if !b {
			if err = os.Rename(r.Target, backup); err != nil {
				return err
			}
			moved = true
		}
	}
	_, present, _ = exists(r.Target)
	if !present {
		if err = os.Symlink(r.Source, r.Target); err != nil {
			if moved {
				os.Rename(backup, r.Target)
				os.Remove(m.statePath(r))
			}
			return err
		}
	}
	if err = m.save(r, State{"link", "active", r.Target, r.Source, backup, "-"}); err != nil {
		return err
	}
	m.say("Linked: %s -> %s", r.Target, r.Source)
	return nil
}
func (m *managed) installBlock(r Resource, preflight bool) error {
	content, err := blockContent(r.Name, m.paths.Config)
	if err != nil {
		return err
	}
	expected, err := checksumBytes(content)
	if err != nil {
		return err
	}
	info, present, err := exists(r.Target)
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
	view, err := inspectBlock(r.Name, data)
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
	if !has && view.status != "absent" {
		return fmt.Errorf("Cannot manage the Selfishell %s block in: %s", r.Name, r.Target)
	}
	if has && view.status == "intact" && view.checksum == expected {
		if !preflight {
			if !m.dry {
				if err = m.save(r, State{"block", "active", r.Target, "-", "-", expected}); err != nil {
					return err
				}
			}
			m.unchanged++
		}
		return nil
	}
	if has && view.status == "intact" && view.checksum != s.Checksum {
		if m.dry {
			if !preflight {
				m.say("Would preserve modified managed block: %s", r.Target)
			}
			return nil
		}
		action := m.actions[r.Name]
		if action == "" {
			if m.yes || !m.c.interactive() {
				return fmt.Errorf("Managed block was modified; preserving it: %s", r.Target)
			}
			fmt.Fprintf(m.c.Out, "Managed block was modified: %s. Overwrite the Selfishell block? [y/N] ", r.Target)
			answer, _ := m.c.readAnswer()
			if affirmative(answer) {
				action = "overwrite"
			} else {
				action = "skip"
			}
			m.actions[r.Name] = action
		}
		if preflight {
			return nil
		}
		if action == "skip" {
			m.say("Skipped modified managed block: %s", r.Target)
			return nil
		}
		conflict, err := m.backup(m.paths.State + "/backups/" + r.Name)
		if err != nil {
			return err
		}
		if err = copyPreserve(r.Target, conflict); err != nil {
			return err
		}
		m.say("Backed up modified managed block: %s -> %s", r.Target, conflict)
	}
	if has && view.status != "intact" && (s.Status != "pending" || view.status != "absent") {
		return fmt.Errorf("Cannot manage the Selfishell %s block in: %s", r.Name, r.Target)
	}
	if preflight {
		return nil
	}
	if view.status == "intact" {
		if m.dry {
			m.say("Would update Selfishell block: %s", r.Target)
			return nil
		}
		if err = m.save(r, State{"block", "pending", r.Target, "-", "-", view.checksum}); err != nil {
			return err
		}
		data = spliceBlock(data, view, content)
	} else {
		if m.dry {
			m.say("Would add Selfishell block: %s", r.Target)
			return nil
		}
		if err = m.save(r, State{"block", "pending", r.Target, "-", "-", expected}); err != nil {
			return err
		}
		data = append(content, data...)
	}
	mode := os.FileMode(0644)
	if present {
		mode = info.Mode().Perm()
	}
	if err = writeAtomic(r.Target, data, mode); err != nil {
		return err
	}
	if err = m.save(r, State{"block", "active", r.Target, "-", "-", expected}); err != nil {
		return err
	}
	if view.status == "intact" {
		m.say("Updated Selfishell block: %s", r.Target)
	} else {
		m.say("Added Selfishell block: %s", r.Target)
	}
	return nil
}
