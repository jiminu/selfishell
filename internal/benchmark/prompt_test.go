package benchmark

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSummarizeNearestRanks(t *testing.T) {
	got, err := Summarize([]float64{9, 1, 5, 3, 7})
	if err != nil {
		t.Fatal(err)
	}
	if got != (Stats{Mean: 5, P50: 5, P95: 9, Max: 9}) {
		t.Fatalf("stats = %+v", got)
	}
	if _, err := Summarize(nil); err == nil {
		t.Fatal("empty samples accepted")
	}
	ordered := make([]float64, 20)
	for i := range ordered {
		ordered[i] = float64(i + 1)
	}
	got, err = Summarize(ordered)
	if err != nil || got != (Stats{Mean: 10.5, P50: 10, P95: 19, Max: 20}) {
		t.Fatalf("20 samples: %+v, %v", got, err)
	}
}

func TestPromptEnvironmentIsPrivate(t *testing.T) {
	home := t.TempDir()
	env := PromptEnvironment("/release", home, filepath.Join(home, "zdot"), "/release/config/zshrc", "/usr/bin:/bin", []string{"WSL_DISTRO_NAME=Ubuntu", "VIRTUAL_ENV=/ambient", "MISE_DATA_DIR=/ambient/mise", "LANG=C.UTF-8"})
	values := make(map[string]string)
	for _, pair := range env {
		key, value, _ := strings.Cut(pair, "=")
		values[key] = value
	}
	for key, want := range map[string]string{
		"HOME": home, "WSL_DISTRO_NAME": "Ubuntu", "LANG": "C.UTF-8",
		"MISE_DATA_DIR":            filepath.Join(home, ".local/share/mise"),
		"MISE_CACHE_DIR":           filepath.Join(home, ".cache/mise"),
		"MISE_STATE_DIR":           filepath.Join(home, ".local/state/mise"),
		"MISE_GLOBAL_CONFIG_FILE":  filepath.Join(home, ".config/mise/config.toml"),
		"SELFISHELL_UPDATE_NOTICE": "0", "STARSHIP_CONFIG": "/release/config/shared/starship.toml",
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
	if _, ok := values["VIRTUAL_ENV"]; ok {
		t.Fatal("ambient virtual environment leaked")
	}
}

func TestMeasurePromptRealZsh(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh unavailable")
	}
	home := t.TempDir()
	zdot := filepath.Join(home, "zdot")
	if err := os.Mkdir(zdot, 0700); err != nil {
		t.Fatal(err)
	}
	fixture := `setopt promptsubst
typeset -gi count=0
precmd() { (( ++count )); }
RPROMPT='__SFS_READY_${count}__'
zshexit() { print finished >> "$HOME/finished"; }
[[ "$MISE_CEILING_PATHS" == "$PWD" ]] || exit 2
[[ "$WSL_DISTRO_NAME" == Ubuntu ]] || exit 3
[[ -z "$VIRTUAL_ENV" ]] || exit 4
[[ "$MISE_DATA_DIR" == "$HOME/.local/share/mise" ]] || exit 5
[[ "$SELFISHELL_UPDATE_NOTICE" == 0 ]] || exit 6
`
	if err := os.WriteFile(filepath.Join(zdot, ".zshrc"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	env := PromptEnvironment("/release", home, zdot, "/release/zshrc", "/usr/bin:/bin", []string{"WSL_DISTRO_NAME=Ubuntu", "VIRTUAL_ENV=/ambient"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := MeasurePrompt(ctx, home, env, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.First) != 1 || len(got.Command) != 3 {
		t.Fatalf("sample shape = %d,%d", len(got.First), len(got.Command))
	}
	for _, v := range append(got.First, got.Command...) {
		if v <= 0 {
			t.Fatalf("non-positive sample %f", v)
		}
	}
	data, err := os.ReadFile(filepath.Join(home, "finished"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(strings.Fields(string(data)), []string{"finished", "finished"}) {
		t.Fatalf("zshexit evidence: %q", data)
	}
}

func TestPromptFromSessionLeader(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh unavailable")
	}
	if home := os.Getenv("SELFISHELL_PTY_SESSION_HELPER"); home != "" {
		env := PromptEnvironment("/release", home, home, "/release/zshrc", "/usr/bin:/bin", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := MeasurePrompt(ctx, home, env, 1); err != nil {
			t.Fatal(err)
		}
		return
	}
	home := t.TempDir()
	config := "setopt promptsubst\ntypeset -gi count=0\nprecmd() { (( ++count )); }\nRPROMPT='__SFS_READY_${count}__'\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestPromptFromSessionLeader$")
	cmd.Env = append(os.Environ(), "SELFISHELL_PTY_SESSION_HELPER="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("session-leading prompt: %v: %s", err, out)
	}
}

func TestWaitForPromptRejectsOldMarkerAndClosedShell(t *testing.T) {
	chunks := make(chan readResult, 4)
	chunks <- readResult{data: []byte("__SFS_READY_1__ __SFS_RE")}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	if err := waitForPrompt(ctx, chunks, 2); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("old marker: %v", err)
	}
	close(chunks)
	if err := waitForPrompt(context.Background(), chunks, 3); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("closed shell: %v", err)
	}
	parts := make(chan readResult, 2)
	parts <- readResult{data: []byte("__SFS_RE")}
	parts <- readResult{data: []byte("ADY_2__")}
	if err := waitForPrompt(context.Background(), parts, 2); err != nil {
		t.Fatalf("split marker: %v", err)
	}
	failed := make(chan readResult, 1)
	failed <- readResult{err: syscall.EIO}
	if err := waitForPrompt(context.Background(), failed, 4); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("EIO: %v", err)
	}
}

func TestMeasurePromptCancelCleansDescendants(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh unavailable")
	}
	home := t.TempDir()
	fixture := `precmd() { (/bin/sleep 2; /usr/bin/touch "$HOME/escaped") & /bin/sleep 5; }` + "\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	env := PromptEnvironment("/release", home, home, "/release/zshrc", "/usr/bin:/bin", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := MeasurePrompt(ctx, home, env, 1)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("cancel behavior: %v, elapsed %s", err, time.Since(start))
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(home, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}
