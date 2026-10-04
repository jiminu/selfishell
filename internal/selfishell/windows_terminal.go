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
	"syscall"
	"time"
	"unicode/utf16"
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
}

func readWindowsTerminalChoice(paths Paths) (*windowsTerminalChoice, error) {
	data, err := readStateFile(paths.State + "/windows-terminal.json")
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

func (c CLI) prepareWindowsTerminal(paths Paths, dry, yes, update, enable bool) (*windowsTerminalChoice, error) {
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil {
		return nil, err
	}
	if choice != nil && (!enable || choice.Enabled) {
		return choice, nil
	}
	if update {
		return choice, nil
	}
	distro := os.Getenv("WSL_DISTRO_NAME")
	if distro == "" {
		if enable {
			return nil, fmt.Errorf("Windows Terminal setup requires a WSL distribution and working Windows interoperability")
		}
		return choice, nil
	}
	p := Process{Out: c.Out, Err: c.Err}
	probe, err := p.windowsScript(c.invocationContext(), map[string]string{"operation": "probe", "distro": distro})
	var detected struct {
		AppData           string   `json:"appData"`
		TerminalInstalled bool     `json:"terminalInstalled"`
		SettingsPaths     []string `json:"settingsPaths"`
		WSLProfileGuids   []string `json:"wslProfileGuids"`
	}
	if err == nil {
		err = json.Unmarshal(probe, &detected)
	}
	if err != nil || distro == "" || !detected.TerminalInstalled {
		if enable {
			return nil, fmt.Errorf("Windows Terminal setup requires an installed Windows Terminal and working WSL Windows interoperability")
		}
		return choice, nil
	}
	settings, guid, name, err := c.findWindowsProfile(p, distro, detected.SettingsPaths, detected.WSLProfileGuids)
	if err != nil {
		c.report("Notes", reportWarning, "Skipping Windows Terminal setup: %s", err)
		return choice, nil
	}
	selected := yes || dry || enable
	if !selected && c.interactive() {
		fmt.Fprintf(c.Out, "Apply the Selfishell font and Dark+ to the existing Windows Terminal profile %q and install the font if missing (recommended)? [Y/n] ", name)
		answer, _ := c.readAnswer()
		selected = !negative(answer)
	}
	choice = &windowsTerminalChoice{Version: 1, Enabled: selected}
	if !selected {
		return choice, nil
	}
	path, err := p.windowsPath(c.invocationContext(), "-u", detected.AppData)
	if err != nil {
		return nil, err
	}
	choice.Distro, choice.AppData, choice.AppDataPath = distro, detected.AppData, path
	choice.SettingsPath, choice.ProfileGUID = settings, guid
	return choice, nil
}

func (c CLI) addWindowsTerminal(p *preparedConfig, dry, yes, update, enable bool) error {
	choice, err := c.prepareWindowsTerminal(p.paths, dry, yes, update, enable)
	if err != nil {
		return err
	}
	p.windowsTerminal = choice
	if choice == nil || !choice.Enabled {
		return nil
	}
	if err := p.m.installWindowsProfile(choice, true); err != nil {
		return err
	}
	r, err := choice.resource()
	if err != nil {
		return err
	}
	if err := p.m.installResource(r, true); err != nil {
		return err
	}
	for i, existing := range p.resources {
		if existing.Name == r.Name {
			p.resources[i] = r
			return nil
		}
	}
	p.resources = append(p.resources, r)
	return nil
}

func (c CLI) saveWindowsTerminalChoice(p preparedConfig) error {
	if p.m.dry || p.windowsTerminal == nil {
		return nil
	}
	data, err := json.Marshal(p.windowsTerminal)
	if err != nil {
		return err
	}
	return writeAtomic(p.paths.State+"/windows-terminal.json", append(data, '\n'), 0600)
}

func (p Process) windowsScript(ctx context.Context, request any) ([]byte, error) {
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

func (p Process) windowsPath(ctx context.Context, direction, path string) (string, error) {
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

func (w windowsTerminalChoice) resource() (Resource, error) {
	guid := strings.ToLower(strings.Trim(w.ProfileGUID, "{}"))
	data, err := json.MarshalIndent(map[string]any{"schemes": []any{windowsTerminalDarkPlus}}, "", "  ")
	return Resource{Kind: "file", Name: "windows-terminal", Target: w.AppDataPath + "/Microsoft/Windows Terminal/Fragments/Selfishell/" + guid + ".json", Source: string(append(data, '\n'))}, err
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
	profiles := j.root.property("profiles")
	if profiles != nil && profiles.object {
		profiles = profiles.property("list")
	}
	var found *terminalJSONNode
	if profiles != nil {
		for _, profile := range profiles.items {
			if terminalGUID(j.text(profile.property("guid"))) == guid {
				if found != nil {
					return nil, fmt.Errorf("duplicate Windows Terminal profile GUID: %s", guid)
				}
				found = profile
			}
		}
	}
	return found, nil
}

func (c CLI) findWindowsProfile(p Process, distro string, windowsPaths, modernGUIDs []string) (string, string, string, error) {
	expected := map[string]string{legacyWSLProfileGUID(distro): "Windows.Terminal.Wsl"}
	for _, guid := range modernGUIDs {
		if id := terminalGUID(guid); id != "" {
			expected[id] = "Microsoft.WSL"
		}
	}
	type candidate struct{ path, guid, name string }
	var matches, current []candidate
	seen := map[string]bool{}
	for _, windowsPath := range windowsPaths {
		path, err := p.windowsPath(c.invocationContext(), "-u", windowsPath)
		if err != nil {
			return "", "", "", err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		data, err := readStateFile(path)
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
			if guid == terminalGUID(os.Getenv("WT_PROFILE_ID")) {
				current = append(current, match)
			}
		}
	}
	if len(current) == 1 {
		matches = current
	}
	if len(matches) != 1 {
		return "", "", "", fmt.Errorf("could not uniquely identify an existing profile for WSL distribution %q; found %d candidates", distro, len(matches))
	}
	return matches[0].path, matches[0].guid, matches[0].name, nil
}

func resourceFileContent(r Resource) ([]byte, error) {
	if r.Name == "windows-terminal" {
		return []byte(r.Source), nil
	}
	return os.ReadFile(r.Source)
}

// DrvFs rejects RENAME_NOREPLACE. Windows File/Directory.Move retain the same
// atomic no-overwrite contract when the Linux operation is unsupported.
func (m *managed) moveBackup(r Resource, source, destination string) error {
	err := moveBackupNoReplace(source, destination)
	if r.Name != "windows-terminal" || (!errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.ENOSYS)) {
		return err
	}
	p := Process{Env: withEnvironment(Process{}, map[string]string{"HOME": rawParent(m.paths.State)}).Env}
	ctx := m.c.invocationContext()
	from, err := p.windowsPath(ctx, "-w", source)
	if err != nil {
		return err
	}
	to, err := p.windowsPath(ctx, "-w", destination)
	if err != nil {
		return err
	}
	_, err = p.windowsScript(ctx, map[string]string{"operation": "file-move", "source": from, "destination": to})
	return err
}

// Dark+ from the same upstream palette Ghostty ships:
// https://github.com/mbadolato/iTerm2-Color-Schemes/blob/master/ghostty/Dark%2B
var windowsTerminalDarkPlus = map[string]string{
	"name": "Selfishell Dark+", "background": "#1e1e1e", "foreground": "#cccccc",
	"cursorColor": "#ffffff", "selectionBackground": "#3a3d41",
	"black": "#000000", "red": "#cd3131", "green": "#0dbc79", "yellow": "#e5e510",
	"blue": "#2472c8", "purple": "#bc3fbc", "cyan": "#11a8cd", "white": "#e5e5e5",
	"brightBlack": "#666666", "brightRed": "#f14c4c", "brightGreen": "#23d18b", "brightYellow": "#f5f543",
	"brightBlue": "#3b8eea", "brightPurple": "#d670d6", "brightCyan": "#29b8db", "brightWhite": "#e5e5e5",
}
