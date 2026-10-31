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

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/report"
)

// Ownership record for the fragment; written before the fragment first exists.
type windowsFragmentRecord struct {
	Version  int    `json:"version"`
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

func windowsFragmentRecordPath(paths managed.Paths) string {
	return paths.State + "/windows-terminal-fragment.json"
}

func readWindowsFragmentRecord(paths managed.Paths) (*windowsFragmentRecord, error) {
	data, err := managed.ReadFile(windowsFragmentRecordPath(paths))
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

// windowsOverrides says which user settings take precedence over the fragment.
func windowsOverrides(choice *windowsTerminalChoice, say func(string)) {
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

// windowsLifecycle is the configuration lifecycle that installs and removes
// the fragment: its reports, dry-run, backups, conflict prompts and writes.
type windowsLifecycle interface {
	report.Reporter
	DryRun() bool
	// Unchanged counts a resource that already had its managed content.
	Unchanged()
	Backup(base string) (string, error)
	ResolveModified(r managed.Resource, kind, question string, preflight bool) (bool, error)
	Write(path string, data []byte, mode os.FileMode) error
}

// installFragment writes choice's fragment and its ownership record; preflight
// only checks that it could.
func installFragment(l windowsLifecycle, paths managed.Paths, choice *windowsTerminalChoice, preflight bool) error {
	if choice == nil || !choice.Enabled {
		return nil
	}
	say := func(tone report.Tone, format string, args ...any) { l.Report("Configuration", tone, format, args...) }
	j, profile, err := readWindowsProfile(choice.SettingsPath, choice.ProfileGUID)
	if errors.Is(err, os.ErrNotExist) || (err == nil && profile == nil) {
		if !preflight {
			say(report.Warning, "Existing Windows Terminal profile is missing; skipping its font and theme.")
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
	record, err := readWindowsFragmentRecord(paths)
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
		if err := removeRecordedFragment(l, record, preflight); err != nil {
			return err
		}
		record = nil
	}
	info, present, err := managed.Exists(path)
	if err != nil {
		return err
	}
	if present && !info.Mode().IsRegular() {
		return fmt.Errorf("Windows Terminal fragment path changed type; preserving it: %s", path)
	}
	sum, err := managed.ChecksumBytes(content)
	if err != nil {
		return err
	}
	var current []byte
	if present {
		if current, err = managed.ReadFile(path); err != nil {
			return err
		}
	}
	currentSum, err := managed.ChecksumBytes(current)
	if err != nil {
		return err
	}
	foreign := present && currentSum != sum && (record == nil || currentSum != record.Checksum)
	if foreign && record != nil {
		if l.DryRun() {
			if !preflight {
				say(report.Warning, "Conflict: modified Windows Terminal fragment: %s", path)
				say(report.Preview, "Would require an overwrite or skip decision.")
			}
			return nil
		}
		r := managed.Resource{Name: "windows-terminal-fragment", Target: path}
		if overwrite, err := l.ResolveModified(r, "Windows Terminal fragment", "Overwrite with the Selfishell fragment?", preflight); err != nil || !overwrite {
			return err
		}
	}
	if preflight {
		return nil
	}
	if currentSum == sum {
		l.Unchanged()
	} else if l.DryRun() {
		say(report.Preview, "Would apply the font and Dark+ to Windows Terminal profile %q through a fragment: %s", name, path)
	}
	saveRecord := func() error {
		data, err := json.Marshal(windowsFragmentRecord{1, path, sum})
		if err != nil {
			return err
		}
		return l.Write(windowsFragmentRecordPath(paths), append(data, '\n'), 0600)
	}
	if !l.DryRun() && record == nil {
		if err := saveRecord(); err != nil {
			return err
		}
	}
	if !l.DryRun() && currentSum != sum {
		if foreign && record == nil {
			backup, err := l.Backup(paths.State + "/backups/windows-terminal-fragment")
			if err != nil {
				return err
			}
			if err := managed.CreateRawExclusive(backup, current, 0600); err != nil {
				return err
			}
			say(report.Success, "Backed up existing Windows Terminal fragment: %s -> %s", path, backup)
		}
		if err := l.Write(path, content, 0644); err != nil {
			return err
		}
		say(report.Success, "Applied the font and Dark+ to Windows Terminal profile %q through a fragment: %s", name, path)
	}
	// After a content change, the new bytes stay recognizable until this update.
	if !l.DryRun() && record != nil && record.Checksum != sum {
		if err := saveRecord(); err != nil {
			return err
		}
	}
	windowsOverrides(choice, func(message string) { l.Report("Notes", report.Warning, "%s", message) })
	return nil
}

// removeFragment removes the recorded fragment if intact, and its record; a
// record copied from another distribution is forgotten without touching it.
func removeFragment(l windowsLifecycle, paths managed.Paths, distro string, preflight bool) error {
	record, err := readWindowsFragmentRecord(paths)
	if err != nil || record == nil {
		return err
	}
	if choice, err := readWindowsTerminalChoice(paths); err == nil && choice.foreign(distro) {
		if preflight || l.DryRun() {
			return nil
		}
		return os.Remove(windowsFragmentRecordPath(paths))
	}
	if err := removeRecordedFragment(l, record, preflight); err != nil || preflight || l.DryRun() {
		return err
	}
	return os.Remove(windowsFragmentRecordPath(paths))
}

func removeRecordedFragment(l windowsLifecycle, record *windowsFragmentRecord, preflight bool) error {
	info, present, err := managed.Exists(record.Path)
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
		data, err := managed.ReadFile(record.Path)
		if err != nil {
			return err
		}
		sum, err := managed.ChecksumBytes(data)
		if err != nil {
			return err
		}
		intact = sum == record.Checksum
	}
	if l.DryRun() {
		if intact {
			l.Report("Configuration", report.Preview, "Would remove Windows Terminal fragment: %s", record.Path)
		}
		return nil
	}
	if intact {
		if err := os.Remove(record.Path); err != nil {
			return err
		}
		syscall.Rmdir(managed.RawParent(record.Path))
	} else if present {
		l.Report("Configuration", report.Warning, "Preserving modified Windows Terminal fragment: %s", record.Path)
	}
	return nil
}

// windowsStatus diagnoses the fragment and its records through say, which
// prints a colored label and a message.
func windowsStatus(paths managed.Paths, distro string, verbose bool, say func(color, label, message string)) (tracked, intact, recordIssue bool) {
	record, err := readWindowsFragmentRecord(paths)
	if err != nil {
		say("31", "MALFORMED", windowsFragmentRecordPath(paths))
		return true, false, true
	}
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil {
		say("31", "MALFORMED", paths.State+"/windows-terminal.json")
		return true, false, true
	}
	if choice.foreign(distro) {
		say("36", "INFO", fmt.Sprintf("Windows Terminal setup was copied from WSL distribution %q; run 'selfishell install' here to set it up", choice.Distro))
		return false, false, false
	}
	if record == nil {
		if choice != nil && choice.Enabled {
			say("31", "MISSING", "Installation record: "+windowsFragmentRecordPath(paths))
			return true, false, true
		}
		return false, false, false
	}
	if choice != nil && choice.Enabled {
		// An interrupted re-target saved the new profile before moving the fragment.
		if path, _, err := windowsFragment(choice); err == nil && path != record.Path {
			say("33", "MISSING", path+" (Windows Terminal fragment); run 'selfishell install' to finish moving it")
			return true, false, false
		}
	}
	data, err := managed.ReadFile(record.Path)
	sum, _ := managed.ChecksumBytes(data)
	intact = err == nil && sum == record.Checksum
	label := record.Path + " (Windows Terminal fragment)"
	if !intact {
		say("33", "CHANGED", label)
		return true, false, false
	}
	if choice != nil && choice.Enabled && windowsProfileMissing(choice) {
		say("33", "MISSING", fmt.Sprintf("Windows Terminal profile %s; run 'selfishell install' to select the current profile", choice.ProfileGUID))
		return true, false, false
	}
	if verbose {
		say("32", "OK", label)
	}
	if choice != nil && choice.Enabled {
		windowsOverrides(choice, func(message string) { say("36", "INFO", message) })
	}
	return true, true, false
}

func readWindowsProfile(path, guid string) (*terminalJSON, *terminalJSONNode, error) {
	data, err := managed.ReadFile(path)
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
