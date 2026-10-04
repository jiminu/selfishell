package selfishell

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// This JSON journal is separate from the fixed-line managed-resource format.
// Original values survive reinstall; pending values recover either side of an
// interrupted atomic settings write. The full initial file is also backed up.
type windowsProfileState struct {
	Version            int             `json:"version"`
	Status             string          `json:"status"`
	SettingsPath       string          `json:"settingsPath"`
	GUID               string          `json:"guid"`
	Backup             string          `json:"backup"`
	BackupChecksum     string          `json:"backupChecksum"`
	FontWasAbsent      bool            `json:"fontWasAbsent"`
	OriginalFace       json.RawMessage `json:"originalFace"`
	OriginalScheme     json.RawMessage `json:"originalScheme"`
	OriginalFaceText   string          `json:"originalFaceText,omitempty"`
	OriginalSchemeText string          `json:"originalSchemeText,omitempty"`
	AppliedFace        json.RawMessage `json:"appliedFace"`
	AppliedScheme      json.RawMessage `json:"appliedScheme"`
	BeforeFace         json.RawMessage `json:"beforeFace,omitempty"`
	BeforeScheme       json.RawMessage `json:"beforeScheme,omitempty"`
}

func windowsProfileStatePath(paths Paths) string {
	return paths.State + "/windows-terminal-profile.json"
}

func readWindowsProfileState(paths Paths) (*windowsProfileState, error) {
	data, err := readStateFile(windowsProfileStatePath(paths))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s windowsProfileState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid Windows Terminal profile journal: %w", err)
	}
	if s.Version != 1 || (s.Status != "pending" && s.Status != "active" && s.Status != "restoring") || !filepath.IsAbs(s.SettingsPath) || !filepath.IsAbs(s.Backup) || !validTerminalGUID(s.GUID) || s.BackupChecksum == "" || len(s.AppliedFace) == 0 || len(s.AppliedScheme) == 0 {
		return nil, fmt.Errorf("invalid Windows Terminal profile journal")
	}
	// null represents a property that was absent; font and scheme null values
	// are rejected by the profile preflight so these remain unambiguous.
	for _, value := range []*json.RawMessage{&s.OriginalFace, &s.OriginalScheme, &s.AppliedFace, &s.AppliedScheme, &s.BeforeFace, &s.BeforeScheme} {
		if len(*value) == 0 || bytes.Equal(*value, []byte("null")) {
			*value = nil
		} else {
			var compact bytes.Buffer
			if err := json.Compact(&compact, *value); err != nil {
				return nil, err
			}
			*value = compact.Bytes()
		}
	}
	if s.AppliedFace == nil || s.AppliedScheme == nil {
		return nil, fmt.Errorf("invalid Windows Terminal profile journal values")
	}
	return &s, nil
}

func (m *managed) saveWindowsProfile(s *windowsProfileState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return m.write(windowsProfileStatePath(m.paths), append(data, '\n'), 0600)
}

func readWindowsProfile(path, guid string) (*terminalJSON, *terminalJSONNode, error) {
	data, err := readStateFile(path)
	if err != nil {
		return nil, nil, err
	}
	j, err := parseTerminalJSON(data)
	if err != nil {
		return nil, nil, err
	}
	profile, err := j.profile(guid)
	if err != nil || profile == nil {
		return j, profile, err
	}
	font := profile.property("font")
	if font != nil && !font.object {
		return nil, nil, fmt.Errorf("Windows Terminal profile font changed type; preserving it: %s", path)
	}
	for _, value := range []*terminalJSONNode{font.property("face"), profile.property("colorScheme")} {
		if string(j.value(value)) == "null" {
			return nil, nil, fmt.Errorf("Windows Terminal profile contains null settings; preserving it: %s", path)
		}
	}
	return j, profile, nil
}

func (m *managed) writeWindowsSettings(path string, before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Windows Terminal settings path changed; preserving it: %s", path)
	}
	publish := func(temp string) error {
		current, err := readStateFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, before) {
			return fmt.Errorf("Windows Terminal settings changed during setup; preserving them: %s", path)
		}
		return os.Rename(temp, path)
	}
	if m.atomicWrite != nil {
		current, err := readStateFile(path)
		if err != nil || !bytes.Equal(current, before) {
			return fmt.Errorf("Windows Terminal settings changed during setup: %s", path)
		}
		return m.write(path, after, info.Mode().Perm())
	}
	mode := info.Mode().Perm()
	return writeRaw(path, after, &mode, publish)
}

func (j *terminalJSON) setProfileAppearance(profile *terminalJSONNode, face, scheme json.RawMessage, removeEmptyFont bool) ([]byte, error) {
	var edits []terminalJSONEdit
	font := profile.property("font")
	if font == nil && face != nil {
		j.set(profile, "font", append(append([]byte(`{"face": `), face...), '}'), &edits)
	} else if font != nil {
		if removeEmptyFont && face == nil && len(font.members) == 1 && font.property("face") != nil && bytes.Equal(j.data[font.start:font.end], j.punctuation[font.start:font.end]) {
			j.set(profile, "font", nil, &edits)
		} else {
			j.set(font, "face", face, &edits)
		}
	}
	// Reparse after editing the font so adjacent removals each select the correct
	// separator. The resulting settings still publish in one atomic write.
	data, err := j.apply(edits)
	if err != nil {
		return nil, err
	}
	updated, err := parseTerminalJSON(data)
	if err != nil {
		return nil, err
	}
	selected, err := updated.profile(terminalGUID(j.text(profile.property("guid"))))
	if err != nil || selected == nil {
		return nil, fmt.Errorf("Windows Terminal profile disappeared during editing")
	}
	edits = nil
	updated.set(selected, "colorScheme", scheme, &edits)
	return updated.apply(edits)
}

func (m *managed) installWindowsProfile(choice *windowsTerminalChoice, preflight bool) error {
	if choice == nil || !choice.Enabled {
		return nil
	}
	s, err := readWindowsProfileState(m.paths)
	if err != nil {
		return err
	}
	if s != nil && (s.SettingsPath != choice.SettingsPath || s.GUID != choice.ProfileGUID || s.Status == "restoring") {
		return fmt.Errorf("Windows Terminal profile state conflicts with setup; finish uninstall before retrying")
	}
	j, profile, err := readWindowsProfile(choice.SettingsPath, choice.ProfileGUID)
	if errors.Is(err, os.ErrNotExist) || (err == nil && profile == nil) {
		if !preflight {
			m.say(reportWarning, "Existing Windows Terminal profile is missing; skipping its font and theme.")
		}
		return nil
	}
	if err != nil {
		return err
	}
	face, scheme := j.value(profile.property("font").property("face")), j.value(profile.property("colorScheme"))
	appliedFace, _ := json.Marshal(terminalFont)
	appliedScheme := json.RawMessage(`"Dark+"`)
	if s != nil {
		intact := bytes.Equal(face, s.AppliedFace) && bytes.Equal(scheme, s.AppliedScheme)
		if s.Status == "pending" {
			intact = intact || (bytes.Equal(face, s.BeforeFace) && bytes.Equal(scheme, s.BeforeScheme))
		}
		if !intact {
			if m.dry {
				if !preflight {
					m.say(reportPreview, "Would preserve modified Windows Terminal font or theme: %s", choice.SettingsPath)
				}
				return nil
			}
			r := Resource{Name: "windows-terminal-profile", Target: choice.SettingsPath}
			if overwrite, err := m.resolveModified(r, "Windows Terminal font or theme", "Reapply the Selfishell font and theme?", preflight); err != nil || !overwrite {
				return err
			}
		}
	}
	if preflight {
		return nil
	}
	if m.dry {
		m.say(reportPreview, "Would apply the font and Dark+ to existing Windows Terminal profile: %s", j.text(profile.property("name")))
		return nil
	}
	if s == nil {
		backup, err := m.backup(m.paths.State + "/backups/windows-terminal-settings")
		if err != nil {
			return err
		}
		sum, err := checksumBytes(j.data)
		if err != nil {
			return err
		}
		s = &windowsProfileState{Version: 1, SettingsPath: choice.SettingsPath, GUID: choice.ProfileGUID, Backup: backup, BackupChecksum: sum, FontWasAbsent: profile.property("font") == nil, OriginalFace: face, OriginalScheme: scheme}
		if n := profile.property("font").property("face"); n != nil {
			s.OriginalFaceText = string(j.data[n.start:n.end])
		}
		if n := profile.property("colorScheme"); n != nil {
			s.OriginalSchemeText = string(j.data[n.start:n.end])
		}
	}
	if s.Status == "active" && bytes.Equal(face, appliedFace) && bytes.Equal(scheme, appliedScheme) {
		m.unchanged++
		return nil
	}
	after, err := j.setProfileAppearance(profile, appliedFace, appliedScheme, false)
	if err != nil {
		return err
	}
	s.Status, s.BeforeFace, s.BeforeScheme = "pending", face, scheme
	s.AppliedFace, s.AppliedScheme = appliedFace, appliedScheme
	if err := m.saveWindowsProfile(s); err != nil {
		return err
	}
	if _, present, err := exists(s.Backup); err != nil {
		return err
	} else if !present {
		sum, err := checksumBytes(j.data)
		if err != nil || sum != s.BackupChecksum {
			return fmt.Errorf("missing original Windows Terminal backup; preserving settings")
		}
		if err := createRawExclusive(s.Backup, j.data, 0600); err != nil {
			return err
		}
	}
	if err := m.writeWindowsSettings(s.SettingsPath, j.data, after); err != nil {
		return err
	}
	s.Status, s.BeforeFace, s.BeforeScheme = "active", nil, nil
	if err := m.saveWindowsProfile(s); err != nil {
		return err
	}
	m.say(reportSuccess, "Applied font and Dark+ to existing Windows Terminal profile: %s", j.text(profile.property("name")))
	return nil
}

func (m *managed) removeWindowsProfile(preflight bool) error {
	s, err := readWindowsProfileState(m.paths)
	if err != nil || s == nil {
		return err
	}
	j, profile, err := readWindowsProfile(s.SettingsPath, s.GUID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if s.Status == "pending" {
		return errInterruptedInstall
	}
	if preflight {
		return nil
	}
	if m.dry {
		m.say(reportPreview, "Would restore unchanged Windows Terminal font and theme: %s", s.SettingsPath)
		return nil
	}
	if profile != nil {
		face, scheme := j.value(profile.property("font").property("face")), j.value(profile.property("colorScheme"))
		restoreFace := j.raw(profile.property("font").property("face"))
		restoreScheme := j.raw(profile.property("colorScheme"))
		if bytes.Equal(face, s.AppliedFace) {
			restoreFace = s.OriginalFace
			if s.OriginalFaceText != "" {
				restoreFace = json.RawMessage(s.OriginalFaceText)
			}
		} else if !bytes.Equal(face, s.OriginalFace) {
			m.say(reportWarning, "Preserving the user's changed Windows Terminal font.")
		}
		if bytes.Equal(scheme, s.AppliedScheme) {
			restoreScheme = s.OriginalScheme
			if s.OriginalSchemeText != "" {
				restoreScheme = json.RawMessage(s.OriginalSchemeText)
			}
		} else if !bytes.Equal(scheme, s.OriginalScheme) {
			m.say(reportWarning, "Preserving the user's changed Windows Terminal theme.")
		}
		after, err := j.setProfileAppearance(profile, restoreFace, restoreScheme, s.FontWasAbsent)
		if err != nil {
			return err
		}
		s.Status = "restoring"
		if err := m.saveWindowsProfile(s); err != nil {
			return err
		}
		if err := m.writeWindowsSettings(s.SettingsPath, j.data, after); err != nil {
			return err
		}
	}
	return os.Remove(windowsProfileStatePath(m.paths))
}

func (c CLI) statusWindowsProfile(paths Paths, verbose bool) (tracked, intact, recordIssue bool) {
	s, err := readWindowsProfileState(paths)
	if err != nil {
		c.sayDiagnostic("31", "MALFORMED", windowsProfileStatePath(paths))
		return true, false, true
	}
	if s == nil {
		choice, err := readWindowsTerminalChoice(paths)
		if err != nil {
			c.sayDiagnostic("31", "MALFORMED", paths.State+"/windows-terminal.json")
			return true, false, true
		}
		if choice != nil && choice.Enabled {
			c.sayDiagnostic("31", "MISSING", "Installation record: "+windowsProfileStatePath(paths))
			return true, false, true
		}
		return false, false, false
	}
	if s.Status != "active" {
		c.sayDiagnostic("33", "PENDING", windowsProfileStatePath(paths))
		return true, false, true
	}
	j, profile, err := readWindowsProfile(s.SettingsPath, s.GUID)
	label := s.SettingsPath + " (Windows Terminal font/theme)"
	if err == nil && profile != nil && bytes.Equal(j.value(profile.property("font").property("face")), s.AppliedFace) && bytes.Equal(j.value(profile.property("colorScheme")), s.AppliedScheme) {
		if verbose {
			c.sayDiagnostic("32", "OK", label)
		}
		return true, true, false
	}
	c.sayDiagnostic("33", "CHANGED", label)
	return true, false, false
}
