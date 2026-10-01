package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestTerminalFontHintAppearsOnce(t *testing.T) {
	macos := "Set your terminal font to " + terminalFont + " to show Neovim's icons."
	desktop := "Install " + terminalFont + " and set it as your terminal font to show Neovim's icons."
	for _, tc := range []struct {
		name, platform, display, ssh, want string
		installed, ghostty, configured     bool
	}{
		{name: "new install", platform: "macos", want: macos},
		{name: "already installed", platform: "macos", installed: true},
		{name: "ghostty chosen", platform: "macos", ghostty: true},
		{name: "wsl first setup", platform: "ubuntu-wsl", want: "Install " + terminalFont + " on Windows and set it as your terminal font to show Neovim's icons."},
		{name: "wsl configured", platform: "ubuntu-wsl", configured: true},
		{name: "ubuntu desktop", platform: "ubuntu", display: ":0", want: desktop},
		{name: "ubuntu over ssh", platform: "ubuntu", display: ":0", ssh: "192.0.2.1 50000 192.0.2.2 22"},
		{name: "ubuntu headless", platform: "ubuntu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DISPLAY", tc.display)
			t.Setenv("WAYLAND_DISPLAY", "")
			t.Setenv("SSH_CONNECTION", tc.ssh)
			f := newPackageFixture(t)
			casks := ""
			if tc.installed {
				casks = terminalFontCask
			}
			f.executable("brew", fmt.Sprintf(`case "$1 $2" in 'list --cask') printf '%%s\n' %q;; esac`, casks))
			paths := Paths{State: t.TempDir()}
			if tc.configured {
				if err := testutil.WriteFile(paths.State+"/configured", []byte("1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			c := CLI{Root: t.TempDir(), Out: &out, Err: io.Discard}
			packages := []Package{{"macos", "optional", "cask", terminalFontCask}}
			if err := c.installPackages(context.Background(), f.op, paths, packages, tc.platform, "arm64", tc.ghostty, false); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); tc.want == "" && strings.Contains(got, terminalFont) || !strings.Contains(got, tc.want) {
				t.Fatalf("want hint %q, output %q", tc.want, got)
			}
		})
	}
}

func TestTerminalFontHintNamesAPackagedCask(t *testing.T) {
	packages, err := ReadPackages("../../packages.conf")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(packages, Package{"macos", "optional", "cask", terminalFontCask}) {
		t.Fatalf("packages.conf does not install %s", terminalFontCask)
	}
}
