package selfishell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGhosttyOverriddenSettings(t *testing.T) {
	defaults := []byte(`theme = Dark+
keybind = global:cmd+grave_accent=toggle_quick_terminal
keybind = ctrl+shift+k=scroll_page_lines:-5
font-family = "MesloLGS Nerd Font Mono"
font-feature = -calt, -liga, -dlig
font-codepoint-map = U+AC00-U+D7AF=Apple SD Gothic Neo
cursor-style = block
`)
	block, err := blockContent("user-ghostty", "/managed")
	if err != nil {
		t.Fatal(err)
	}
	user := string(block) + `theme = Nord
# cursor-style = bar
font-family = Fira Code
font-feature = ss01
font-codepoint-map = U+E000-U+F8FF=Symbols Nerd Font
keybind = cmd+grave_accent=toggle_quick_terminal
keybind = ctrl+shift+t=new_tab
cursor-style = bar` + "\r" + `
theme = Nord
window-decoration = false
`
	got := ghosttyOverridden(defaults, []byte(user))
	want := []string{"theme", "keybind cmd+grave_accent", "cursor-style"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := ghosttyOverridden(defaults, block); len(got) != 0 {
		t.Fatalf("block alone reported %q", got)
	}
}

func TestInstallNotesOverriddenGhosttySettings(t *testing.T) {
	for _, tc := range []struct{ name, existing, want string }{
		{"overridden", "theme = Nord\nfont-family = Fira Code\n", "theme"},
		{"fallback only", "font-family = Fira Code\n", ""},
		{"absent", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			entrypoint := filepath.Dir(paths.Config) + "/ghostty/config.ghostty"
			if tc.existing != "" {
				if err := os.MkdirAll(filepath.Dir(entrypoint), 0700); err != nil {
					t.Fatal(err)
				}
				blockWrite(t, entrypoint, []byte(tc.existing))
			}
			for _, args := range [][]string{{"install", "--skip-packages", "--yes", "--dry-run"}, {"install", "--skip-packages", "--yes"}} {
				out := blockOK(t, root, args...)
				note := "Selfishell's Ghostty defaults override these settings in " + entrypoint + ": " + tc.want + ". Move them to user.ghostty to keep them."
				if tc.want == "" && strings.Contains(out, "override these settings") || tc.want != "" && !strings.Contains(out, note) {
					t.Fatalf("%v: note for %q in %s", args, tc.want, out)
				}
			}
		})
	}
}
