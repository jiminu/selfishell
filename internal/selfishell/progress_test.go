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

func TestProgressPrintsNeovimErrorOnce(t *testing.T) {
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, Paths{State: t.TempDir()})
	ui.compact = true
	defer ui.finish()
	ui.stage("Synchronizing tools")
	op := PackageOperation{Process: Process{Out: &out, Err: &stderr, progress: ui}}
	_, err := op.runNvim(context.Background(), "", "/bin/sh", "", "-c", "printf 'nvim failed\\n' >&2; exit 1")
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
