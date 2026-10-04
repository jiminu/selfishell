package selfishell

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestGhosttyChoicePreflightBeforePackages(t *testing.T) {
	if root := os.Getenv("SELFISHELL_TEST_GHOSTTY_CHOICE_ROOT"); root != "" {
		os.Exit((CLI{Root: root, Out: os.Stdout, Err: os.Stderr}).Run(strings.Fields(os.Getenv("SELFISHELL_TEST_GHOSTTY_CHOICE_ARGS"))))
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"install", "install --ghostty", "install --skip-packages --ghostty", "update"} {
		for _, kind := range []string{"directory", "symlink", "dangling", "fifo"} {
			t.Run(command+"/"+kind, func(t *testing.T) {
				root, paths := compactDiagnosticFixture(t, "macos", false)
				home := os.Getenv("HOME")
				choice := paths.State + "/ghostty"
				if err := os.Remove(choice); err != nil {
					t.Fatal(err)
				}
				target := home + "/external-choice"
				if err := testutil.WriteFile(target, []byte("0\n"), 0600); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "directory":
					err = os.Mkdir(choice, 0700)
				case "symlink":
					err = os.Symlink(target, choice)
				case "dangling":
					err = os.Symlink(home+"/absent-choice", choice)
				case "fifo":
					err = syscall.Mkfifo(choice, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				before, err := os.Lstat(choice)
				if err != nil {
					t.Fatal(err)
				}
				original := map[string][]byte{}
				for _, path := range []string{target, home + "/.zshrc", paths.Config + "/zsh/common.zsh", paths.Resources + "/zsh-common.state", paths.State + "/configured"} {
					original[path] = blockRead(t, path)
				}
				if err := testutil.WriteFile(root+"/packages.conf", []byte("package macos required formula fixture-package\n"), 0600); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"brew", "mise", "curl", "xcode-select"} {
					if err := testutil.WriteFile(home+"/tools/"+name, []byte("#!/bin/sh\nprintf called >\"$HOME/package-called\"\nexit 99\n"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				args := command + " --yes"
				if command == "update" {
					args += " --tools-only"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, executable, "-test.run=^TestGhosttyChoicePreflightBeforePackages$")
				cmd.Env = append(os.Environ(), "SELFISHELL_TEST_GHOSTTY_CHOICE_ROOT="+root, "SELFISHELL_TEST_GHOSTTY_CHOICE_ARGS="+args)
				output, err := cmd.CombinedOutput()
				if ctx.Err() != nil {
					t.Fatalf("reading %s choice exceeded the deadline: %v", kind, ctx.Err())
				}
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(output), choice) || !strings.Contains(string(output), "not a regular file") {
					t.Errorf("unsafe choice was not rejected: %v %s", err, output)
				}
				if _, err := os.Stat(home + "/package-called"); !os.IsNotExist(err) {
					t.Error("packages ran before the choice preflight", err)
				}
				for path, data := range original {
					blockEqual(t, path, data)
				}
				if after, err := os.Lstat(choice); err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
					t.Fatal("failed preparation replaced the choice", err)
				}
			})
		}
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
	t.Setenv("WT_PROFILE_ID", "")
	settings := windowsHome + "/Microsoft/Windows Terminal/settings.json"
	if err := os.MkdirAll(rawParent(settings), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(settings, []byte(`{"profiles":{"list":[{"guid":"{963ff2f7-6aed-5ce3-9d91-90d99571f53a}","name":"Ubuntu-24.04","source":"Windows.Terminal.Wsl"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	probe := `#!/bin/sh
printf '%s\n' '{"appData":"C:\\Users\\Fixture\\AppData\\Local","terminalInstalled":true,"fontInstalled":false,"settingsPaths":["C:\\Users\\Fixture\\AppData\\Local\\Microsoft\\Windows Terminal\\settings.json"],"wslProfileGuids":[]}'
`
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(probe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(home+"/tools/wslpath", []byte(`#!/bin/sh
case "$*" in
 *settings.json*) printf '%s\n' "$HOME/windows-localappdata/Microsoft/Windows Terminal/settings.json" ;;
 *) printf '%s\n' "$HOME/windows-localappdata" ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	return root, home, paths, windowsHome
}

func TestWindowsTerminalChoiceLifecycle(t *testing.T) {
	root, _, paths, windowsHome := windowsTerminalFixture(t)
	settings := windowsHome + "/Microsoft/Windows Terminal/settings.json"
	original := blockRead(t, settings)
	code, out, stderr := blockRun(t, root, "y\nn\n", "install", "--skip-packages")
	if code != 0 || !strings.Contains(out, "Windows Terminal") {
		t.Fatalf("choice: %d %s %s", code, out, stderr)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if string(blockRead(t, settings)) != string(original) {
		t.Fatal("declined choice was not retained")
	}
	blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal")
	before := blockRead(t, settings)
	if !strings.Contains(string(before), `"colorScheme": "Dark+"`) {
		t.Fatalf("existing profile did not receive built-in Dark+: %s", before)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	if string(blockRead(t, settings)) != string(before) {
		t.Fatal("reinstall/update changed the profile")
	}
	blockOK(t, root, "uninstall", "--restore", "--yes")
	j, err := parseTerminalJSON(blockRead(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := j.profile("{963ff2f7-6aed-5ce3-9d91-90d99571f53a}")
	if err != nil || profile == nil || profile.property("font") != nil || profile.property("colorScheme") != nil {
		t.Fatal("uninstall left originally absent appearance values", err)
	}
	for _, path := range []string{paths.State + "/windows-terminal.json", windowsProfileStatePath(paths), paths.Resources + "/windows-terminal.state", windowsHome + "/Microsoft/Windows Terminal/Fragments"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("terminal state or fragment left after uninstall: %s: %v", path, err)
		}
	}
}

func TestWindowsTerminalDryRunAndUnavailableInterop(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	out := blockOK(t, root, "install", "--skip-packages", "--yes", "--windows-terminal", "--dry-run")
	if !strings.Contains(out, "Windows Terminal") {
		t.Fatalf("missing preview: %s", out)
	}
	for _, path := range []string{paths.Config, paths.State, windowsHome + "/Microsoft/Windows Terminal/Fragments"} {
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

func TestWindowsTerminalProfileBackupAndPlatformGuard(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	original := blockRead(t, settings)
	blockOK(t, root, "install", "--windows-terminal", "--skip-packages", "--yes")
	first, err := readWindowsProfileState(paths)
	if err != nil || first == nil {
		t.Fatal("profile journal missing", err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	second, err := readWindowsProfileState(paths)
	if err != nil || second == nil || first.Backup != second.Backup {
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
	if string(blockRead(t, settings)) != string(original) {
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

func TestWindowsTerminalPreservesExistingSchemes(t *testing.T) {
	root, _, _, windowsHome := windowsTerminalFixture(t)
	settings := windowsHome + "/Microsoft/Windows Terminal/settings.json"
	personal := `"schemes":[{"name":"Dark+","background":"#010203"}]`
	original := strings.TrimSuffix(string(blockRead(t, settings)), "}") + "," + personal + "}"
	if err := testutil.WriteFile(settings, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	fragment := windowsHome + "/Microsoft/Windows Terminal/Fragments/Selfishell/963ff2f7-6aed-5ce3-9d91-90d99571f53a.json"
	data := []byte(`{"schemes":[{"name":"Personal","background":"#123456"}]}`)
	if err := os.MkdirAll(rawParent(fragment), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(fragment, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"install", "--windows-terminal", "--skip-packages", "--yes"}, {"uninstall", "--yes"}} {
		blockOK(t, root, args...)
		if string(blockRead(t, fragment)) != string(data) || !strings.Contains(string(blockRead(t, settings)), personal) {
			t.Fatal("changed a user-owned scheme or fragment")
		}
	}
}

func TestWindowsFontUpgradeUsesNewPathAndOwnedRegistration(t *testing.T) {
	for _, scenario := range []string{"reuse", "modified", "unrecorded", "registration-retry"} {
		t.Run(scenario, func(t *testing.T) {
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
 if os.path.exists(os.environ['HOME']+'/fail-registration'): sys.exit(1)
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
			var request struct {
				Path         string `json:"path"`
				PreviousPath string `json:"previousPath"`
			}
			if err := json.Unmarshal(blockRead(t, home+"/registration-request"), &request); err != nil {
				t.Fatal(err)
			}
			if request.PreviousPath != oldTarget || request.Path != newTarget || !strings.HasPrefix(newTarget, windowsHome) {
				t.Fatal("registration cannot verify prior ownership", request)
			}
			// Returning to a retained pin must neither download nor replace its file.
			before, err := os.Stat(oldTarget)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			writeManifest("3.4.0", []byte("old-font"))
			switch scenario {
			case "modified":
				if err := testutil.WriteFile(oldTarget, []byte("user-edited-font"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unrecorded":
				if err := os.Remove(paths.State + "/retained-fonts/jetbrainsmono-regular/3.4.0.json"); err != nil {
					t.Fatal(err)
				}
			case "registration-retry":
				if err := testutil.WriteFile(home+"/fail-registration", nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "modified" || scenario == "unrecorded" {
				preserved := blockRead(t, oldTarget)
				registration := blockRead(t, home+"/registration-request")
				op = &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
				err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false)
				if err == nil || !strings.Contains(err.Error(), "preserving") {
					t.Fatal("unsafe retained path accepted", err)
				}
				if string(blockRead(t, oldTarget)) != string(preserved) || string(blockRead(t, home+"/registration-request")) != string(registration) || string(blockRead(t, paths.State+"/dependencies/jetbrainsmono-regular")) != "3.4.1\n" {
					t.Fatal("rejected rollback changed font, registration, or current pin")
				}
				return
			}
			if scenario == "registration-retry" {
				op = &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
				if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err == nil {
					t.Fatal("registration failure ignored")
				}
				if _, err := os.Stat(paths.State + "/pending-fonts/jetbrainsmono-regular"); err != nil {
					t.Fatal("registration failure lost recovery journal", err)
				}
				if err := os.Remove(home + "/fail-registration"); err != nil {
					t.Fatal(err)
				}
			}
			op = &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
			if err := op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false); err != nil {
				t.Fatal("return to retained pin", err)
			}
			after, err := os.Stat(oldTarget)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("retained font was replaced", err)
			}
			if string(blockRead(t, paths.State+"/dependencies/jetbrainsmono-regular")) != "3.4.0\n" {
				t.Fatal("restored pin not recorded")
			}
			if err := json.Unmarshal(blockRead(t, home+"/registration-request"), &request); err != nil {
				t.Fatal(err)
			}
			if request.PreviousPath != newTarget || request.Path != oldTarget {
				t.Fatal("rollback registration cannot verify prior ownership", request)
			}
			if _, err := os.Stat(paths.State + "/pending-fonts/jetbrainsmono-regular"); !os.IsNotExist(err) {
				t.Fatal("completed registration retained pending journal", err)
			}
		})
	}
}

func TestWindowsTerminalSavedChoiceDoesNotNeedInterop(t *testing.T) {
	root, home, paths, _ := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	before := blockRead(t, paths.State+"/windows-terminal.json")
	if err := os.Remove(home + "/tools/powershell.exe"); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	if string(blockRead(t, paths.State+"/windows-terminal.json")) != string(before) {
		t.Fatal("saved choice changed")
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
	choice := windowsTerminalChoice{Version: 1, Enabled: true, Distro: "Ubuntu", AppData: "C:/fixture", AppDataPath: scratch, SettingsPath: scratch + "/settings.json", ProfileGUID: "{2c4de342-38b7-51cf-b940-2309a097f518}"}
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
	oldVersion := dep.Version
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
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	dep.Version = oldVersion
	record = strings.Join([]string{dep.Kind, dep.Name, dep.Version, dep.Platform, dep.Arch, dep.Source, dep.Checksum, dep.Target, dep.Marker}, " ") + "\n"
	if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	op = &PackageOperation{Process: op.Process}
	if err := op.InstallDirect(ctx, paths, manifest, "required", dep.Name, "ubuntu-wsl", "amd64", false); err != nil {
		t.Fatal("return to locked retained font", err)
	}
	if string(blockRead(t, paths.State+"/dependencies/"+dep.Name)) != oldVersion+"\n" || string(blockRead(t, oldTarget)) != string(payload) {
		t.Fatal("loaded-font rollback did not restore pin")
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

func TestWindowsPackageSelectionKeepsNonFontPackages(t *testing.T) {
	for _, scenario := range []string{"declined", "enabled", "absent", "in-memory"} {
		t.Run(scenario, func(t *testing.T) {
			root, _, paths, _ := windowsTerminalFixture(t)
			enabled := scenario == "enabled"
			answer := "y\nn\n"
			if enabled {
				answer = "y\ny\n"
			}
			code, _, stderr := blockRun(t, root, answer, "install", "--skip-packages")
			if code != 0 {
				t.Fatal(stderr)
			}
			if scenario == "absent" {
				if err := os.Remove(paths.State + "/windows-terminal.json"); err != nil {
					t.Fatal(err)
				}
			}
			root = t.TempDir() // The normal CLI fixture root is the source checkout.
			manifest := "package ubuntu-wsl required direct wsl-helper\npackage ubuntu-wsl required apt wsl-package\npackage ubuntu-wsl optional direct fixture-font\npackage ubuntu required apt ubuntu-package\npackage macos required formula mac-package\n"
			if err := testutil.WriteFile(root+"/packages.conf", []byte(manifest), 0600); err != nil {
				t.Fatal(err)
			}
			// Font selection follows the existing marker, not a package-name prefix.
			deps := "download fixture-font 1 linux all https://example.invalid/font abc .local/share/fonts/fixture.ttf font\ndownload wsl-helper 1 linux all https://example.invalid/helper abc .local/bin/wsl-helper binary\n"
			if err := testutil.WriteFile(root+"/dependencies.conf", []byte(deps), 0600); err != nil {
				t.Fatal(err)
			}
			packages, err := ReadPackages(root + "/packages.conf")
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			c := CLI{Root: root, Out: &out, Err: io.Discard}
			op := &PackageOperation{Process: Process{Out: &out, Err: io.Discard}}
			if scenario == "in-memory" {
				op.windowsTerminal = &windowsTerminalChoice{Enabled: true}
			}
			if err := c.installPackages(context.Background(), op, paths, packages, "ubuntu-wsl", "amd64", false, true); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"wsl-helper", "wsl-package", "ubuntu-package"} {
				if !strings.Contains(out.String(), name) {
					t.Errorf("install skipped %s: %s", name, out.String())
				}
			}
			if strings.Contains(out.String(), "fixture-font") != (enabled || scenario == "in-memory") || strings.Contains(out.String(), "mac-package") {
				t.Errorf("wrong install selection: %s", out.String())
			}
			selected, err := diagnosticPackages(root, "ubuntu-wsl")
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, p := range selected {
				names[p.Name] = true
			}
			for _, name := range []string{"wsl-helper", "wsl-package", "ubuntu-package"} {
				if !names[name] {
					t.Errorf("status skipped %s", name)
				}
			}
			if names["fixture-font"] != enabled || names["mac-package"] {
				t.Fatalf("wrong status selection: %v", names)
			}
		})
	}
}

func TestPrepareConfigIncludesWindowsTerminal(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	c := CLI{Root: root, Out: io.Discard, Err: io.Discard}
	original := blockRead(t, settings)
	prepared, err := c.prepareConfig("ubuntu-wsl", true, true, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.windowsTerminal == nil || !prepared.windowsTerminal.Enabled {
		t.Fatal("common preparation omitted Windows Terminal")
	}
	if string(blockRead(t, settings)) != string(original) {
		t.Fatal("preparation changed settings")
	}
	for _, path := range []string{paths.Config, paths.State} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("preparation created %s: %v", path, err)
		}
	}
}

func TestWindowsTerminalPreflightBeforePackages(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			root, home, paths, settings := existingWindowsProfileFixture(t)
			blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
			// A changed path must stop setup before a package-manager process runs.
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(home+"/absent-settings", settings); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"apt-get", "dpkg-query", "mise", "curl"} {
				if err := testutil.WriteFile(home+"/tools/"+name, []byte("#!/bin/sh\nprintf called >\"$HOME/package-called\"\nexit 99\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			journal := blockRead(t, windowsProfileStatePath(paths))
			args := []string{command, "--yes"}
			if command == "update" {
				args = append(args, "--tools-only")
			}
			code, _, stderr := blockRun(t, root, "", args...)
			if code == 0 || !strings.Contains(stderr, "not a regular file") {
				t.Fatalf("unsafe settings accepted: %d %s", code, stderr)
			}
			if _, err := os.Stat(home + "/package-called"); !os.IsNotExist(err) {
				t.Fatal("packages ran before terminal preflight", err)
			}
			if string(blockRead(t, windowsProfileStatePath(paths))) != string(journal) {
				t.Fatal("failed preflight changed journal")
			}
		})
	}
}
