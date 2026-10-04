package selfishell

import (
	"bytes"
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

//go:embed windows_terminal.ps1
var windowsTerminalScript string

// A separate versioned choice, not the fixed-line resource-state format.
type windowsTerminalChoice struct {
	Version     int    `json:"version"`
	User        string `json:"user,omitempty"`
	Enabled     bool   `json:"enabled"`
	Distro      string `json:"distro,omitempty"`
	Home        string `json:"home,omitempty"`
	AppData     string `json:"appData,omitempty"`
	AppDataPath string `json:"appDataPath,omitempty"`
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
		if choice.User == "" {
			account, err := user.LookupId(strconv.Itoa(os.Getuid()))
			if err != nil {
				return nil, err
			}
			choice.User = account.Username
		}
		for _, value := range []string{choice.User, choice.Distro, choice.Home, choice.AppData, choice.AppDataPath} {
			if value == "" || strings.ContainsAny(value, "\x00\r\n") {
				return nil, fmt.Errorf("invalid Windows Terminal choice")
			}
		}
		if !filepath.IsAbs(choice.Home) || !filepath.IsAbs(choice.AppDataPath) {
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
	p := Process{Out: c.Out, Err: c.Err}
	distro := os.Getenv("WSL_DISTRO_NAME")
	probe, err := p.windowsScript(c.invocationContext(), map[string]string{"operation": "probe"})
	var detected struct {
		AppData           string `json:"appData"`
		TerminalInstalled bool   `json:"terminalInstalled"`
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
	selected := yes || dry || enable
	if !selected && c.interactive() {
		fmt.Fprint(c.Out, "Add a Selfishell Windows Terminal profile and install its font if missing (recommended)? [Y/n] ")
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
	account, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil {
		return nil, err
	}
	choice.User = account.Username
	choice.Distro, choice.Home, choice.AppData, choice.AppDataPath = distro, os.Getenv("HOME"), detected.AppData, path
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

// quoteWindowsArgument follows CommandLineToArgvW's backslash/quote rules.
func quoteWindowsArgument(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	backslashes := 0
	for _, ch := range value {
		if ch == '\\' {
			backslashes++
			continue
		}
		if ch == '"' {
			out.WriteString(strings.Repeat("\\", backslashes*2+1))
		} else {
			out.WriteString(strings.Repeat("\\", backslashes))
		}
		backslashes = 0
		out.WriteRune(ch)
	}
	out.WriteString(strings.Repeat("\\", backslashes*2))
	out.WriteByte('"')
	return out.String()
}

func (w windowsTerminalChoice) resource() (Resource, error) {
	// Stable app-specific UUID v5; reinstalling cannot duplicate the profile.
	namespace := []byte{0x91, 0x54, 0x83, 0x49, 0x43, 0xaa, 0x4f, 0xe9, 0xb6, 0x63, 0xee, 0x70, 0xa9, 0x8e, 0x32, 0x40}
	sum := sha1.Sum(append(namespace, []byte(w.Distro+"\x00"+w.Home)...))
	id := sum[:16]
	id[6], id[8] = (id[6]&0x0f)|0x50, (id[8]&0x3f)|0x80
	guid := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	profile := map[string]any{
		"guid": "{" + guid + "}", "name": "Selfishell – " + w.Distro,
		"commandline": "wsl.exe --distribution " + quoteWindowsArgument(w.Distro) + " --user " + quoteWindowsArgument(w.User) + " --cd " + quoteWindowsArgument(w.Home) + " --exec zsh --login",
		"font":        map[string]string{"face": terminalFont},
		"colorScheme": "Selfishell Dark+",
	}
	data, err := json.MarshalIndent(map[string]any{"profiles": []any{profile}, "schemes": []any{windowsTerminalDarkPlus}}, "", "  ")
	return Resource{Kind: "file", Name: "windows-terminal", Target: w.AppDataPath + "/Microsoft/Windows Terminal/Fragments/Selfishell/" + guid + ".json", Source: string(append(data, '\n'))}, err
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
