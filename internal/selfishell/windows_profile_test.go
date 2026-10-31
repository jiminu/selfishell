package selfishell

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

// A renamed Ubuntu profile with an unrelated profile and user font settings.
const terminalSettingsFixture = `{
  // Keep this comment and all unrelated settings.
  "defaultProfile": "{00000000-0000-0000-0000-000000000001}",
  "profiles": {
    "defaults": {"font": {"size": 13}, "colorScheme": "Campbell"},
    "list": [
      {"guid": "{00000000-0000-0000-0000-000000000001}", "name": "PowerShell", "font": {"face": "Consolas"}},
      {"guid": "{963ff2f7-6aed-5ce3-9d91-90d99571f53a}", "name": "My development shell", "source": "Windows.Terminal.Wsl", "commandline": "wsl.exe -d Ubuntu-24.04 --cd /work --exec zsh --login", "font": {"face": "Cascadia Mono", "size": 15, "weight": "bold"}, "colorScheme": {"dark": "Campbell", "light": "One Half Light"}, "startingDirectory": "~",},
    ],
  },
}
`

func TestWindowsProfileDiscoveryUsesDistroIdentity(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, scenario := range []string{"other-distro-renamed", "modern", "missing", "ambiguous", "current", "hidden", "malformed", "duplicate-face"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			env := env
			root, home, paths, settings := existingWindowsProfileFixture(t, &env)
			data := `{"profiles":{"list":[{"guid":"{2c4de342-38b7-51cf-b940-2309a097f518}","source":"Windows.Terminal.Wsl","name":"Renamed shell"}]}}`
			setEnv(&env, "WSL_DISTRO_NAME", "Ubuntu")
			modern := `{"guid":"{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}","source":"Microsoft.WSL","name":"Renamed modern shell"}`
			if scenario == "modern" || scenario == "ambiguous" || scenario == "current" {
				probe := `#!/bin/sh
printf '%s\n' '{"appData":"C:\\Users\\Fixture\\AppData\\Local","terminalInstalled":true,"settingsPaths":["C:\\fixture\\settings.json"],"wslProfileGuids":["{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}"]}'
`
				if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(probe), 0700); err != nil {
					t.Fatal(err)
				}
				if scenario == "modern" {
					data = `{"profiles":{"list":[` + modern + `]}}`
				} else {
					data = strings.Replace(data, `}]}}`, `},`+modern+`]}}`, 1)
				}
			}
			switch scenario {
			case "current":
				setEnv(&env, "WT_PROFILE_ID", "{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}")
			case "missing":
				data = `{"profiles":{"list":[]}}`
			case "hidden":
				data = strings.Replace(data, `"name":`, `"hidden":true,"name":`, 1)
			case "malformed":
				data = `{"profiles":/*`
			case "duplicate-face":
				data = strings.Replace(data, `"name":`, `"font":{"face":"A","face":"B"},"name":`, 1)
			}
			if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			out := blockOK(t, env, root, "install", "--skip-packages", "--windows-terminal", "--yes")
			blockEqual(t, settings, []byte(data))
			record, err := readFragmentRecord(paths)
			if err != nil {
				t.Fatal(err)
			}
			applied := scenario == "other-distro-renamed" || scenario == "modern" || scenario == "current" || scenario == "duplicate-face"
			if !applied {
				if record != nil || !strings.Contains(out, "Skipping Windows Terminal setup") {
					t.Fatal("unsafe or unreported profile selection", out)
				}
				return
			}
			guid := "{2c4de342-38b7-51cf-b940-2309a097f518}"
			if scenario == "modern" || scenario == "current" {
				guid = "{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}"
			}
			if record == nil || !bytes.Contains(blockRead(t, record.Path), []byte(`"updates": "`+guid+`"`)) {
				t.Fatal("fragment does not update the distro profile", record)
			}
			if (scenario == "duplicate-face") != strings.Contains(out, "Could not check Windows Terminal settings") {
				t.Fatal("duplicate appearance keys must skip only the override notice", out)
			}
		})
	}
}

func TestWindowsTerminalOverrides(t *testing.T) {
	guid := "{963ff2f7-6aed-5ce3-9d91-90d99571f53a}"
	for _, tc := range []struct{ profile, defaults, want string }{
		{``, ``, ``},
		{`"font": {"face": "JetBrainsMonoNL Nerd Font Mono"}, "colorScheme": "Dark+",`, `"font": {"size": 12},`, ``},
		{`"font": {"face": "Consolas"}, "colorScheme": {"dark": "Campbell"},`, ``, `profile font.face "Consolas" takes precedence over Selfishell's font|profile colorScheme.dark "Campbell" takes precedence over Dark+`},
		{`"fontFace": "Consolas",`, `"colorScheme": "Campbell", "fontFace": "Lucida",`, `profile fontFace "Consolas" takes precedence over Selfishell's font|Defaults fontFace "Lucida" takes precedence over Selfishell's font|Defaults colorScheme "Campbell" takes precedence over Dark+`},
		// The Settings UI writes per-mode objects; dark mode still uses the fragment.
		{`"colorScheme": {"light": "Ubuntu"},`, `"colorScheme": {"dark": "Dark+", "light": "Campbell"},`, ``},
		// A font object makes Windows Terminal ignore the legacy key.
		{`"fontFace": "Consolas", "font": {"size": 12},`, ``, ``},
	} {
		j, err := parseTerminalJSON([]byte(`{"profiles": {"defaults": {` + tc.defaults + `}, "list": [{` + tc.profile + ` "guid": "` + guid + `"}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		profile, err := j.profile(guid)
		if err != nil {
			t.Fatal(err)
		}
		got, err := windowsTerminalOverrides(j, profile)
		if err != nil || strings.Join(got, "|") != tc.want {
			t.Errorf("%s / %s: got %q, %v", tc.profile, tc.defaults, got, err)
		}
	}
}

func TestWindowsTerminalReportsUserOverrides(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, _, _, settings := existingWindowsProfileFixture(t, &env)
	out := blockOK(t, env, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	blockEqual(t, settings, []byte(terminalSettingsFixture))
	if !strings.Contains(out, `profile font.face "Cascadia Mono" takes precedence`) || !strings.Contains(out, `Defaults colorScheme "Campbell" takes precedence`) {
		t.Fatal("missing override notice", out)
	}
	if _, out, _ := blockRun(t, env, root, "", "status"); !strings.Contains(out, `[INFO] Windows Terminal profile font.face "Cascadia Mono"`) {
		t.Fatal("status missed the override", out)
	}
}

func TestWindowsTerminalFragmentPreservesUserFiles(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root, _, paths, windowsHome := windowsTerminalFixture(t, &env)
	fragment := windowsHome + "/Microsoft/Windows Terminal/Fragments/Selfishell/963ff2f7-6aed-5ce3-9d91-90d99571f53a.json"
	foreign := []byte(`{"profiles": []}`)
	for _, dir := range []string{managed.RawParent(fragment), paths.State} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.WriteFile(fragment, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, env, root, "install", "--skip-packages", "--yes", "--windows-terminal")
	backups, err := filepath.Glob(paths.State + "/backups/windows-terminal-fragment.backup.*")
	if err != nil || len(backups) != 1 {
		t.Fatal("existing fragment was not backed up", backups, err)
	}
	blockEqual(t, backups[0], foreign)
	modified := append(blockRead(t, fragment), "// mine\n"...)
	if err := testutil.WriteFile(fragment, modified, 0600); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := blockRun(t, env, root, "", "status"); !strings.Contains(out, "[CHANGED] ~/windows-localappdata/Microsoft/Windows Terminal/Fragments/Selfishell/963ff2f7-6aed-5ce3-9d91-90d99571f53a.json (Windows Terminal fragment)") {
		t.Fatal("status missed the modified fragment", out)
	}
	code, _, stderr := blockRun(t, env, root, "", "update", "--tools-only", "--skip-packages", "--yes")
	if code == 0 || !strings.Contains(stderr, "modified") {
		t.Fatal("update overwrote a modified fragment", code, stderr)
	}
	blockEqual(t, fragment, modified)
	blockOK(t, env, root, "uninstall", "--yes")
	blockEqual(t, fragment, modified)
	if _, err := os.Stat(fragmentRecordPath(paths)); !os.IsNotExist(err) {
		t.Fatal("uninstall retained the fragment record", err)
	}
}

func existingWindowsProfileFixture(t *testing.T, env *process.Environment) (string, string, managed.Paths, string) {
	t.Helper()
	root, home, paths, windowsHome := windowsTerminalFixture(t, env)
	setEnv(env, "WT_PROFILE_ID", "")
	settings := windowsHome + "/Microsoft/Windows Terminal/settings.json"
	if err := os.MkdirAll(managed.RawParent(settings), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(settings, []byte(terminalSettingsFixture), 0600); err != nil {
		t.Fatal(err)
	}
	probe := `#!/bin/sh
printf '%s\n' '{"appData":"C:\\Users\\Fixture\\AppData\\Local","terminalInstalled":true,"fontInstalled":false,"settingsPaths":["C:\\Users\\Fixture\\AppData\\Local\\Microsoft\\Windows Terminal\\settings.json"],"wslProfileGuids":[]}'
`
	if err := testutil.WriteFile(home+"/tools/powershell.exe", []byte(probe), 0700); err != nil {
		t.Fatal(err)
	}
	pathScript := `#!/bin/sh
case "$*" in
  *settings.json*) printf '%s\n' "$HOME/windows-localappdata/Microsoft/Windows Terminal/settings.json" ;;
  *) printf '%s\n' "$HOME/windows-localappdata" ;;
esac
`
	if err := testutil.WriteFile(home+"/tools/wslpath", []byte(pathScript), 0700); err != nil {
		t.Fatal(err)
	}
	return root, home, paths, settings
}

func TestWindowsTerminalNativeFragment(t *testing.T) {
	base := os.Getenv("SELFISHELL_TEST_WINDOWS_TEMP")
	if base == "" {
		t.Skip("requires a private scratch directory on Windows filesystem")
	}
	t.Parallel()
	scratch, err := os.MkdirTemp(base, "selfishell-profile-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(scratch) })
	home := t.TempDir()
	paths := managed.Paths{State: home + "/state", Resources: home + "/state/resources"}
	settings := scratch + "/settings.json"
	if err := testutil.WriteFile(settings, []byte(terminalSettingsFixture), 0600); err != nil {
		t.Fatal(err)
	}
	choice := &windowsTerminalChoice{Version: 1, Enabled: true, Distro: "Ubuntu-24.04", AppData: "C:/fixture", AppDataPath: scratch, SettingsPath: settings, ProfileGUID: "{963ff2f7-6aed-5ce3-9d91-90d99571f53a}"}
	m := lifecycle{c: CLI{Out: io.Discard, Err: io.Discard}, paths: paths, yes: true, actions: map[string]string{}}
	for _, preflight := range []bool{true, false} {
		if err := installFragment(&m, paths, choice, preflight); err != nil {
			t.Fatal(err)
		}
	}
	path := scratch + "/Microsoft/Windows Terminal/Fragments/Selfishell/963ff2f7-6aed-5ce3-9d91-90d99571f53a.json"
	if !bytes.Contains(blockRead(t, path), []byte(`"updates": "`+choice.ProfileGUID+`"`)) {
		t.Fatal("native fragment does not update the profile")
	}
	for _, preflight := range []bool{true, false} {
		if err := removeFragment(&m, paths, "", preflight); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(managed.RawParent(path)); !os.IsNotExist(err) {
		t.Fatal("native fragment directory left behind", err)
	}
	blockEqual(t, settings, []byte(terminalSettingsFixture))
}
