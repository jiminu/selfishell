package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProgressCapturesToolsButPreservesQueriesAndInput(t *testing.T) {
	home := t.TempDir()
	isolateHome(t, home)
	var out, stderr bytes.Buffer
	state := filepath.Join(home, "state")
	ui := newProgress(&out, &stderr, Paths{State: state})
	ui.compact = true
	p := Process{Out: &out, Err: &stderr, progress: ui}
	ui.stage("Synchronizing tools")
	code, err := p.Run(context.Background(), "/bin/sh", "-c", "printf 'successful tool chatter\\n'; printf 'tool detail\\n' >&2")
	ui.pause()
	if err != nil || code != 0 || strings.Contains(out.String()+stderr.String(), "tool chatter") {
		t.Fatalf("capture: %d %v %q %q", code, err, out.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(state, "logs")); !os.IsNotExist(err) {
		t.Fatalf("successful command created logs: %v", err)
	}
	var query bytes.Buffer
	q := p
	q.Out = &query
	code, err = q.Run(context.Background(), "/bin/sh", "-c", "printf 'machine-readable'")
	if code != 0 || err != nil || query.String() != "machine-readable" {
		t.Fatalf("query redirected: %q %d %v", query.String(), code, err)
	}
	ui.stage("Interactive installer")
	p.foreground = true
	p.In = strings.NewReader("answer\n")
	code, err = p.Run(context.Background(), "/bin/sh", "-c", "printf 'Password: '; read reply; printf 'received %s\\n' \"$reply\"")
	if code != 0 || err != nil || !strings.Contains(out.String(), "Password: received answer") || ui.stop != nil {
		t.Fatalf("input hidden: %d %v %q", code, err, out.String())
	}
	p.foreground = false
	code, err = p.Run(context.Background(), "/bin/sh", "-c", "printf 'failure detail\\n' >&2; exit 7")
	if code != 7 || err != nil || !strings.Contains(stderr.String(), "failure detail") || strings.Contains(stderr.String(), "successful tool chatter") {
		t.Fatalf("failure output: %d %v %q", code, err, stderr.String())
	}
	ui.finish()
	if strings.Contains(out.String(), "Log:") {
		t.Fatalf("summary advertised a log: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(state, "logs")); !os.IsNotExist(err) {
		t.Fatalf("failed command created logs: %v", err)
	}
}

func TestProgressSummarizesOnlyCompletedActionsOnFailure(t *testing.T) {
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, Paths{State: t.TempDir()})
	ui.compact = true
	c := CLI{Out: &out, Err: &stderr, progress: ui}
	c.report("Configuration", reportSuccess, "Updated managed file: example.zsh")
	if out.Len() != 0 {
		t.Fatal("action printed before completion")
	}
	c.error("remaining installation failed")
	ui.finish()
	if !strings.Contains(out.String(), "Completed changes before stopping:") || !strings.Contains(out.String(), "example.zsh") || strings.Contains(out.String(), "✓") {
		t.Fatalf("misleading failure summary: %q", out.String())
	}
	if !strings.Contains(stderr.String(), "remaining installation failed") {
		t.Fatal(stderr.String())
	}
}

func TestProgressPlainOutputDoesNotCreateLogs(t *testing.T) {
	state := filepath.Join(t.TempDir(), "absent")
	var out bytes.Buffer
	ui := newProgress(&out, &out, Paths{State: state})
	c := CLI{Out: &out, Err: &out, progress: ui}
	ui.stage("Applying configuration")
	c.report("Configuration", reportSuccess, "Updated managed file: example.zsh")
	c.complete("Installation complete.")
	ui.finish()
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "Updated managed file: example.zsh") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("plain output wrote state: %v", err)
	}
}

func TestProgressIgnoresOccupiedLogPath(t *testing.T) {
	isolateHome(t, t.TempDir())
	var out, stderr bytes.Buffer
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "logs"), []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	ui := newProgress(&out, &stderr, Paths{State: state})
	ui.compact = true
	defer ui.finish()
	ui.stage("Synchronizing tools")
	code, err := (Process{Out: &out, Err: &stderr, progress: ui}).Run(context.Background(), "/bin/sh", "-c", "printf successful")
	if code != 0 || err != nil || strings.Contains(out.String()+stderr.String(), "successful") {
		t.Fatalf("successful output should stay hidden: %d %v %q %q", code, err, out.String(), stderr.String())
	}
	code, err = (Process{Out: &out, Err: &stderr, progress: ui}).Run(context.Background(), "/bin/sh", "-c", "printf 'failure detail\\n' >&2; exit 7")
	if code != 7 || err != nil || !strings.Contains(stderr.String(), "failure detail") || strings.Contains(stderr.String(), "Could not open operation log") {
		t.Fatalf("failure output: %d %v %q", code, err, stderr.String())
	}
	data, _ := os.ReadFile(filepath.Join(state, "logs"))
	if string(data) != "user data" {
		t.Fatal("occupied log path modified")
	}
	op := PackageOperation{Process: Process{Out: &out, Err: &stderr, progress: ui}}
	_, err = op.runNvim(context.Background(), "", "/bin/sh", "", "-c", "printf 'nvim failed\\n' >&2; exit 1")
	if err == nil || strings.Count(stderr.String(), "nvim failed") != 1 {
		t.Fatalf("Neovim error must appear once: %v %q", err, stderr.String())
	}
}

func TestProgressFailureOutputIsBounded(t *testing.T) {
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, Paths{State: t.TempDir()})
	ui.compact = true
	defer ui.finish()
	ui.stage("Synchronizing tools")
	code, err := (Process{Out: &out, Err: &stderr, progress: ui}).Run(context.Background(), "/bin/sh", "-c", "head -c 16384 /dev/zero | tr '\\000' x; printf 'failure detail\\n' >&2; exit 7")
	if code != 7 || err != nil || !strings.HasSuffix(stderr.String(), "failure detail\n") || len(stderr.String()) > 8192 {
		t.Fatalf("unbounded failure output: %d %v %d %q", code, err, stderr.Len(), stderr.String())
	}
}

func TestConfigurationProgressCancelsBlockedTrust(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockWrite(t, home+"/tools/mise", []byte("#!/bin/sh\nexec /bin/sleep 2\n"))
	if err := os.Chmod(home+"/tools/mise", 0700); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, paths)
	ui.compact = true
	defer ui.finish()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	c := CLI{Root: root, Out: &out, Err: &stderr, Context: ctx, progress: ui}
	p, err := c.prepareConfig("macos", false, true, true)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = c.applyManagedResources(&p)
	if err == nil || time.Since(start) > time.Second || ui.stop != nil {
		t.Fatalf("uncancellable config phase: %v after %s spinner=%v", err, time.Since(start), ui.stop)
	}
}

func TestProgressStopsAnimationAfterCancellation(t *testing.T) {
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, Paths{State: t.TempDir()})
	ui.compact = true
	defer ui.finish()
	ui.stage("Synchronizing tools")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	code, err := (Process{Out: &out, Err: &stderr, progress: ui}).Run(ctx, "/bin/sleep", "10")
	if code != 130 || err == nil || ui.stop != nil {
		t.Fatalf("cancellation: %d %v spinner=%v", code, err, ui.stop)
	}
}

func TestProgressLSPAndPluginSuccessUseLabelColor(t *testing.T) {
	op, paths, root, manifest, _, _ := neovimFixture(t)
	out := op.Process.Out.(*bytes.Buffer)
	ui := newProgress(out, op.Process.Err, paths)
	ui.compact, ui.color = true, true
	op.Process.progress = ui
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := op.UpdateDefaultLSP(context.Background(), root, paths, false); err != nil {
		t.Fatal(err)
	}
	ui.finish()
	for _, want := range []string{
		"\x1b[32mSynchronized Neovim plugins:\x1b[0m approved revisions",
		"\x1b[32mNeovim LSP servers:\x1b[0m approved versions",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("success label was not colored: want %q, got %q", want, out.String())
		}
	}
}

func TestProgressMiseListWrapsWithoutLosingPins(t *testing.T) {
	op, paths, root, _, home, _ := neovimFixture(t)
	writeTestFile(t, home+"/bin/mise", "#!/bin/sh\ncase \"$*\" in *--dry-run-code*) exit 1 ;; esac\n", 0755)
	out := op.Process.Out.(*bytes.Buffer)
	ui := newProgress(out, op.Process.Err, paths)
	ui.compact = true
	op.Process.progress = ui
	names := []string{"starship@1.26.0", "fzf@0.74.4", "zoxide@0.10.0", "ripgrep@15.2.0", "jq@1.8.2", "neovim@0.12.5", "tree-sitter@0.27.0", "node@24.18.0"}
	if err := op.InstallMise(context.Background(), root, paths, "required", false, names...); err != nil {
		t.Fatal(err)
	}
	ui.finish()
	if !strings.Contains(out.String(), "Synchronized mise tools: 8\n") {
		t.Fatalf("missing list count: %q", out.String())
	}
	for _, name := range names {
		if strings.Count(out.String(), name) != 1 {
			t.Errorf("pin lost or repeated: %s in %q", name, out.String())
		}
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "    ") && len(line) > 80 {
			t.Errorf("line exceeds fallback terminal width: %q", line)
		}
	}
}

func TestProgressListWrapBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		words []string
		width int
		want  string
	}{
		{"empty", nil, 20, ""},
		{"fits exactly", []string{"a@1", "bb@22"}, 10, "a@1  bb@22"},
		{"narrow", []string{"a@1", "bb@22", "c@3"}, 9, "a@1\nbb@22\nc@3"},
		{"oversized item", []string{"long-tool@123", "a@1"}, 4, "long-tool@123\na@1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wrapWords(tc.words, tc.width); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProgressShortensHomePaths(t *testing.T) {
	home := t.TempDir()
	isolateHome(t, home)
	var out bytes.Buffer
	ui := newProgress(&out, &out, Paths{})
	ui.compact = true
	c := CLI{Out: &out, Err: &out, progress: ui}
	c.report("Configuration", reportSuccess, "Updated managed file: %s", home+"/.zshrc")
	ui.finish()
	if !strings.Contains(out.String(), "Updated managed file: ~/.zshrc") || strings.Contains(out.String(), home) {
		t.Fatalf("long home prefix: %q", out.String())
	}
}
