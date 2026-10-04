package selfishell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

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
	for _, scenario := range []string{"other-distro-renamed", "modern", "missing", "ambiguous", "current", "hidden", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			root, home, paths, settings := existingWindowsProfileFixture(t)
			data := `{"profiles":{"list":[{"guid":"{2c4de342-38b7-51cf-b940-2309a097f518}","source":"Windows.Terminal.Wsl","name":"Renamed shell"}]}}`
			t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
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
				t.Setenv("WT_PROFILE_ID", "{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}")
			case "missing":
				data = `{"profiles":{"list":[]}}`
			case "hidden":
				data = strings.Replace(data, `"name":`, `"hidden":true,"name":`, 1)
			case "malformed":
				data = `{"profiles":/*`
			}
			if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			out := blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
			applied := scenario == "other-distro-renamed" || scenario == "modern" || scenario == "current"
			if !applied {
				if string(blockRead(t, settings)) != data || !strings.Contains(out, "Skipping Windows Terminal setup") {
					t.Fatal("unsafe or unreported profile selection", out)
				}
				if _, err := os.Stat(windowsProfileStatePath(paths)); !os.IsNotExist(err) {
					t.Fatal("created integration without a unique target", err)
				}
				return
			}
			j, err := parseTerminalJSON(blockRead(t, settings))
			if err != nil {
				t.Fatal(err)
			}
			guid := "{2c4de342-38b7-51cf-b940-2309a097f518}"
			if scenario == "modern" || scenario == "current" {
				guid = "{cea72b9a-15bb-5424-9d74-b2555c3f2eaa}"
			}
			profile, err := j.profile(guid)
			if err != nil || j.text(profile.property("font").property("face")) != "JetBrainsMonoNL Nerd Font Mono" {
				t.Fatal("wrong distro profile", err)
			}
			if scenario == "current" {
				legacy, _ := j.profile("{2c4de342-38b7-51cf-b940-2309a097f518}")
				if legacy.property("font") != nil || legacy.property("colorScheme") != nil {
					t.Fatal("modified a second profile")
				}
			}
		})
	}
}

func TestWindowsProfilePreservesUserEditsAndRestoresOnlyOwnedValues(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	data := strings.Replace(string(blockRead(t, settings)), `"size": 15`, `"size": 19`, 1)
	if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	if string(blockRead(t, settings)) != data {
		t.Fatal("update changed unrelated user settings")
	}
	data = strings.Replace(data, `"face": "JetBrainsMonoNL Nerd Font Mono"`, `"face": "My personal font"`, 1)
	if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := blockRun(t, root, "", "update", "--tools-only", "--skip-packages", "--yes")
	if code == 0 || !strings.Contains(stderr, "modified") || string(blockRead(t, settings)) != data {
		t.Fatal("update overwrote changed user font", code, stderr)
	}
	s, err := readWindowsProfileState(paths)
	if err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, s.Backup)) != terminalSettingsFixture {
		t.Fatal("original settings backup changed")
	}
	blockOK(t, root, "uninstall", "--yes")
	want := strings.Replace(terminalSettingsFixture, `"size": 15`, `"size": 19`, 1)
	want = strings.Replace(want, `"face": "Cascadia Mono"`, `"face": "My personal font"`, 1)
	if string(blockRead(t, settings)) != want {
		t.Fatal("uninstall failed to preserve user edits or restore unchanged theme", string(blockRead(t, settings)))
	}
}

func TestWindowsProfileUninstallPreservesRawUserThemeAndRestoreRetry(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupted), func(t *testing.T) {
			root, _, paths, settings := existingWindowsProfileFixture(t)
			original := strings.Replace(terminalSettingsFixture, `"dark": "Campbell"`, `"dark": /* original comment */ "Campbell"`, 1)
			if err := testutil.WriteFile(settings, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
			want := original
			if interrupted {
				m := managed{c: CLI{Out: io.Discard, Err: io.Discard}, paths: paths}
				m.atomicWrite = func(path string, data []byte, mode os.FileMode) error {
					if err := writeAtomic(path, data, mode); err != nil {
						return err
					}
					if path == settings {
						return fmt.Errorf("interrupted after restoring settings")
					}
					return nil
				}
				if err := m.removeWindowsProfile(false); err == nil {
					t.Fatal("ignored restoration failure")
				}
				if string(blockRead(t, settings)) != original {
					t.Fatal("first restore lost original comments")
				}
			} else {
				personal := `{"dark": /* personal comment */ "Personal theme", "light": "Campbell"}`
				data := strings.Replace(string(blockRead(t, settings)), `"Dark+"`, personal, 1)
				if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				want = strings.Replace(data, `"face": "JetBrainsMonoNL Nerd Font Mono"`, `"face": "Cascadia Mono"`, 1)
			}
			blockOK(t, root, "uninstall", "--yes")
			if string(blockRead(t, settings)) != want {
				t.Fatal("uninstall rewrote retained theme bytes or lost comments", string(blockRead(t, settings)))
			}
		})
	}
}

func TestWindowsProfileUninstallPreservesCommentsInCreatedFont(t *testing.T) {
	for _, key := range []string{"face", "colorScheme"} {
		for _, tc := range []struct{ name, property, comment string }{
			{"before-key", `/* 사용자 메모 */ %s:`, "/* 사용자 메모 */"},
			{"before-colon", `%s /* 사용자 메모 */:`, "/* 사용자 메모 */"},
			{"before-value", `%s: /* 사용자 메모 */`, "/* 사용자 메모 */"},
			{"line-comment", "%s: // 사용자 메모\n", "// 사용자 메모\n"},
			{"line-comment-crlf", "%s: // 사용자 메모\r\n", "// 사용자 메모\r\n"},
		} {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				root, _, _, settings := existingWindowsProfileFixture(t)
				// Both appearance properties are absent before setup; other user
				// settings, the other profile, comments, and BOM must survive.
				original := "\xef\xbb\xbf" + strings.Replace(terminalSettingsFixture, `"font": {"face": "Cascadia Mono", "size": 15, "weight": "bold"}, "colorScheme": {"dark": "Campbell", "light": "One Half Light"}, `, "", 1)
				if err := testutil.WriteFile(settings, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
				installed := string(blockRead(t, settings))
				// Select the added property in the target profile, not the other
				// profile's font or the global colorScheme default.
				value := `"JetBrainsMonoNL Nerd Font Mono"`
				if key == "colorScheme" {
					value = `"Dark+"`
				}
				quotedKey := `"` + key + `"`
				data := strings.Replace(installed, quotedKey+": "+value, fmt.Sprintf(tc.property, quotedKey)+" "+value, 1)
				if data == installed {
					t.Fatal("test did not insert a user comment")
				}
				if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				blockOK(t, root, "uninstall", "--yes")
				after := blockRead(t, settings)
				if !bytes.Contains(after, []byte(tc.comment)) {
					t.Fatalf("uninstall removed user comment or its newline: %s", after)
				}
				j, err := parseTerminalJSON(after)
				if err != nil {
					t.Fatal(err)
				}
				profile, err := j.profile("{963ff2f7-6aed-5ce3-9d91-90d99571f53a}")
				if err != nil || profile == nil {
					t.Fatal("profile identity changed", err)
				}
				if profile.property("font").property("face") != nil || profile.property("colorScheme") != nil {
					t.Fatal("originally absent properties were not removed")
				}
				if key == "face" && profile.property("font") == nil {
					t.Fatal("font container with a user comment was removed")
				}
				before, err := parseTerminalJSON([]byte(original))
				if err != nil {
					t.Fatal(err)
				}
				var want, got map[string]any
				if err := json.Unmarshal(before.clean, &want); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(j.clean, &got); err != nil {
					t.Fatal(err)
				}
				// An empty font container retains the user's comment.
				target := got["profiles"].(map[string]any)["list"].([]any)[1].(map[string]any)
				if key == "face" {
					delete(target, "font")
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatal("uninstall changed unrelated settings")
				}
				unchangedPrefix := original[:strings.Index(original, `      {"guid": "{963ff2f7`)]
				if !bytes.HasPrefix(after, []byte(unchangedPrefix)) {
					t.Fatal("uninstall changed the BOM, existing comments, other profile, or unrelated formatting")
				}
			})
		}
	}
}

func TestWindowsProfileStatusDetectsChangedAppearance(t *testing.T) {
	root, _, _, settings := existingWindowsProfileFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	data := strings.Replace(string(blockRead(t, settings)), `"Dark+"`, `"Personal theme"`, 1)
	if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	_, out, _ := blockRun(t, root, "", "status", "--verbose")
	if !strings.Contains(out, "[CHANGED] ~/windows-localappdata/Microsoft/Windows Terminal/settings.json (Windows Terminal font/theme)") {
		t.Fatal("status missed changed Windows settings", out)
	}
}

func TestWindowsProfileInterruptedWriteRecovery(t *testing.T) {
	for _, stage := range []string{"before-backup", "before-settings", "after-settings"} {
		t.Run(stage, func(t *testing.T) {
			root, _, paths, settings := existingWindowsProfileFixture(t)
			c := CLI{Root: root, Out: io.Discard, Err: io.Discard}
			choice, err := c.prepareWindowsTerminal(paths, false, true, false, true)
			if err != nil {
				t.Fatal(err)
			}
			m := managed{c: c, paths: paths, yes: true, actions: map[string]string{}}
			m.atomicWrite = func(path string, data []byte, mode os.FileMode) error {
				if path != settings {
					if err := writeAtomic(path, data, mode); err != nil {
						return err
					}
					if stage == "before-backup" && path == windowsProfileStatePath(paths) {
						return fmt.Errorf("interrupted after pending journal")
					}
					return nil
				}
				if stage == "after-settings" {
					if err := writeAtomic(path, data, mode); err != nil {
						return err
					}
				}
				return fmt.Errorf("interrupted settings write")
			}
			if err := m.installWindowsProfile(choice, false); err == nil {
				t.Fatal("write failure was ignored")
			}
			s, err := readWindowsProfileState(paths)
			if err != nil || s == nil || s.Status != "pending" {
				t.Fatal("missing pending journal", s, err)
			}
			if stage == "before-backup" {
				if _, err := os.Stat(s.Backup); !os.IsNotExist(err) {
					t.Fatal("backup created before interruption", err)
				}
				if string(blockRead(t, settings)) != terminalSettingsFixture {
					t.Fatal("settings changed before backup")
				}
			} else if string(blockRead(t, s.Backup)) != terminalSettingsFixture {
				t.Fatal("missing original backup")
			}
			if err := m.removeWindowsProfile(true); err == nil {
				t.Fatal("uninstall accepted pending changes to an existing profile")
			}
			m.atomicWrite = nil
			if err := m.installWindowsProfile(choice, false); err != nil {
				t.Fatal("interruption could not recover", err)
			}
			if string(blockRead(t, s.Backup)) != terminalSettingsFixture {
				t.Fatal("recovery lost original backup")
			}
			if err := m.removeWindowsProfile(false); err != nil {
				t.Fatal(err)
			}
			if string(blockRead(t, settings)) != terminalSettingsFixture {
				t.Fatal("recovery lost original values")
			}
		})
	}
}

func TestWindowsProfileBeforeBackupRecoveryPreservesUnrelatedEdits(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	c := CLI{Root: root, Out: io.Discard, Err: io.Discard}
	choice, err := c.prepareWindowsTerminal(paths, false, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	m := managed{c: c, paths: paths, yes: true, actions: map[string]string{}}
	m.atomicWrite = func(path string, data []byte, mode os.FileMode) error {
		if err := writeAtomic(path, data, mode); err != nil {
			return err
		}
		if path == windowsProfileStatePath(paths) {
			return fmt.Errorf("interrupted before backup")
		}
		return nil
	}
	if err := m.installWindowsProfile(choice, false); err == nil {
		t.Fatal("interruption was ignored")
	}
	pending, err := readWindowsProfileState(paths)
	if err != nil || pending == nil || pending.Status != "pending" {
		t.Fatal("missing recovery journal", pending, err)
	}
	changed := strings.Replace(terminalSettingsFixture, `"size": 15`, `"size": 20`, 1)
	if err := testutil.WriteFile(settings, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	m.atomicWrite = nil
	m.dry = true
	if err := m.installWindowsProfile(choice, false); err != nil {
		t.Fatal("recovery preview failed", err)
	}
	if _, err := os.Stat(pending.Backup); !os.IsNotExist(err) {
		t.Fatal("preview created a backup", err)
	}
	blockEqual(t, settings, []byte(changed))
	m.dry = false
	if err := m.installWindowsProfile(choice, false); err != nil {
		t.Fatal("unrelated edit prevented recovery", err)
	}
	active, err := readWindowsProfileState(paths)
	if err != nil || active == nil || active.Status != "active" || active.Backup != pending.Backup {
		t.Fatal("recovery replaced the original backup path", active, err)
	}
	blockEqual(t, active.Backup, []byte(changed))
	if err := m.removeWindowsProfile(false); err != nil {
		t.Fatal("recovered configuration could not be removed", err)
	}
	blockEqual(t, settings, []byte(changed))
}

func TestWindowsProfilePendingTargetRemoved(t *testing.T) {
	for _, target := range []string{"missing-settings", "missing-profile", "malformed", "symlink", "directory"} {
		t.Run(target, func(t *testing.T) {
			root, _, paths, settings := existingWindowsProfileFixture(t)
			c := CLI{Root: root, Out: io.Discard, Err: io.Discard}
			choice, err := c.prepareWindowsTerminal(paths, false, true, false, true)
			if err != nil {
				t.Fatal(err)
			}
			m := managed{c: c, paths: paths, yes: true, actions: map[string]string{}}
			m.atomicWrite = func(path string, data []byte, mode os.FileMode) error {
				if path == settings {
					return fmt.Errorf("interrupted settings write")
				}
				return writeAtomic(path, data, mode)
			}
			if err := m.installWindowsProfile(choice, false); err == nil {
				t.Fatal("ignored interruption")
			}
			s, err := readWindowsProfileState(paths)
			if err != nil || s == nil || s.Status != "pending" {
				t.Fatal("missing pending journal", s, err)
			}
			journal := blockRead(t, windowsProfileStatePath(paths))
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			remaining := `{"profiles":{"list":[{"guid":"{00000000-0000-0000-0000-000000000001}","name":"PowerShell","font":{"face":"Consolas"}}]}}`
			switch target {
			case "missing-profile":
				err = testutil.WriteFile(settings, []byte(remaining), 0600)
			case "malformed":
				err = testutil.WriteFile(settings, []byte(`{"profiles":/*`), 0600)
			case "symlink":
				err = os.Symlink(settings+".absent", settings)
			case "directory":
				err = os.Mkdir(settings, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			m.atomicWrite = nil
			if target == "malformed" || target == "symlink" || target == "directory" {
				if err := m.installWindowsProfile(choice, false); err == nil {
					t.Fatal("retry accepted unsafe settings")
				}
				code, _, _ := blockRun(t, root, "", "uninstall", "--yes")
				if code == 0 || !bytes.Equal(blockRead(t, windowsProfileStatePath(paths)), journal) {
					t.Fatal("uninstall discarded state for unsafe settings")
				}
				return
			}
			// Follow the recovery advice through the public CLI, then check that
			// a dry run leaves the journal for the real uninstall to remove.
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockOK(t, root, "uninstall", "--dry-run", "--yes")
			if !bytes.Equal(blockRead(t, windowsProfileStatePath(paths)), journal) {
				t.Fatal("preview changed pending journal")
			}
			blockOK(t, root, "uninstall", "--yes")
			if _, err := os.Stat(windowsProfileStatePath(paths)); !os.IsNotExist(err) {
				t.Fatal("uninstall left pending journal", err)
			}
			if string(blockRead(t, s.Backup)) != terminalSettingsFixture {
				t.Fatal("uninstall changed original backup")
			}
			if target == "missing-settings" {
				if _, err := os.Stat(settings); !os.IsNotExist(err) {
					t.Fatal("recreated removed settings", err)
				}
			} else if string(blockRead(t, settings)) != remaining {
				t.Fatal("changed remaining profile")
			}
		})
	}
}

func TestWindowsProfileMissingDefaultsAndChangedPathProtection(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	data := `{"profiles":{"list":[{"guid":"{963ff2f7-6aed-5ce3-9d91-90d99571f53a}","source":"Windows.Terminal.Wsl","name":"Ubuntu",}]}}`
	if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(paths.Config+"/zsh/zshrc", settings); err != nil {
		t.Fatal(err)
	}
	code, _, _ := blockRun(t, root, "", "uninstall", "--yes")
	if code == 0 {
		t.Fatal("uninstall followed a replaced settings symlink")
	}
	if _, err := os.Stat(paths.Config + "/zsh/zshrc"); err != nil {
		t.Fatal("uninstall removed files before preflight", err)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	// Resume with the unchanged installed file, then restore absent properties.
	installed := strings.Replace(data, `"name":"Ubuntu"`, `"name":"Ubuntu", "font": {"face": "JetBrainsMonoNL Nerd Font Mono"}, "colorScheme": "Dark+"`, 1)
	if err := testutil.WriteFile(settings, []byte(installed), 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "uninstall", "--yes")
	j, err := parseTerminalJSON(blockRead(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := j.profile("{963ff2f7-6aed-5ce3-9d91-90d99571f53a}")
	if profile.property("font") != nil || profile.property("colorScheme") != nil {
		t.Fatal("uninstall left values that were originally absent")
	}
}

func existingWindowsProfileFixture(t *testing.T) (string, string, Paths, string) {
	t.Helper()
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	t.Setenv("WT_PROFILE_ID", "")
	settings := windowsHome + "/Microsoft/Windows Terminal/settings.json"
	if err := os.MkdirAll(rawParent(settings), 0700); err != nil {
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

func TestWindowsSetupUpdatesExistingRenamedProfile(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	after := blockRead(t, settings)
	if !bytes.Contains(after, []byte(`"face": "JetBrainsMonoNL Nerd Font Mono"`)) || !bytes.Contains(after, []byte(`"colorScheme": "Dark+"`)) {
		t.Fatalf("existing profile did not receive font and theme: %s", after)
	}
	want := strings.Replace(terminalSettingsFixture, `"face": "Cascadia Mono"`, `"face": "JetBrainsMonoNL Nerd Font Mono"`, 1)
	want = strings.Replace(want, `{"dark": "Campbell", "light": "One Half Light"}`, `"Dark+"`, 1)
	if string(after) != want {
		t.Fatal("setup changed settings outside the two selected values")
	}
	for _, path := range []string{paths.Resources + "/windows-terminal.state", rawParent(settings) + "/Fragments"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("setup created a scheme fragment or record: %s: %v", path, err)
		}
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	if !bytes.Equal(blockRead(t, settings), after) {
		t.Fatal("repeated setup changed settings")
	}
	blockOK(t, root, "uninstall", "--yes")
	if string(blockRead(t, settings)) != terminalSettingsFixture {
		t.Fatal("uninstall failed to restore the original values")
	}
}

func TestWindowsProfileJSONCommentsAndAbsentProperties(t *testing.T) {
	for _, extra := range []string{
		``,
		`, "font": {}`,
		`, "font": {"size": 17,}`,
		`, "font": {"face":"Original", /* comma , inside comment */ "size":17,}`,
		`, "font": {"size":17, "face":"Original",}`,
		`, "font": {"face":"Original",}, "colorScheme": {"dark": /* preserved */ "Campbell", "light":"One Half Light",}`,
	} {
		t.Run(extra, func(t *testing.T) {
			root, _, _, settings := existingWindowsProfileFixture(t)
			data := "\xef\xbb\xbf" + `{"other":"https://example.invalid/escaped\\\"text", "profiles":{"list":[{"guid":"{963ff2f7-6aed-5ce3-9d91-90d99571f53a}","source":"Windows.Terminal.Wsl","name":"개발 shell"` + extra + `,}]}}`
			if err := testutil.WriteFile(settings, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
			blockOK(t, root, "uninstall", "--yes")
			before, err := parseTerminalJSON([]byte(data))
			if err != nil {
				t.Fatal(err)
			}
			after, err := parseTerminalJSON(blockRead(t, settings))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before.value(before.root), after.value(after.root)) {
				t.Fatal("restoration changed original values", string(after.data))
			}
			if strings.Contains(data, "/* preserved */") && !bytes.Contains(after.data, []byte("/* preserved */")) {
				t.Fatal("lost comment inside original scheme")
			}
			if !bytes.HasPrefix(after.data, []byte{0xef, 0xbb, 0xbf}) {
				t.Fatal("lost UTF-8 BOM")
			}
		})
	}
}

func TestWindowsProfileNativeLifecycle(t *testing.T) {
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
	paths := Paths{State: home + "/state", Resources: home + "/state/resources"}
	settings := scratch + "/settings.json"
	if err := testutil.WriteFile(settings, []byte(terminalSettingsFixture), 0600); err != nil {
		t.Fatal(err)
	}
	choice := &windowsTerminalChoice{Version: 1, Enabled: true, Distro: "Ubuntu-24.04", AppData: "C:/fixture", AppDataPath: scratch, SettingsPath: settings, ProfileGUID: "{963ff2f7-6aed-5ce3-9d91-90d99571f53a}"}
	m := managed{c: CLI{Out: io.Discard, Err: io.Discard}, paths: paths, yes: true, actions: map[string]string{}}
	if err := m.installWindowsProfile(choice, true); err != nil {
		t.Fatal(err)
	}
	if err := m.installWindowsProfile(choice, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(blockRead(t, settings), []byte(`"face": "JetBrainsMonoNL Nerd Font Mono"`)) {
		t.Fatal("native profile unchanged")
	}
	if err := m.removeWindowsProfile(true); err != nil {
		t.Fatal(err)
	}
	if err := m.removeWindowsProfile(false); err != nil {
		t.Fatal(err)
	}
	if string(blockRead(t, settings)) != terminalSettingsFixture {
		t.Fatal("native original values not restored")
	}
}

func TestWindowsProfileDiscoveryReadOnly(t *testing.T) {
	if os.Getenv("SELFISHELL_TEST_WSL_INTEROP") != "1" {
		t.Skip("requires native WSL interoperability")
	}
	t.Parallel()
	distro := os.Getenv("WSL_DISTRO_NAME")
	if distro == "" {
		t.Skip("requires a native WSL distribution identity")
	}
	p := Process{Env: withEnvironment(Process{}, map[string]string{"HOME": t.TempDir()}).Env}
	data, err := p.windowsScript(context.Background(), map[string]string{"operation": "probe", "distro": distro})
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		SettingsPaths []string `json:"settingsPaths"`
		Guids         []string `json:"wslProfileGuids"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatal(err)
	}
	_, guid, _, err := (CLI{}).findWindowsProfile(p, distro, probe.SettingsPaths, probe.Guids)
	if err != nil {
		t.Skipf("native settings have no unique visible target: %v", err)
	}
	if !validTerminalGUID(guid) {
		t.Fatal("invalid native profile identity")
	}
}

func TestWindowsProfileSettingsChangedAfterJournal(t *testing.T) {
	root, _, paths, settings := existingWindowsProfileFixture(t)
	c := CLI{Root: root, Out: io.Discard, Err: io.Discard}
	choice, err := c.prepareWindowsTerminal(paths, false, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	m := managed{c: c, paths: paths, yes: true, actions: map[string]string{}}
	changed := strings.Replace(terminalSettingsFixture, `"size": 15`, `"size": 20`, 1)
	m.atomicWrite = func(path string, data []byte, mode os.FileMode) error {
		if err := writeAtomic(path, data, mode); err != nil {
			return err
		}
		if path == windowsProfileStatePath(paths) {
			if err := testutil.WriteFile(settings, []byte(changed), 0600); err != nil {
				return err
			}
			m.atomicWrite = nil // Exercise the actual writeRaw publish callback below.
		}
		return nil
	}
	err = m.installWindowsProfile(choice, false)
	if err == nil || !strings.Contains(err.Error(), "changed during setup") {
		t.Fatal("expected concurrent modification guard", err)
	}
	if string(blockRead(t, settings)) != changed {
		t.Fatal("overwrote concurrent user edit")
	}
	state, err := readWindowsProfileState(paths)
	if err != nil || state == nil || state.Status != "pending" {
		t.Fatal("missing recoverable journal", err)
	}
	if err := m.installWindowsProfile(choice, false); err != nil {
		t.Fatal("retry", err)
	}
	if err := m.removeWindowsProfile(false); err != nil {
		t.Fatal("uninstall", err)
	}
	if string(blockRead(t, settings)) != changed {
		t.Fatal("retry lost concurrent user edit")
	}
}
