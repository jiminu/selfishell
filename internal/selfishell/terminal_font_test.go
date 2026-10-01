package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestTerminalFontHintFollowsFirstCaskInstall(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		installed, ghostty, wanted bool
	}{
		{name: "new install", wanted: true},
		{name: "already installed", installed: true},
		{name: "ghostty chosen", ghostty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackageFixture(t)
			casks := ""
			if tc.installed {
				casks = terminalFontCask
			}
			f.executable("brew", fmt.Sprintf(`case "$1 $2" in 'list --cask') printf '%%s\n' %q;; esac`, casks))
			var out bytes.Buffer
			c := CLI{Root: t.TempDir(), Out: &out, Err: io.Discard}
			packages := []Package{{"macos", "optional", "cask", terminalFontCask}}
			if err := c.installPackages(context.Background(), f.op, Paths{}, packages, "macos", "arm64", tc.ghostty, false); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out.String(), "Set your terminal font to "+terminalFont+" to show Neovim's icons."); got != tc.wanted {
				t.Fatalf("hint %t, output %q", got, out.String())
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
