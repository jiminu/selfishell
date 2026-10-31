package terminal

import (
	"strings"
	"testing"
)

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
