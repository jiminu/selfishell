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
	ui := newProgress(&out, &stderr, Paths{State: filepath.Join(home, "state")})
	ui.compact = true
	defer ui.finish()
	p := Process{Out: &out, Err: &stderr, progress: ui}
	ui.stage("Synchronizing tools")
	code, err := p.Run(context.Background(), "/bin/sh", "-c", "printf 'successful tool chatter\\n'; printf 'tool detail\\n' >&2")
	ui.pause()
	if err != nil || code != 0 || strings.Contains(out.String()+stderr.String(), "tool chatter") {
		t.Fatalf("capture: %d %v %q %q", code, err, out.String(), stderr.String())
	}
	if ui.log == nil {
		t.Fatal("no log")
	}
	data, err := os.ReadFile(ui.log.Name())
	if err != nil || !strings.Contains(string(data), "successful tool chatter") || !strings.Contains(string(data), "tool detail") {
		t.Fatalf("log: %q %v", data, err)
	}
	if info, err := ui.log.Stat(); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("log must be private: %v %v", info, err)
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
}

func TestProgressSummarizesOnlyCompletedActionsOnFailure(t *testing.T) {
	var out, stderr bytes.Buffer
	ui := newProgress(&out, &stderr, Paths{State: t.TempDir()})
	ui.compact = true
	c := CLI{Out: &out, Err: &stderr, progress: ui}
	c.report("Configuration", "Updated managed file: example.zsh")
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
	c.report("Configuration", "Updated managed file: example.zsh")
	c.complete("Installation complete.")
	ui.finish()
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "Updated managed file: example.zsh") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("plain output wrote state: %v", err)
	}
}

func TestProgressLogFailureFallsBackToVisibleToolOutput(t *testing.T) {
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
	code, err := (Process{Out: &out, Err: &stderr, progress: ui}).Run(context.Background(), "/bin/sh", "-c", "printf visible")
	if code != 0 || err != nil || !strings.Contains(out.String(), "visible") || !strings.Contains(stderr.String(), "Could not open operation log") || ui.stop != nil {
		t.Fatalf("hidden output without a log: %d %v %q %q", code, err, out.String(), stderr.String())
	}
	ui.stage("Next tool")
	if ui.stop != nil {
		t.Fatal("spinner restarted over fallback tool output")
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
