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
		check := func(key string, node *terminalJSONNode, want []byte, applied string) {
			if value := j.value(node); value != nil && !bytes.Equal(value, want) {
				found = append(found, fmt.Sprintf("%s %s %s takes precedence over %s", layer.name, key, value, applied))
			}
		}
		check("font.face", font.property("face"), face, "Selfishell's font")
		if font == nil {
			check("fontFace", layer.node.property("fontFace"), face, "Selfishell's font")
		}
		// An object sets each mode separately; a missing mode keeps the fragment's
		// Dark+. A light-mode scheme is the user's choice for a light theme.
		scheme, dark := layer.node.property("colorScheme"), []byte(`"Dark+"`)
		if scheme != nil && scheme.object {
			if err := scheme.unique("dark"); err != nil {
				return nil, err
			}
			check("colorScheme.dark", scheme.property("dark"), dark, "Dark+")
		} else {
			check("colorScheme", scheme, dark, "Dark+")
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
		say(fmt.Sprintf("Windows Terminal %s; remove it there to use Selfishell's.", o))
	}
}

func (m *managed) installWindowsTerminal(choice *windowsTerminalChoice, preflight bool) error {
	if choice == nil || !choice.Enabled {
		return nil
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
	if choice.replacesForeign {
		// Dry-run and preflight still see the copied record of another distribution.
		record = nil
	}
	// A copied record from another distribution is dropped before saving a new
	// choice, so a different path is this distribution's previous profile.
	if record != nil && record.Path != path {
		if err := m.removeWindowsFragment(record, preflight); err != nil {
			return err
		}
		record = nil
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
	record, err := readWindowsFragmentRecord(m.paths)
	if err != nil || record == nil {
		return err
	}
	if choice, err := readWindowsTerminalChoice(m.paths); err == nil && choice.foreign() {
		if preflight || m.dry {
			return nil
		}
		return os.Remove(windowsFragmentRecordPath(m.paths))
	}
	if err := m.removeWindowsFragment(record, preflight); err != nil || preflight || m.dry {
		return err
	}
	return os.Remove(windowsFragmentRecordPath(m.paths))
}

func (m *managed) removeWindowsFragment(record *windowsFragmentRecord, preflight bool) error {
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
	return nil
}

func (c CLI) statusWindowsTerminal(paths Paths, verbose bool) (tracked, intact, recordIssue bool) {
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
	if choice.foreign() {
		c.sayDiagnostic("36", "INFO", fmt.Sprintf("Windows Terminal setup was copied from WSL distribution %q; run 'selfishell install' here to set it up", choice.Distro))
		return false, false, false
	}
	if record == nil {
		if choice != nil && choice.Enabled {
			c.sayDiagnostic("31", "MISSING", "Installation record: "+windowsFragmentRecordPath(paths))
			return true, false, true
		}
		return false, false, false
	}
	if choice != nil && choice.Enabled {
		// An interrupted re-target saved the new profile before moving the fragment.
		if path, _, err := windowsFragment(choice); err == nil && path != record.Path {
			c.sayDiagnostic("33", "MISSING", path+" (Windows Terminal fragment); run 'selfishell install' to finish moving it")
			return true, false, false
		}
	}
	data, err := readStateFile(record.Path)
	sum, _ := checksumBytes(data)
	intact = err == nil && sum == record.Checksum
	label := record.Path + " (Windows Terminal fragment)"
	if !intact {
		c.sayDiagnostic("33", "CHANGED", label)
		return true, false, false
	}
	if choice != nil && choice.Enabled && windowsProfileMissing(choice) {
		c.sayDiagnostic("33", "MISSING", fmt.Sprintf("Windows Terminal profile %s; run 'selfishell install' to select the current profile", choice.ProfileGUID))
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
	return j, profile, err
}
