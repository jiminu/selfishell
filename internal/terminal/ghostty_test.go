package terminal

import (
	"reflect"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
)

func TestGhosttyOverriddenSettings(t *testing.T) {
	t.Parallel()
	defaults := []byte(`theme = Dark+
keybind = global:cmd+grave_accent=toggle_quick_terminal
keybind = ctrl+shift+k=scroll_page_lines:-5
font-family = "MesloLGS Nerd Font Mono"
font-feature = -calt, -liga, -dlig
font-codepoint-map = U+AC00-U+D7AF=Apple SD Gothic Neo
cursor-style = block
`)
	block, err := managed.BlockContent("user-ghostty", "/managed")
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
