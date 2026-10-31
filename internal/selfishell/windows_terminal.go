package selfishell

import (
	"bytes"
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
)

//go:embed windows_terminal.ps1
var windowsTerminalScript string

// A separate versioned choice, not the fixed-line resource-state format.
type windowsTerminalChoice struct {
	Version      int    `json:"version"`
	Enabled      bool   `json:"enabled"`
	Distro       string `json:"distro,omitempty"`
	AppData      string `json:"appData,omitempty"`
	AppDataPath  string `json:"appDataPath,omitempty"`
	SettingsPath string `json:"settingsPath,omitempty"`
	ProfileGUID  string `json:"profileGuid,omitempty"`
	// Not saved: a choice moved to a new profile, replacing a choice copied
	// from another distribution, or keeping a choice only for this run.
	Retargeted                 bool `json:"-"`
	replacesForeign, transient bool
}

// A cloned distribution inherits the original's state; that fragment is not its own.
func (w *windowsTerminalChoice) foreign(distro string) bool {
	return w != nil && w.Enabled && distro != "" && w.Distro != distro
}

func windowsProfileMissing(choice *windowsTerminalChoice) bool {
	_, profile, err := readWindowsProfile(choice.SettingsPath, choice.ProfileGUID)
	if err != nil || profile == nil {
		return errors.Is(err, os.ErrNotExist) || err == nil
	}
	// Windows Terminal keeps settings stubs of profiles WSL no longer generates.
	return terminalGUID(choice.ProfileGUID) != legacyWSLProfileGUID(choice.Distro) && !wslFragmentListsProfile(choice)
}

// Mirrors the probe offline: WSL lists its current profiles in its own fragments.
func wslFragmentListsProfile(choice *windowsTerminalChoice) bool {
	dir := choice.AppDataPath + "/Microsoft/Windows Terminal/Fragments/Microsoft.WSL"
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := managed.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			continue
		}
		j, err := parseTerminalJSON(data)
		if err != nil || j.root.property("profiles") == nil {
			continue
		}
		for _, profile := range j.root.property("profiles").items {
			if j.text(profile.property("name")) == choice.Distro && terminalGUID(j.text(profile.property("guid"))) == terminalGUID(choice.ProfileGUID) {
				return true
			}
		}
	}
	return false
}

func readWindowsTerminalChoice(paths managed.Paths) (*windowsTerminalChoice, error) {
	data, err := managed.ReadFile(paths.State + "/windows-terminal.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var choice windowsTerminalChoice
	if err := json.Unmarshal(data, &choice); err != nil {
		return nil, fmt.Errorf("invalid Windows Terminal choice: %w", err)
	}
	if choice.Version != 1 {
		return nil, fmt.Errorf("unsupported Windows Terminal choice version: %d", choice.Version)
	}
	if choice.Enabled {
		for _, value := range []string{choice.Distro, choice.AppData, choice.AppDataPath, choice.SettingsPath, choice.ProfileGUID} {
			if value == "" || strings.ContainsAny(value, "\x00\r\n") {
				return nil, fmt.Errorf("invalid Windows Terminal choice")
			}
		}
		if !filepath.IsAbs(choice.SettingsPath) || !filepath.IsAbs(choice.AppDataPath) || !validTerminalGUID(choice.ProfileGUID) {
			return nil, fmt.Errorf("invalid Windows Terminal paths")
		}
	}
	return &choice, nil
}

// windowsSetup selects the Windows Terminal profile for this WSL distribution.
type windowsSetup struct {
	Ctx     context.Context
	Process process.Process
	Distro  string
	Report  report.Reporter
	// Ask prompts with question; ok is false when the CLI is not interactive.
	Ask func(question string) (answer string, ok bool)
}

// prepare returns the saved choice, or a new or moved one; it writes nothing.
func (s windowsSetup) prepare(paths managed.Paths, dry, yes, update, enable bool) (*windowsTerminalChoice, error) {
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil {
		return nil, err
	}
	if choice.foreign(s.Distro) {
		if update {
			s.Report.Report("Notes", report.Warning, "Skipping Windows Terminal setup copied from WSL distribution %q; run 'selfishell install' here to set it up.", choice.Distro)
			return &windowsTerminalChoice{Version: 1, transient: true}, nil
		}
		next, err := s.setUp(nil, dry, yes, enable)
		if next == nil {
			next = &windowsTerminalChoice{Version: 1, transient: true}
		}
		next.replacesForeign = true
		return next, err
	}
	if choice != nil && choice.Enabled && s.Distro != "" && windowsProfileMissing(choice) {
		// A re-imported distribution gets a new profile; keep the opt-in and move to it.
		target, err := s.findTarget()
		if err == nil {
			next, err := s.choiceFor(target)
			if next != nil {
				next.Retargeted = true
			}
			return next, err
		}
		if !errors.Is(err, errWindowsTerminalUnavailable) {
			s.Report.Report("Notes", report.Warning, "Skipping Windows Terminal setup: %s", err)
		}
		return choice, nil
	}
	if choice != nil && (!enable || choice.Enabled) {
		return choice, nil
	}
	if update {
		return choice, nil
	}
	return s.setUp(choice, dry, yes, enable)
}

var errWindowsTerminalUnavailable = errors.New("Windows Terminal setup requires an installed Windows Terminal and working WSL Windows interoperability")

type windowsTerminalTarget struct{ appData, settings, guid, name string }

func (s windowsSetup) findTarget() (*windowsTerminalTarget, error) {
	probe, err := runWindowsScript(s.Ctx, s.Process, map[string]string{"operation": "probe", "distro": s.Distro})
	var detected struct {
		AppData           string   `json:"appData"`
		TerminalInstalled bool     `json:"terminalInstalled"`
		SettingsPaths     []string `json:"settingsPaths"`
		WSLProfileGuids   []string `json:"wslProfileGuids"`
	}
	if err == nil {
		err = json.Unmarshal(probe, &detected)
	}
	if err != nil || !detected.TerminalInstalled {
		return nil, errWindowsTerminalUnavailable
	}
	settings, guid, name, err := s.findProfile(detected.SettingsPaths, detected.WSLProfileGuids)
	if err != nil {
		return nil, err
	}
	return &windowsTerminalTarget{detected.AppData, settings, guid, name}, nil
}

func (s windowsSetup) choiceFor(target *windowsTerminalTarget) (*windowsTerminalChoice, error) {
	path, err := convertWindowsPath(s.Ctx, s.Process, "-u", target.appData)
	if err != nil {
		return nil, err
	}
	return &windowsTerminalChoice{Version: 1, Enabled: true, Distro: s.Distro, AppData: target.appData, AppDataPath: path, SettingsPath: target.settings, ProfileGUID: target.guid}, nil
}

func (s windowsSetup) setUp(choice *windowsTerminalChoice, dry, yes, enable bool) (*windowsTerminalChoice, error) {
	if s.Distro == "" {
		if enable {
			return nil, fmt.Errorf("Windows Terminal setup requires a WSL distribution and working Windows interoperability")
		}
		return choice, nil
	}
	target, err := s.findTarget()
	if errors.Is(err, errWindowsTerminalUnavailable) {
		if enable {
			return nil, err
		}
		return choice, nil
	}
	if err != nil {
		s.Report.Report("Notes", report.Warning, "Skipping Windows Terminal setup: %s", err)
		return choice, nil
	}
	selected := yes || dry || enable
	if !selected {
		if answer, ok := s.Ask(fmt.Sprintf("Apply the Selfishell font and Dark+ to the existing Windows Terminal profile %q and install the font if missing (recommended)? [Y/n] ", target.name)); ok {
			selected = !report.Negative(answer)
		}
	}
	if !selected {
		return &windowsTerminalChoice{Version: 1}, nil
	}
	return s.choiceFor(target)
}

// saveWindowsChoice records choice unless it is a dry run or only for this run.
func saveWindowsChoice(paths managed.Paths, choice *windowsTerminalChoice, dry bool) error {
	if dry || choice == nil || choice.transient {
		return nil
	}
	// The copied record names another distribution's fragment; forget only the copy.
	if choice.replacesForeign {
		if err := os.Remove(windowsFragmentRecordPath(paths)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	data, err := json.Marshal(choice)
	if err != nil {
		return err
	}
	return managed.WriteAtomic(paths.State+"/windows-terminal.json", append(data, '\n'), 0600)
}

func runWindowsScript(ctx context.Context, p process.Process, request any) ([]byte, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	// Encoded data keeps paths and names out of PowerShell source.
	script := "$request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString(data) + "')) | ConvertFrom-Json\n" + windowsTerminalScript
	words := utf16.Encode([]rune(script))
	encoded := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(encoded[i*2:], word)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var out, stderr bytes.Buffer
	p.Out, p.Err = &out, &stderr
	code, err := p.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("Windows integration failed (exit %d): %s", code, strings.TrimSpace(stderr.String()))
	}
	return bytes.TrimSpace(out.Bytes()), nil
}

func convertWindowsPath(ctx context.Context, p process.Process, direction, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out, stderr bytes.Buffer
	p.Out, p.Err = &out, &stderr
	code, err := p.Run(ctx, "wslpath", direction, path)
	if err != nil {
		return "", err
	}
	result := strings.TrimRight(out.String(), "\r\n")
	if code != 0 || result == "" || strings.ContainsAny(result, "\x00\r\n") || (direction == "-u" && !filepath.IsAbs(result)) {
		return "", fmt.Errorf("could not resolve Windows path: %s", path)
	}
	return result, nil
}

func terminalGUID(value string) string {
	id := strings.ToLower(strings.Trim(value, "{}"))
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return ""
	}
	if _, err := hex.DecodeString(strings.ReplaceAll(id, "-", "")); err != nil {
		return ""
	}
	return "{" + id + "}"
}

func validTerminalGUID(value string) bool { return terminalGUID(value) != "" }

// Windows Terminal's legacy WSL generator uses UUID v5 of the original distro
// name in UTF-16LE. Display-name changes do not change this identifier.
func legacyWSLProfileGUID(distro string) string {
	namespace := []byte{0x2b, 0xde, 0x4a, 0x90, 0xd0, 0x5f, 0x40, 0x1c, 0x94, 0x92, 0xe4, 0x08, 0x84, 0xea, 0xd1, 0xd8}
	for _, word := range utf16.Encode([]rune(distro)) {
		namespace = binary.LittleEndian.AppendUint16(namespace, word)
	}
	sum := sha1.Sum(namespace)
	id := sum[:16]
	id[6], id[8] = (id[6]&0x0f)|0x50, (id[8]&0x3f)|0x80
	return fmt.Sprintf("{%x-%x-%x-%x-%x}", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func (j *terminalJSON) profile(guid string) (*terminalJSONNode, error) {
	if err := j.root.unique("profiles"); err != nil {
		return nil, err
	}
	profiles := j.root.property("profiles")
	if profiles != nil && profiles.object {
		if err := profiles.unique("list"); err != nil {
			return nil, err
		}
		profiles = profiles.property("list")
	}
	var found *terminalJSONNode
	if profiles != nil {
		for _, profile := range profiles.items {
			if err := profile.unique("guid"); err != nil {
				return nil, err
			}
			if terminalGUID(j.text(profile.property("guid"))) == guid {
				if found != nil {
					return nil, fmt.Errorf("duplicate Windows Terminal profile GUID: %s", guid)
				}
				found = profile
			}
		}
	}
	// Font and scheme keys only feed the override notice, which checks its own.
	if err := found.unique("source", "hidden"); err != nil {
		return nil, err
	}
	return found, nil
}

func (s windowsSetup) findProfile(windowsPaths, modernGUIDs []string) (string, string, string, error) {
	expected := map[string]string{legacyWSLProfileGUID(s.Distro): "Windows.Terminal.Wsl"}
	for _, guid := range modernGUIDs {
		if id := terminalGUID(guid); id != "" {
			expected[id] = "Microsoft.WSL"
		}
	}
	type candidate struct{ path, guid, name string }
	var matches, current []candidate
	seen := map[string]bool{}
	for _, windowsPath := range windowsPaths {
		path, err := convertWindowsPath(s.Ctx, s.Process, "-u", windowsPath)
		if err != nil {
			return "", "", "", err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		data, err := managed.ReadFile(path)
		if err != nil {
			return "", "", "", err
		}
		j, err := parseTerminalJSON(data)
		if err != nil {
			return "", "", "", err
		}
		for guid, source := range expected {
			profile, err := j.profile(guid)
			if err != nil {
				return "", "", "", err
			}
			if profile == nil || j.text(profile.property("source")) != source || string(j.value(profile.property("hidden"))) == "true" {
				continue
			}
			match := candidate{path, guid, j.text(profile.property("name"))}
			matches = append(matches, match)
			if guid == terminalGUID(s.Process.Environment().Get("WT_PROFILE_ID")) {
				current = append(current, match)
			}
		}
	}
	if len(current) == 1 {
		matches = current
	}
	if len(matches) != 1 {
		return "", "", "", fmt.Errorf("could not uniquely identify an existing profile for WSL distribution %q; found %d candidates", s.Distro, len(matches))
	}
	return matches[0].path, matches[0].guid, matches[0].name, nil
}
