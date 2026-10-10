package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// CI supplies the approved mise binary explicitly. Keep this integration with its
// real generated hooks covered when dependency automation changes that pin.
func TestNativeRealMiseWSLFirstPrompt(t *testing.T) {
	t.Parallel()
	mise := os.Getenv("SELFISHELL_TEST_PINNED_MISE")
	if mise == "" {
		t.Skip("requires SELFISHELL_TEST_PINNED_MISE pointing to the approved mise binary")
	}
	if !filepath.IsAbs(mise) {
		t.Fatal("SELFISHELL_TEST_PINNED_MISE must be an absolute path")
	}
	for _, tc := range []struct {
		name, distro, beforePrompt string
		wantCalls                  int
	}{
		{"wsl", "Ubuntu-24.04", ":", 0},
		{"user_path_change", "Ubuntu-24.04", `path=("$HOME/custom/bin" $path)`, 1},
		{"mise_env_change", "Ubuntu-24.04", `export MISE_LOG_LEVEL=error`, 1},
		{"ubuntu", "", ":", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := nativeHome(t)
			mustFS(t, os.MkdirAll(filepath.Join(home, ".local/bin"), 0700))
			mustFS(t, os.Symlink(mise, filepath.Join(home, ".local/bin/mise")))
			mustFS(t, os.MkdirAll(filepath.Join(home, ".config/selfishell"), 0700))
			mustFS(t, os.Symlink(filepath.Join(repoRoot(), "config/shared/zsh"), filepath.Join(home, ".config/selfishell/zsh")))
			for _, version := range []string{"22.0.0", "24.0.0"} {
				nativeWrite(t, filepath.Join(home, ".local/share/mise/installs/node", version, "bin/node"), "#!/bin/sh\nprintf '%s\\n' '"+version+"'\n", 0700)
			}
			nativeWrite(t, filepath.Join(home, ".config/mise/config.toml"), "[tools]\nnode = '22.0.0'\n[env]\nSELFISHELL_TEST_PROJECT = 'global'\n", 0600)
			nativeWrite(t, filepath.Join(home, "project with spaces/mise.toml"), "[tools]\nnode = '24.0.0'\n[env]\nSELFISHELL_TEST_PROJECT = 'project'\n", 0600)
			probe := `source "$SELFISHELL_SOURCE"
[[ "$(node)" == 22.0.0 && "$SELFISHELL_TEST_PROJECT" == global ]] || exit 10
(( ${precmd_functions[(Ie)_mise_hook_precmd]} && ${chpwd_functions[(Ie)_mise_hook_chpwd]} )) || exit 11
functions[_selfishell_original_mise_hook]=$functions[_mise_hook]
typeset -gi hook_calls=0
_mise_hook() { (( ++hook_calls )); _selfishell_original_mise_hook "$@"; }
` + tc.beforePrompt + `
_mise_hook_precmd
print -r -- "$hook_calls"
_mise_hook_precmd
[[ "$(node)" == 22.0.0 ]] || exit 12
cd "$HOME/project with spaces" || exit 13
[[ "$(node)" == 24.0.0 && "$SELFISHELL_TEST_PROJECT" == project ]] || exit 14
_mise_hook_precmd
cd "$HOME" || exit 15
[[ "$(node)" == 22.0.0 && "$SELFISHELL_TEST_PROJECT" == global ]] || exit 16
[[ ${path[(Ie)/mnt/c/Windows]} -gt 0 && ${path[(Ie)/mnt/c/Program Files/tools]} -gt 0 ]] || exit 17
`
			r := nativeRun(t, home, probe,
				"PATH=/usr/bin:/mnt/c/Windows:/bin:/mnt/c/Program Files/tools", "WSL_DISTRO_NAME="+tc.distro,
				"MISE_OFFLINE=1", "MISE_CEILING_PATHS="+home, "MISE_TRUSTED_CONFIG_PATHS="+home,
				"SELFISHELL_UPDATE_NOTICE=0", "SELFISHELL_SOURCE="+filepath.Join(repoRoot(), "config/ubuntu/zshrc"))
			if string(r.Stdout) != fmt.Sprintf("%d\n", tc.wantCalls) || len(r.Stderr) != 0 {
				t.Fatalf("first prompt hook calls: want %d, got stdout=%q stderr=%q", tc.wantCalls, r.Stdout, r.Stderr)
			}
		})
	}
}
