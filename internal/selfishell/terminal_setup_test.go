package selfishell

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	target := windowsHome + "/Microsoft/Windows/Fonts/Selfishell/Regular.ttf"
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
	if _, err := os.Stat(windowsHome + "/Microsoft/Windows/Fonts/Selfishell/Regular.ttf"); !os.IsNotExist(err) {
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
	target := windowsHome + "/Microsoft/Windows/Fonts/Selfishell/Regular.ttf"
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
	response, _ := json.Marshal(map[string]any{"fontInstalled": true, "registrations": map[string]string{"Selfishell jetbrainsmono-regular (TrueType)": `C:\Users\Fixture\AppData\Local\Microsoft\Windows\Fonts\Selfishell\Regular.ttf`}})
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
}
