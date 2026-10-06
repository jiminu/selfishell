package selfishell

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Ownership record for the fragment; written before the fragment first exists.
type windowsFragmentRecord struct {
	Version  int    `json:"version"`
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

func windowsFragmentRecordPath(paths Paths) string {
	return paths.State + "/windows-terminal-fragment.json"
}

func readWindowsFragmentRecord(paths Paths) (*windowsFragmentRecord, error) {
	data, err := readStateFile(windowsFragmentRecordPath(paths))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r windowsFragmentRecord
	if json.Unmarshal(data, &r) != nil || r.Version != 1 || !filepath.IsAbs(r.Path) || r.Checksum == "" {
		return nil, fmt.Errorf("invalid Windows Terminal fragment record: %s", windowsFragmentRecordPath(paths))
	}
	return &r, nil
}

// Windows Terminal layers this fragment below the profile and Defaults in settings.json.
func windowsFragment(choice *windowsTerminalChoice) (string, []byte, error) {
	profile := struct {
		Updates     string            `json:"updates"`
		Font        map[string]string `json:"font"`
		ColorScheme string            `json:"colorScheme"`
	}{choice.ProfileGUID, map[string]string{"face": terminalFont}, "Dark+"}
	data, err := json.MarshalIndent(map[string]any{"profiles": []any{profile}}, "", "  ")
	path := choice.AppDataPath + "/Microsoft/Windows Terminal/Fragments/Selfishell/" + strings.Trim(choice.ProfileGUID, "{}") + ".json"
	return path, append(data, '\n'), err
}

// User settings take precedence over the fragment; a legacy fontFace counts
// only without a font object in the same layer.
func windowsTerminalOverrides(j *terminalJSON, profile *terminalJSONNode) ([]string, error) {
	var defaults *terminalJSONNode
	if profiles := j.root.property("profiles"); profiles != nil && profiles.object {
		if err := profiles.unique("defaults"); err != nil {
			return nil, err
		}
		defaults = profiles.property("defaults")
	}
	face, _ := json.Marshal(terminalFont)
	var found []string
	for _, layer := range []struct {
		name string
		node *terminalJSONNode
	}{{"profile", profile}, {"Defaults", defaults}} {
		if err := layer.node.unique("font", "fontFace", "colorScheme"); err != nil {
			return nil, err
		}
		font := layer.node.property("font")
		if err := font.unique("face"); err != nil {
			return nil, err
		}
		values := map[string]*terminalJSONNode{"font.face": font.property("face"), "colorScheme": layer.node.property("colorScheme")}
		if font == nil {
			values["fontFace"] = layer.node.property("fontFace")
		}
		for _, key := range []string{"font.face", "fontFace", "colorScheme"} {
			want := face
			if key == "colorScheme" {
				want = []byte(`"Dark+"`)
			}
			if value := j.value(values[key]); value != nil && !bytes.Equal(value, want) {
				found = append(found, fmt.Sprintf("%s %s %s", layer.name, key, value))
			}
		}
	}
	return found, nil
}

func (c CLI) noteWindowsTerminalOverrides(choice *windowsTerminalChoice, say func(string)) {
	j, profile, err := readWindowsProfile(choice.SettingsPath, choice.ProfileGUID)
	var overrides []string
	if err == nil && profile != nil {
		overrides, err = windowsTerminalOverrides(j, profile)
	}
	if err != nil {
		say(fmt.Sprintf("Could not check Windows Terminal settings for an overriding font or theme: %s", err))
	}
	for _, o := range overrides {
		say(fmt.Sprintf("Windows Terminal %s takes precedence over Selfishell's; remove it there to apply the font and Dark+.", o))
	}
}

func (m *managed) installWindowsTerminal(choice *windowsTerminalChoice, preflight bool) error {
	if choice == nil || !choice.Enabled {
		return nil
	}
	// Releases 1.6.3-1.6.5 edited settings.json; restore it before the fragment applies.
	if err := m.removeWindowsProfile(preflight); err != nil {
		return err
	}
	j, profile, err := readWindowsProfile(choice.SettingsPath, choice.ProfileGUID)
	if errors.Is(err, os.ErrNotExist) || (err == nil && profile == nil) {
		if !preflight {
			m.say(reportWarning, "Existing Windows Terminal profile is missing; skipping its font and theme.")
		}
		return nil
	}
	name := choice.ProfileGUID
	if err == nil {
		name = j.text(profile.property("name"))
	}
	path, content, err := windowsFragment(choice)
	if err != nil {
		return err
	}
	record, err := readWindowsFragmentRecord(m.paths)
	if err != nil {
		return err
	}
	if record != nil && record.Path != path {
		return fmt.Errorf("Windows Terminal fragment record does not match this setup; run 'selfishell uninstall' before retrying: %s", record.Path)
	}
	info, present, err := exists(path)
	if err != nil {
		return err
	}
	if present && !info.Mode().IsRegular() {
		return fmt.Errorf("Windows Terminal fragment path changed type; preserving it: %s", path)
	}
	sum, err := checksumBytes(content)
	if err != nil {
		return err
	}
	var current []byte
	if present {
		if current, err = readStateFile(path); err != nil {
			return err
		}
	}
	currentSum, err := checksumBytes(current)
	if err != nil {
		return err
	}
	foreign := present && currentSum != sum && (record == nil || currentSum != record.Checksum)
	if foreign && record != nil {
		if m.dry {
			if !preflight {
				m.say(reportWarning, "Conflict: modified Windows Terminal fragment: %s", path)
				m.say(reportPreview, "Would require an overwrite or skip decision.")
			}
			return nil
		}
		r := Resource{Name: "windows-terminal-fragment", Target: path}
		if overwrite, err := m.resolveModified(r, "Windows Terminal fragment", "Overwrite with the Selfishell fragment?", preflight); err != nil || !overwrite {
			return err
		}
	}
	if preflight {
		return nil
	}
	if currentSum == sum {
		m.unchanged++
	} else if m.dry {
		m.say(reportPreview, "Would apply the font and Dark+ to Windows Terminal profile %q through a fragment: %s", name, path)
	}
	saveRecord := func() error {
		data, err := json.Marshal(windowsFragmentRecord{1, path, sum})
		if err != nil {
			return err
		}
		return m.write(windowsFragmentRecordPath(m.paths), append(data, '\n'), 0600)
	}
	if !m.dry && record == nil {
		if err := saveRecord(); err != nil {
			return err
		}
	}
	if !m.dry && currentSum != sum {
		if foreign && record == nil {
			backup, err := m.backup(m.paths.State + "/backups/windows-terminal-fragment")
			if err != nil {
				return err
			}
			if err := createRawExclusive(backup, current, 0600); err != nil {
				return err
			}
			m.say(reportSuccess, "Backed up existing Windows Terminal fragment: %s -> %s", path, backup)
		}
		if err := m.write(path, content, 0644); err != nil {
			return err
		}
		m.say(reportSuccess, "Applied the font and Dark+ to Windows Terminal profile %q through a fragment: %s", name, path)
	}
	// After a content change, the new bytes stay recognizable until this update.
	if !m.dry && record != nil && record.Checksum != sum {
		if err := saveRecord(); err != nil {
			return err
		}
	}
	m.c.noteWindowsTerminalOverrides(choice, func(message string) { m.c.report("Notes", reportWarning, "%s", message) })
	return nil
}

func (m *managed) removeWindowsTerminal(preflight bool) error {
	if err := m.removeWindowsProfile(preflight); err != nil {
		return err
	}
	record, err := readWindowsFragmentRecord(m.paths)
	if err != nil || record == nil {
		return err
	}
	info, present, err := exists(record.Path)
	if err != nil {
		return err
	}
	if present && !info.Mode().IsRegular() {
		return fmt.Errorf("Windows Terminal fragment path changed type; preserving it: %s", record.Path)
	}
	if preflight {
		return nil
	}
	intact := false
	if present {
		data, err := readStateFile(record.Path)
		if err != nil {
			return err
		}
		sum, err := checksumBytes(data)
		if err != nil {
			return err
		}
		intact = sum == record.Checksum
	}
	if m.dry {
		if intact {
			m.say(reportPreview, "Would remove Windows Terminal fragment: %s", record.Path)
		}
		return nil
	}
	if intact {
		if err := os.Remove(record.Path); err != nil {
			return err
		}
		syscall.Rmdir(rawParent(record.Path))
	} else if present {
		m.say(reportWarning, "Preserving modified Windows Terminal fragment: %s", record.Path)
	}
	return os.Remove(windowsFragmentRecordPath(m.paths))
}

func (c CLI) statusWindowsTerminal(paths Paths, verbose bool) (tracked, intact, recordIssue bool) {
	legacy, err := readWindowsProfileState(paths)
	if err != nil {
		c.sayDiagnostic("31", "MALFORMED", windowsProfileStatePath(paths))
		return true, false, true
	}
	if legacy != nil {
		c.sayDiagnostic("36", "INFO", "Windows Terminal settings from an earlier Selfishell; the next install or update moves them to a fragment")
		return true, true, false
	}
	record, err := readWindowsFragmentRecord(paths)
	if err != nil {
		c.sayDiagnostic("31", "MALFORMED", windowsFragmentRecordPath(paths))
		return true, false, true
	}
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil {
		c.sayDiagnostic("31", "MALFORMED", paths.State+"/windows-terminal.json")
		return true, false, true
	}
	if record == nil {
		if choice != nil && choice.Enabled {
			c.sayDiagnostic("31", "MISSING", "Installation record: "+windowsFragmentRecordPath(paths))
			return true, false, true
		}
		return false, false, false
	}
	data, err := readStateFile(record.Path)
	sum, _ := checksumBytes(data)
	intact = err == nil && sum == record.Checksum
	label := record.Path + " (Windows Terminal fragment)"
	if !intact {
		c.sayDiagnostic("33", "CHANGED", label)
		return true, false, false
	}
	if verbose {
		c.sayDiagnostic("32", "OK", label)
	}
	if choice != nil && choice.Enabled {
		c.noteWindowsTerminalOverrides(choice, func(message string) { c.sayDiagnostic("36", "INFO", message) })
	}
	return true, true, false
}

// Releases 1.6.3-1.6.5 edited settings.json directly. Their journal only
// supports restoring the values Selfishell applied; remove it after 1.6.6.
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
	for _, value := range []*json.RawMessage{&s.OriginalFace, &s.OriginalScheme, &s.AppliedFace, &s.AppliedScheme} {
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

func writeWindowsSettings(path string, before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Windows Terminal settings path changed; preserving it: %s", path)
	}
	mode := info.Mode().Perm()
	return writeRaw(path, after, &mode, func(temp string) error {
		current, err := readStateFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, before) {
			return fmt.Errorf("Windows Terminal settings changed during setup; preserving them: %s", path)
		}
		return os.Rename(temp, path)
	})
}

func (j *terminalJSON) restoreProfileAppearance(profile *terminalJSONNode, face, scheme json.RawMessage, removeEmptyFont bool) ([]byte, error) {
	var edits []terminalJSONEdit
	if font := profile.property("font"); font != nil {
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

// Restores only values still equal to what Selfishell applied, so a pending
// journal is safe too; a removed target has nothing left to restore.
func (m *managed) removeWindowsProfile(preflight bool) error {
	s, err := readWindowsProfileState(m.paths)
	if err != nil || s == nil {
		return err
	}
	j, profile, err := readWindowsProfile(s.SettingsPath, s.GUID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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
		after, err := j.restoreProfileAppearance(profile, restoreFace, restoreScheme, s.FontWasAbsent)
		if err != nil {
			return err
		}
		s.Status = "restoring"
		if err := m.saveWindowsProfile(s); err != nil {
			return err
		}
		if err := writeWindowsSettings(s.SettingsPath, j.data, after); err != nil {
			return err
		}
		if !bytes.Equal(j.data, after) {
			m.say(reportSuccess, "Restored the Windows Terminal font and theme in settings.json: %s", s.SettingsPath)
		}
	}
	return os.Remove(windowsProfileStatePath(m.paths))
}
