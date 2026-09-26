package migration_test

import (
	"fmt"

	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func TestNativeHistoryConfiguration(t *testing.T) {
	home := nativeHome(t)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; print -rl -- "file=$HISTFILE" "sizes=$HISTSIZE,$SAVEHIST"; for option in EXTENDED_HISTORY INC_APPEND_HISTORY_TIME HIST_IGNORE_SPACE HIST_REDUCE_BLANKS; do print -r -- "option=$option:$options[$option]"; done; /bin/sh -c 'printf "export=%s\n" "$HISTFILE"'`, "SELFISHELL_SOURCE="+filepath.Join(repoRoot(), "config/shared/zsh/history.zsh"))
	lines := strings.Split(strings.TrimSpace(string(r.Stdout)), "\n")
	if len(lines) != 7 {
		t.Fatalf("history observations: %q", r.Stdout)
	}
	if lines[0] != "file="+filepath.Join(home, ".zsh_history") {
		t.Fatalf("HISTFILE %q", lines[0])
	}
	var hist, save int
	if _, err := fmt.Sscanf(lines[1], "sizes=%d,%d", &hist, &save); err != nil || hist <= 0 || save <= 0 {
		t.Fatalf("sizes %q: %v", lines[1], err)
	}
	for i, opt := range []string{"EXTENDED_HISTORY", "INC_APPEND_HISTORY_TIME", "HIST_IGNORE_SPACE", "HIST_REDUCE_BLANKS"} {
		if lines[i+2] != "option="+opt+":on" {
			t.Fatalf("option %q", lines[i+2])
		}
	}
	if lines[6] != "export=" {
		t.Fatalf("unexpected history export %q", lines[6])
	}
}
func TestNativeHistoryManagedResource(t *testing.T) {
	home := nativeHome(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	resources, err := selfishell.ResourcesForPlatform(repoRoot(), "macos", false)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(home, ".config/selfishell/zsh/history.zsh")
	wantSource := filepath.Join(repoRoot(), "config/shared/zsh/history.zsh")
	found := false
	for _, r := range resources {
		if r.Name == "zsh-history" {
			if r.Kind != "file" || r.Target != wantTarget || r.Source != wantSource {
				t.Fatalf("history descriptor: %+v", r)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("history resource absent")
	}
}
