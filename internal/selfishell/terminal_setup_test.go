package selfishell

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestExistingGhosttyOffersConfigurationAndPreservesExternalApp(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	ghostty := filepath.Join(home, "tools/ghostty")
	if err := testutil.WriteFile(ghostty, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := blockRun(t, root, "y\ny\n", "install", "--skip-packages")
	if code != 0 || !strings.Contains(out, "Use Selfishell configuration for your existing Ghostty terminal") {
		t.Fatalf("%d %s %s", code, out, stderr)
	}
	if _, err := os.Stat(paths.Resources + "/user-ghostty.state"); err != nil {
		t.Fatal(err)
	}
	f := newPackageFixture(t)
	f.executable("ghostty", "exit 0")
	f.executable("brew", "echo attempted >\"$HOME/brew-attempted\"; exit 90")
	c := CLI{Root: root, Out: &f.out, Err: &f.err}
	if err := c.installPackages(context.Background(), f.op, paths, nil, "macos", "arm64", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.home + "/brew-attempted"); !os.IsNotExist(err) {
		t.Fatalf("tried installing an existing Ghostty: %v", err)
	}
}

func windowsTerminalFixture(t *testing.T) (string, string, Paths, string) {
	t.Helper()
	root, home, paths := blockHome(t, "ubuntu")
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu-24.04")
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	if err := testutil.WriteFile(os.Getenv("SELFISHELL_TEST_PROC_VERSION_FILE"), []byte("Linux microsoft WSL2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	windowsHome := home + "/windows-localappdata"
	probe := `#!/bin/sh
printf '%s\n' '{"appData":"C:\\Users\\Fixture\\AppData\\Local","terminalInstalled":true,"fontInstalled":false}'
`
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(probe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(home+"/tools/wslpath", []byte("#!/bin/sh\nprintf '%s\\n' \"$HOME/windows-localappdata\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return root, home, paths, windowsHome
}

func TestWindowsTerminalChoiceLifecycle(t *testing.T) {
	root, _, paths, windowsHome := windowsTerminalFixture(t)
	code, out, stderr := blockRun(t, root, "y\nn\n", "install", "--skip-packages")
	if code != 0 || !strings.Contains(out, "Windows Terminal") {
		t.Fatalf("choice: %d %s %s", code, out, stderr)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if _, err := os.Stat(windowsHome); !os.IsNotExist(err) {
		t.Fatalf("declined choice was not retained: %v", err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal")
	state, err := ReadState(paths.Resources + "/windows-terminal.state")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(state.Target, windowsHome+"/Microsoft/Windows Terminal/Fragments/Selfishell/") {
		t.Fatalf("wrong fragment path: %s", state.Target)
	}
	before := blockRead(t, state.Target)
	var fragment struct {
		Profiles []struct {
			Name, Commandline string
			Font              struct{ Face string }
		}
	}
	if err := json.Unmarshal(before, &fragment); err != nil {
		t.Fatal(err)
	}
	if len(fragment.Profiles) != 1 || fragment.Profiles[0].Name != "Selfishell – Ubuntu-24.04" || fragment.Profiles[0].Font.Face != "JetBrainsMonoNL Nerd Font Mono" || !strings.Contains(fragment.Profiles[0].Commandline, "Ubuntu-24.04") {
		t.Fatalf("wrong fragment: %s", before)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	if string(blockRead(t, state.Target)) != string(before) {
		t.Fatal("reinstall/update changed the profile")
	}
	if err := testutil.AppendFile(state.Target, []byte("\n ")); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = blockRun(t, root, "", "update", "--tools-only", "--yes")
	if code == 0 || !strings.Contains(stderr, "modified") {
		t.Fatalf("modified profile update: %d %s", code, stderr)
	}
	code, _, _ = blockRun(t, root, "", "uninstall", "--yes")
	if code == 0 {
		t.Fatal("uninstall accepted a changed fragment")
	}
	if _, err := os.Stat(os.Getenv("HOME") + "/.zshrc"); err != nil {
		t.Fatal("uninstall removed resources before preflight")
	}
	if err := testutil.WriteFile(state.Target, before, 0644); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "uninstall", "--restore", "--yes")
	if _, err := os.Stat(state.Target); !os.IsNotExist(err) {
		t.Fatalf("fragment left after uninstall: %v", err)
	}
	if _, err := os.Stat(paths.State + "/windows-terminal.json"); !os.IsNotExist(err) {
		t.Fatalf("saved choice left after uninstall: %v", err)
	}
}

func TestWindowsTerminalDryRunAndUnavailableInterop(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	out := blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal", "--dry-run")
	if !strings.Contains(out, "Windows Terminal") {
		t.Fatalf("missing preview: %s", out)
	}
	for _, path := range []string{paths.Config, paths.State, windowsHome} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("dry run created %s: %v", path, err)
		}
	}
	if err := os.Remove(home + "/tools/powershell.exe"); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes", "--windows-terminal")
	if code == 0 || !strings.Contains(stderr, "Windows") {
		t.Fatalf("missing interop: %d %s", code, stderr)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
}

func TestWindowsFontPinnedInstallRecoveryAndExternalPreservation(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal")
	if err := testutil.WriteFile(home+"/tools/curl", []byte("#!/bin/sh\nexec /usr/bin/curl \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	source := home + "/font-source"
	payload := []byte("pinned-font-fixture")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	if err := testutil.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := home + "/font-manifest"
	record := fmt.Sprintf("download jetbrainsmono-regular 3.4.0 linux all file://%s %s .local/share/selfishell/fonts/Regular.ttf font\n", source, sum)
	if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	target := windowsHome + "/Microsoft/Windows/Fonts/Selfishell/3.4.0/Regular.ttf"
	op := &PackageOperation{Process: Process{Out: new(strings.Builder), Err: new(strings.Builder)}}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, target)) != string(payload) {
		t.Fatal("font not installed on Windows")
	}
	if err := testutil.WriteFile(target, []byte("changed-managed-font"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, target)) != string(payload) {
		t.Fatal("font pin not restored")
	}
	// An interrupted first activation can be recovered only while its journal matches.
	if err := os.Remove(paths.State + "/dependencies/jetbrainsmono-regular"); err != nil {
		t.Fatal(err)
	}
	pending, _ := json.Marshal(map[string]string{"target": target, "version": "3.4.0", "checksum": sum})
	if err := testutil.WriteFile(paths.State+"/pending-fonts/jetbrainsmono-regular", pending, 0600); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.State + "/dependencies/jetbrainsmono-regular"); err != nil {
		t.Fatal("ownership not recovered", err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal("unchanged font downloaded", err)
	}
	// External font family is preserved without downloads or ownership records.
	if err := os.Remove(paths.State + "/dependencies/jetbrainsmono-regular"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	probe := `#!/bin/sh
printf '%s\n' '{"fontInstalled":true}'
`
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(probe), 0700); err != nil {
		t.Fatal(err)
	}
	op = &PackageOperation{Process: Process{Out: new(strings.Builder), Err: new(strings.Builder)}}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("external family overwritten")
	}
}

func TestWindowsTerminalFragmentBackupAndPlatformGuard(t *testing.T) {
	root, _, paths, _ := windowsTerminalFixture(t)
	choice, err := (CLI{Root: root, Out: io.Discard, Err: io.Discard}).prepareWindowsTerminal(paths, false, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := choice.resource()
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"profiles":[],"personal":true}`)
	if err := os.MkdirAll(rawParent(resource.Target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(resource.Target, original, 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--windows-terminal", "--skip-packages", "--yes")
	first, err := ReadState(paths.Resources + "/windows-terminal.state")
	if err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	second, err := ReadState(paths.Resources + "/windows-terminal.state")
	if err != nil || first.Backup != second.Backup {
		t.Fatal("backup changed", err)
	}
	if string(blockRead(t, first.Backup)) != string(original) {
		t.Fatal("backup overwritten")
	}
	// Uninstall must inspect Windows records even after platform detection changes.
	if err := testutil.WriteFile(os.Getenv("SELFISHELL_TEST_PROC_VERSION_FILE"), []byte("Linux"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := blockRun(t, root, "", "install", "--windows-terminal", "--skip-packages", "--yes")
	if code != 2 || !strings.Contains(stderr, "only on Ubuntu on WSL") {
		t.Fatalf("%d %s", code, stderr)
	}
	blockOK(t, root, "uninstall", "--restore", "--yes")
	if string(blockRead(t, resource.Target)) != string(original) {
		t.Fatal("original profile not restored")
	}
}

func TestWindowsFontChecksumFailureAndOptionalInteropFailure(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal")
	if err := testutil.WriteFile(home+"/tools/curl", []byte("#!/bin/sh\nexec /usr/bin/curl \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	source, manifest := home+"/bad-font", home+"/manifest"
	if err := testutil.WriteFile(source, []byte("wrong"), 0600); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf("download jetbrainsmono-regular 3.4.0 linux all file://%s %064d .local/share/selfishell/fonts/Regular.ttf font\n", source, 0)
	if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	op := &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
	err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false)
	if err == nil || !strings.Contains(err.Error(), "Checksum mismatch") {
		t.Fatal("bad checksum accepted", err)
	}
	if _, err := os.Stat(windowsHome + "/Microsoft/Windows/Fonts/Selfishell/3.4.0/Regular.ttf"); !os.IsNotExist(err) {
		t.Fatal("bad font activated", err)
	}
	if err := os.Remove(home + "/tools/powershell.exe"); err != nil {
		t.Fatal(err)
	}
	op = &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
	if err := op.InstallDirect(context.Background(), paths, manifest, "optional", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if len(op.SkippedOptional) != 1 {
		t.Fatal("optional failure unreported")
	}
}

// Opt-in native probes never write Windows settings or font registrations.
func TestWindowsInteropReadOnly(t *testing.T) {
	if os.Getenv("SELFISHELL_TEST_WSL_INTEROP") != "1" {
		t.Skip("requires native WSL interoperability")
	}
	t.Parallel()
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Fatal(err)
	}
	p := Process{Env: withEnvironment(Process{}, map[string]string{"HOME": t.TempDir()}).Env}
	data, err := p.windowsScript(context.Background(), map[string]string{"operation": "probe"})
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		AppData string `json:"appData"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.AppData == "" {
		t.Fatal("invalid native probe", string(data), err)
	}
	linux, err := p.windowsPath(context.Background(), "-u", probe.AppData)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := p.windowsPath(context.Background(), "-w", linux)
	if err != nil || !strings.EqualFold(windows, probe.AppData) {
		t.Fatal("path round trip", windows, err)
	}
}

func TestWindowsFragmentNativeFilesystem(t *testing.T) {
	base := os.Getenv("SELFISHELL_TEST_WINDOWS_TEMP")
	if base == "" {
		t.Skip("requires a private scratch directory on Windows filesystem")
	}
	t.Parallel()
	scratch, err := os.MkdirTemp(base, "selfishell-terminal-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(scratch) })
	home := t.TempDir()
	paths := Paths{State: home + "/state", Resources: home + "/state/resources"}
	m := managed{paths: paths, c: CLI{Out: io.Discard, Err: io.Discard}, yes: true, actions: map[string]string{}}
	w := windowsTerminalChoice{Version: 1, Enabled: true, Distro: "Ubuntu", Home: home, AppDataPath: scratch, AppData: "C:\\fixture"}
	r, err := w.resource()
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"profiles":[]}`)
	if err := os.MkdirAll(rawParent(r.Target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(r.Target, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.installResource(r, true); err != nil {
		t.Fatal(err)
	}
	if err := m.installResource(r, false); err != nil {
		t.Fatal(err)
	}
	state, err := ReadState(paths.Resources + "/windows-terminal.state")
	if err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, state.Backup)) != string(original) {
		t.Fatal("native backup differs")
	}
	if err := m.preflightUninstall(ResourceState{r, state}, true); err != nil {
		t.Fatal(err)
	}
	if err := m.removeResource(ResourceState{r, state}, true); err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, r.Target)) != string(original) {
		t.Fatal("native restoration differs")
	}
}

func TestWindowsFontStatusRequiresRegistration(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	payload := []byte("approved-font")
	manifest := home + "/font-manifest"
	record := fmt.Sprintf("download jetbrainsmono-regular 3.4.0 linux all file://unused %x .local/share/selfishell/fonts/Regular.ttf font\n", sha256.Sum256(payload))
	if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_DEPENDENCIES_FILE", manifest)
	target := windowsHome + "/Microsoft/Windows/Fonts/Selfishell/3.4.0/Regular.ttf"
	if err := os.MkdirAll(rawParent(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(target, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.State+"/dependencies", 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(paths.State+"/dependencies/jetbrainsmono-regular", []byte("3.4.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, err := NewToolInventory(root, paths, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	result, err := inventory.Detect("direct", "jetbrainsmono-regular", "linux", "amd64")
	if err != nil || result.Installed != "missing" {
		t.Fatal("unregistered file reported installed", result, err)
	}
	response, _ := json.Marshal(map[string]any{"fontInstalled": true, "registrations": map[string]string{"Selfishell jetbrainsmono-regular (TrueType)": `C:\Users\Fixture\AppData\Local\Microsoft\Windows\Fonts\Selfishell\3.4.0\Regular.ttf`}})
	script := "#!/bin/sh\nprintf '%s\\n' '" + string(response) + "'\n"
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	inventory, err = NewToolInventory(root, paths, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	result, err = inventory.Detect("direct", "jetbrainsmono-regular", "linux", "amd64")
	if err != nil || result.Installed != "3.4.0" {
		t.Fatal("registered font missing", result, err)
	}
	// CLI-only pin changes must still report the recorded installed version.
	nextRecord := fmt.Sprintf("download jetbrainsmono-regular 3.4.1 linux all file://unused %064d .local/share/selfishell/fonts/Regular.ttf font\n", 0)
	if err := testutil.WriteFile(manifest, []byte(nextRecord), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, err = NewToolInventory(root, paths, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	result, err = inventory.Detect("direct", "jetbrainsmono-regular", "linux", "amd64")
	if err != nil || result.Installed != "3.4.0" || result.Approved != "3.4.1" {
		t.Fatal("CLI-only update forgot recorded font", result, err)
	}

}

func TestWindowsTerminalAccountAndDarkPlus(t *testing.T) {
	t.Parallel()
	w := windowsTerminalChoice{Version: 1, Enabled: true, Distro: "Ubuntu", Home: "/home/other", AppData: "C:/fixture", AppDataPath: "/windows"}
	data, _ := json.Marshal(w)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	fields["user"] = "other-user"
	data, _ = json.Marshal(fields)
	_ = json.Unmarshal(data, &w)
	r, err := w.resource()
	if err != nil {
		t.Fatal(err)
	}
	var fragment struct {
		Profiles []struct{ Commandline, ColorScheme string }
		Schemes  []map[string]string
	}
	if err := json.Unmarshal([]byte(r.Source), &fragment); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fragment.Profiles[0].Commandline, `--user "other-user"`) {
		t.Fatal("profile launches default Linux user", fragment.Profiles[0].Commandline)
	}
	if fragment.Profiles[0].ColorScheme != "Selfishell Dark+" || len(fragment.Schemes) != 1 || fragment.Schemes[0]["background"] != "#1e1e1e" || fragment.Schemes[0]["name"] != "Selfishell Dark+" {
		t.Fatal("missing Dark+ scheme", r.Source)
	}
}

func TestWindowsFontUpgradeUsesNewPathAndOwnedRegistration(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	if err := testutil.WriteFile(home+"/tools/curl", []byte("#!/bin/sh\nexec /usr/bin/curl \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	source, manifest := home+"/font-source", home+"/manifest"
	payload := []byte("old-font")
	if err := testutil.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	writeManifest := func(version string, payload []byte) {
		t.Helper()
		record := fmt.Sprintf("download jetbrainsmono-regular %s linux all file://%s %x .local/share/selfishell/fonts/Regular.ttf font\n", version, source, sha256.Sum256(payload))
		if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest("3.4.0", payload)
	op := &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	deps, _ := ReadDependencies(manifest)
	oldTarget, err := dependencyTarget(deps[0], paths)
	if err != nil {
		t.Fatal(err)
	}
	// Capture the registration request, including its ownership-approved prior path.
	script := `#!/usr/bin/python3
import sys,base64,json,re,os
script=base64.b64decode(sys.argv[-1]).decode('utf-16le')
data=re.search("FromBase64String\('([^']+)'",script).group(1)
request=json.loads(base64.b64decode(data))
if request['operation']=='font-register':
 with open(os.environ['HOME']+'/registration-request','w') as f: json.dump(request,f)
print('{}')
`
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(home+"/tools/wslpath", []byte("#!/bin/sh\nprintf '%s\n' \"$2\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	payload = []byte("new-font")
	if err := testutil.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	writeManifest("3.4.1", payload)
	op = &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	deps, _ = ReadDependencies(manifest)
	newTarget, err := dependencyTarget(deps[0], paths)
	if err != nil {
		t.Fatal(err)
	}
	if oldTarget == newTarget {
		t.Fatal("upgrade replaced a potentially loaded font", oldTarget)
	}
	if string(blockRead(t, oldTarget)) != "old-font" || string(blockRead(t, newTarget)) != "new-font" {
		t.Fatal("versioned font payloads not retained")
	}
	var request map[string]string
	if err := json.Unmarshal(blockRead(t, home+"/registration-request"), &request); err != nil {
		t.Fatal(err)
	}
	if request["previousPath"] != oldTarget || request["path"] != newTarget || !strings.HasPrefix(newTarget, windowsHome) {
		t.Fatal("registration cannot verify prior ownership", request)
	}
}

func TestWindowsTerminalChoiceWithoutUserMigrates(t *testing.T) {
	root, home, paths, _ := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	data := blockRead(t, paths.State+"/windows-terminal.json")
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	originalUser := fields["user"]
	delete(fields, "user")
	data, _ = json.Marshal(fields)
	if err := testutil.WriteFile(paths.State+"/windows-terminal.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(home + "/tools/powershell.exe"); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	choice, err := readWindowsTerminalChoice(paths)
	if err != nil || choice.User != originalUser {
		t.Fatal("old choice failed to migrate", choice, err)
	}
}

// Load a font privately in a short-lived Windows process, then sync a new pin.
// No font is exposed to other applications and no real registry entry is written.
func TestWindowsFontLoadedNativeUpgrade(t *testing.T) {
	base := os.Getenv("SELFISHELL_TEST_WINDOWS_TEMP")
	if base == "" || os.Getenv("SELFISHELL_TEST_WSL_FONTS") != "1" {
		t.Skip("requires opt-in native Windows private-font probe")
	}
	t.Parallel()
	home := t.TempDir()
	scratch, err := os.MkdirTemp(base, "selfishell-font-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(scratch) })
	paths := Paths{State: home + "/state", Data: home + "/data/selfishell"}
	if err := os.MkdirAll(paths.State+"/dependencies", 0700); err != nil {
		t.Fatal(err)
	}
	choice := windowsTerminalChoice{Version: 1, Enabled: true, User: "fixture", Distro: "Ubuntu", Home: home, AppData: "C:/fixture", AppDataPath: scratch}
	data, _ := json.Marshal(choice)
	if err := testutil.WriteFile(paths.State+"/windows-terminal.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	deps, err := ReadDependencies("../../dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	var dep Dependency
	for _, d := range deps {
		if d.Name == "jetbrainsmono-regular" {
			dep = d
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", dep.Source, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || fmt.Sprintf("%x", sha256.Sum256(payload)) != dep.Checksum {
		t.Fatal("font download verification", err, response.StatusCode)
	}
	oldTarget, err := dependencyTarget(dep, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rawParent(oldTarget), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(oldTarget, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(paths.State+"/dependencies/"+dep.Name, []byte(dep.Version+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	windowsPath, err := (Process{}).windowsPath(ctx, "-w", oldTarget)
	if err != nil {
		t.Fatal(err)
	}
	encodedPath := base64.StdEncoding.EncodeToString([]byte(windowsPath))
	script := `$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$path=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encodedPath + `'))
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class NativePrivateFont {
[DllImport("gdi32.dll",CharSet=CharSet.Unicode)] public static extern int AddFontResourceEx(string path,uint flags,IntPtr reserved);
[DllImport("gdi32.dll",CharSet=CharSet.Unicode)] public static extern bool RemoveFontResourceEx(string path,uint flags,IntPtr reserved);
}
'@
if ([NativePrivateFont]::AddFontResourceEx($path,16,[IntPtr]::Zero) -eq 0) { throw 'Private font load failed' }
# Private GDI resources do not always hold an exclusive file lock. Also keep
# a reader open to exercise the sharing restriction of active Windows clients.
$handle=[IO.File]::Open($path,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
try { [Console]::WriteLine('loaded'); [void][Console]::ReadLine() }
finally { $handle.Dispose(); [void][NativePrivateFont]::RemoveFontResourceEx($path,16,[IntPtr]::Zero) }
`
	words := utf16.Encode([]rune(script))
	raw := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(raw[i*2:], word)
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.Env = withEnvironment(Process{}, map[string]string{"HOME": home}).Env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close(); cmd.Wait() })
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || strings.TrimSpace(line) != "loaded" {
		t.Fatal("private font load", line, err)
	}
	if err := os.Rename(oldTarget, oldTarget+".moved"); err == nil {
		t.Fatal("Windows did not protect the loaded font")
	}
	source := home + "/font-source"
	if err := testutil.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	dep.Version = "native-upgrade"
	dep.Source = "file://" + source
	manifest := home + "/manifest"
	record := strings.Join([]string{dep.Kind, dep.Name, dep.Version, dep.Platform, dep.Arch, dep.Source, dep.Checksum, dep.Target, dep.Marker}, " ") + "\n"
	if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	tools := home + "/tools"
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	// Stub only persistent registration; path conversion and font file activation are native.
	if err := testutil.WriteFile(tools+"/powershell.exe", []byte("#!/bin/sh\nprintf '{}\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	op := &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard, Env: withEnvironment(Process{}, map[string]string{"HOME": home, "PATH": tools + ":" + os.Getenv("PATH")}).Env}}
	if err := op.InstallDirect(ctx, paths, manifest, "required", dep.Name, "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal(err)
	}
	newTarget, err := dependencyTarget(dep, paths)
	if err != nil {
		t.Fatal(err)
	}
	if newTarget == oldTarget || string(blockRead(t, oldTarget)) != string(payload) || string(blockRead(t, newTarget)) != string(payload) {
		t.Fatal("loaded-font upgrade changed old payload")
	}
}

func TestUpdateHomeCannotReachInheritedWindowsInterop(t *testing.T) {
	outer := t.TempDir()
	t.Setenv("WSL_DISTRO_NAME", "Inherited-Real-Distro")
	home := isolatedUpdateHome(t)
	tools := outer + "/tools"
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	probe := "#!/bin/sh\nprintf touched > " + outer + "/host-touched\nprintf '%s\\n' '{\"appData\":\"C:/host\",\"terminalInstalled\":true}'\n"
	if err := testutil.WriteFile(tools+"/powershell.exe", []byte(probe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(tools+"/wslpath", []byte("#!/bin/sh\nprintf '%s\\n' '"+outer+"/host-fragments'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	for name, data := range map[string]string{"os-release": "ID=ubuntu\n", "proc-version": "Microsoft WSL2\n"} {
		if err := testutil.WriteFile(home+"/"+name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", home+"/os-release")
	t.Setenv("SELFISHELL_TEST_PROC_VERSION_FILE", home+"/proc-version")
	code, _, stderr := commandResult(testRelease(t), "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal("isolated setup failed", stderr)
	}
	for _, path := range []string{outer + "/host-touched", outer + "/host-fragments"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("isolated HOME reached inherited Windows environment: %s", path)
		}
	}
}
