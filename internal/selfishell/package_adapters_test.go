package selfishell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type packageFixture struct {
	t         *testing.T
	home, bin string
	out, err  bytes.Buffer
	op        *PackageOperation
}

func newPackageFixture(t *testing.T) *packageFixture {
	t.Helper()
	root := t.TempDir()
	f := &packageFixture{t: t, home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin")}
	for _, p := range []string{f.home, f.bin} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + f.home, "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_STATE_HOME=" + filepath.Join(root, "state"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "PATH=" + f.bin}
	f.op = &PackageOperation{Process: Process{Out: &f.out, Err: &f.err, Env: env}}
	return f
}

func (f *packageFixture) executable(name, body string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		f.t.Fatal(err)
	}
}

func (f *packageFixture) calls() string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.home, "calls"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *packageFixture) apt() {
	f.executable("dpkg-query", `printf 'dpkg %s\n' "$3" >>"$HOME/calls"
case "$3" in installed|second) echo 'install ok installed';; held) echo 'hold ok installed';; removed) echo 'deinstall ok config-files';; *) exit 1;; esac`)
	f.executable("apt-cache", `printf 'cache %s\n' "$2" >>"$HOME/calls"
case "$2" in available|removed|first) exit 0;; *) exit 1;; esac`)
	f.executable("apt-get", `printf 'apt %s\n' "$*" >>"$HOME/calls"
case "$1" in update) [ ! -f "$HOME/fail-update" ];; install) [ ! -f "$HOME/fail-install" ];; esac`)
	f.executable("sudo", `printf 'sudo %s\n' "$*" >>"$HOME/calls"
shift
"$PATH/apt-get" "$@"`)
	f.op.uid = func() int { return 1000 }
}

func (f *packageFixture) brew() {
	f.executable("brew", `printf 'brew %s no_ask=%s\n' "$*" "$HOMEBREW_NO_ASK" >>"$HOME/calls"
case "$1 $2" in 'list --formula') printf 'installed\nfirst\nsecond\n';; 'list --cask') printf 'first\nsecond\n';; esac
if [ "$1" = install ] && [ -f "$HOME/fail-install" ]; then exit 1; fi`)
}

func (f *packageFixture) flag(name string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.home, name), nil, 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *packageFixture) skipped(want ...string) {
	f.t.Helper()
	if !reflect.DeepEqual(f.op.SkippedOptional, want) {
		f.t.Fatalf("skipped %q want %q", f.op.SkippedOptional, want)
	}
}

func TestAptReinstallsRemovedButNotPurgedPackage(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	if err := f.op.InstallApt(context.Background(), "required", false, "removed", "second"); err != nil {
		t.Fatal(err)
	}
	if got := f.calls(); !strings.Contains(got, "apt install -y removed\n") || strings.Contains(got, "apt install -y second") {
		t.Fatal(got)
	}
}

func TestAptLeavesInstalledAndHeldPackagesQuiet(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	if err := f.op.InstallApt(context.Background(), "required", false, "installed", "second", "held"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.calls(), "apt ") || f.out.Len() != 0 || f.err.Len() != 0 {
		t.Fatalf("calls %q out %q err %q", f.calls(), f.out.String(), f.err.String())
	}
}

func TestAptNonRootRequiresSudo(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	if err := f.op.InstallApt(context.Background(), "required", false, "available"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f.calls(), "sudo apt-get "); got != 2 {
		t.Fatalf("sudo count %d: %q", got, f.calls())
	}
}

func TestAptNonRootWithoutSudoFails(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	os.Remove(filepath.Join(f.bin, "sudo"))
	if err := f.op.InstallApt(context.Background(), "required", false, "available"); err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("error %v", err)
	}
	if strings.Contains(f.calls(), "apt update") {
		t.Fatal(f.calls())
	}
}

func TestAptRootDoesNotRequireSudo(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	f.op.uid = func() int { return 0 }
	os.Remove(filepath.Join(f.bin, "sudo"))
	if err := f.op.InstallApt(context.Background(), "required", false, "available"); err != nil {
		t.Fatal(err)
	}
	if got := f.calls(); !strings.Contains(got, "apt install -y available\n") || strings.Contains(got, "sudo ") {
		t.Fatal(got)
	}
}

func TestAptInstallsAvailableOptionalAndSkipsUnavailable(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	if err := f.op.InstallApt(context.Background(), "optional", false, "available", "missing"); err != nil {
		t.Fatal(err)
	}
	f.skipped("missing")
	if got := f.calls(); !strings.Contains(got, "apt install -y available\n") || strings.Contains(got, "apt install -y missing") {
		t.Fatal(got)
	}
	if !strings.Contains(f.err.String(), "missing") {
		t.Fatal(f.err.String())
	}
}

func TestAptRequiredUnavailableFails(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	if err := f.op.InstallApt(context.Background(), "required", false, "missing"); err == nil {
		t.Fatal("required unavailable succeeded")
	}
	if strings.Contains(f.calls(), "apt install") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewLeavesInstalledFormulaAndCaskQuiet(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	for _, tc := range []struct{ requirement, manager string }{{"required", "formula"}, {"optional", "cask"}} {
		if err := f.op.InstallHomebrew(context.Background(), tc.requirement, tc.manager, false, "first", "second"); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(f.calls(), "brew install") || f.out.Len() != 0 || f.err.Len() != 0 {
		t.Fatalf("calls %q out %q err %q", f.calls(), f.out.String(), f.err.String())
	}
}

func TestHomebrewOptionalFailureReported(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	f.flag("fail-install")
	if err := f.op.InstallHomebrew(context.Background(), "optional", "formula", false, "optional-tool"); err != nil {
		t.Fatal(err)
	}
	f.skipped("optional-tool")
	if !strings.Contains(f.err.String(), "optional-tool") {
		t.Fatal(f.err.String())
	}
}

func TestHomebrewInstallsOnlyMissingFormulae(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	if err := f.op.InstallHomebrew(context.Background(), "required", "formula", false, "installed", "missing"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls(), "brew install missing no_ask=1\n") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewSuppressesDuplicateConfirmation(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	if err := f.op.InstallHomebrew(context.Background(), "required", "formula", false, "required-tool"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls(), "brew install required-tool no_ask=1\n") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewRequiredFailureFails(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	f.flag("fail-install")
	if err := f.op.InstallHomebrew(context.Background(), "required", "cask", false, "required-tool"); err == nil {
		t.Fatal("required cask failure succeeded")
	}
	if !strings.Contains(f.calls(), "brew install --cask required-tool no_ask=1\n") {
		t.Fatal(f.calls())
	}
}

func TestPackageAdaptersEmptyAndDryRunDoNotExecute(t *testing.T) {
	f := newPackageFixture(t)
	for _, err := range []error{f.op.InstallApt(context.Background(), "required", false), f.op.InstallHomebrew(context.Background(), "optional", "cask", false), f.op.InstallApt(context.Background(), "required", true, "x"), f.op.InstallHomebrew(context.Background(), "required", "formula", true, "x")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.calls() != "" || len(f.op.SkippedOptional) != 0 {
		t.Fatal(f.calls(), f.op.SkippedOptional)
	}
	if !strings.Contains(f.out.String(), "Would install required apt packages: x") || !strings.Contains(f.out.String(), "Would install required Homebrew formula: x") {
		t.Fatal(f.out.String())
	}
}

func TestAptUpdatesOncePerOperationAndRetriesAfterOptionalFailure(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	f.flag("fail-update")
	if err := f.op.InstallApt(context.Background(), "optional", false, "available", "installed"); err != nil {
		t.Fatal(err)
	}
	f.skipped("available")
	if strings.Contains(f.calls(), "apt install") {
		t.Fatal(f.calls())
	}
	if err := os.Remove(filepath.Join(f.home, "fail-update")); err != nil {
		t.Fatal(err)
	}
	if err := f.op.InstallApt(context.Background(), "required", false, "removed"); err != nil {
		t.Fatal(err)
	}
	if err := f.op.InstallApt(context.Background(), "required", false, "first"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(f.calls(), "apt update\n"); got != 2 {
		t.Fatalf("update count %d: %q", got, f.calls())
	}
}

func TestAptRequiredIndexFailureIsFatal(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	f.flag("fail-update")
	if err := f.op.InstallApt(context.Background(), "required", false, "available"); err == nil {
		t.Fatal("required update failure succeeded")
	}
	if strings.Contains(f.calls(), "apt install") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewInventoriesOncePerKindPerOperation(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	for _, manager := range []string{"formula", "formula", "cask", "cask"} {
		if err := f.op.InstallHomebrew(context.Background(), "optional", manager, false, "missing"); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(f.calls(), "brew list --formula"); got != 1 {
		t.Fatalf("formula queries %d: %q", got, f.calls())
	}
	if got := strings.Count(f.calls(), "brew list --cask"); got != 1 {
		t.Fatalf("cask queries %d: %q", got, f.calls())
	}
	if got := strings.Count(f.calls(), "brew install missing no_ask=1"); got != 1 {
		t.Fatalf("formula installs %d: %q", got, f.calls())
	}
	if got := strings.Count(f.calls(), "brew install --cask missing no_ask=1"); got != 1 {
		t.Fatalf("cask installs %d: %q", got, f.calls())
	}
}

func TestHomebrewOptionalMissingDoesNotBootstrap(t *testing.T) {
	f := newPackageFixture(t)
	if err := f.op.InstallHomebrew(context.Background(), "optional", "formula", false, "missing"); err != nil {
		t.Fatal(err)
	}
	f.skipped("missing")
	if f.calls() != "" {
		t.Fatal(f.calls())
	}
}

func TestAptOptionalInstallFailureIsSkipped(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	f.flag("fail-install")
	if err := f.op.InstallApt(context.Background(), "optional", false, "available", "installed"); err != nil {
		t.Fatal(err)
	}
	f.skipped("available")
	if !strings.Contains(f.err.String(), "available") {
		t.Fatal(f.err.String())
	}
}

func TestAptOptionalMissingSudoIsSkipped(t *testing.T) {
	f := newPackageFixture(t)
	f.apt()
	os.Remove(filepath.Join(f.bin, "sudo"))
	if err := f.op.InstallApt(context.Background(), "optional", false, "available", "installed"); err != nil {
		t.Fatal(err)
	}
	f.skipped("available")
	if strings.Contains(f.calls(), "apt update") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewActivatesStandardLocationAfterBootstrap(t *testing.T) {
	f := newPackageFixture(t)
	standard := filepath.Join(t.TempDir(), "bin", "brew")
	if err := os.MkdirAll(filepath.Dir(standard), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(standard, []byte("#!/bin/sh\nprintf 'activated %s no_ask=%s\\n' \"$*\" \"$HOMEBREW_NO_ASK\" >>\"$HOME/calls\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	f.op.brewLocations = []string{standard}
	f.executable("curl", `printf 'curl %s\n' "$*" >>"$HOME/calls"; printf ':\n'`)
	if err := f.op.InstallHomebrew(context.Background(), "required", "formula", false, "needed"); err != nil {
		t.Fatal(err)
	}
	if got := f.calls(); !strings.Contains(got, "curl -fsSL") || !strings.Contains(got, "activated install needed no_ask=1") {
		t.Fatal(got)
	}
}

func TestHomebrewInventoryCacheDoesNotTreatPrefixesAsInstalled(t *testing.T) {
	f := newPackageFixture(t)
	f.brew()
	if err := f.op.InstallHomebrew(context.Background(), "required", "formula", false, "first-extra"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls(), "brew install first-extra no_ask=1") {
		t.Fatal(f.calls())
	}
}

func TestHomebrewDiscardsPartialFailedInventory(t *testing.T) {
	f := newPackageFixture(t)
	f.executable("brew", `printf 'brew %s\n' "$*" >>"$HOME/calls"
case "$1 $2" in 'list --formula') printf 'needed\n'; exit 1;; esac`)
	if err := f.op.InstallHomebrew(context.Background(), "required", "formula", false, "needed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls(), "brew install needed\n") {
		t.Fatal(f.calls())
	}
}

func TestPackageAdaptersAlreadyCancelledBeforeMissingTools(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*packageFixture)
		install func(*PackageOperation, context.Context) error
	}{
		{"apt-get missing", func(*packageFixture) {}, func(o *PackageOperation, ctx context.Context) error {
			return o.InstallApt(ctx, "optional", false, "needed")
		}},
		{"brew missing", func(*packageFixture) {}, func(o *PackageOperation, ctx context.Context) error {
			return o.InstallHomebrew(ctx, "optional", "formula", false, "needed")
		}},
		{"sudo missing", func(f *packageFixture) { f.apt(); os.Remove(filepath.Join(f.bin, "sudo")) }, func(o *PackageOperation, ctx context.Context) error {
			return o.InstallApt(ctx, "optional", false, "needed")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackageFixture(t)
			tc.prepare(f)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tc.install(f.op, ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want context.Canceled", err)
			}
			if f.calls() != "" || len(f.op.SkippedOptional) != 0 || f.out.Len() != 0 || f.err.Len() != 0 {
				t.Fatalf("cancelled operation had effects: calls=%q skipped=%q out=%q err=%q", f.calls(), f.op.SkippedOptional, f.out.String(), f.err.String())
			}
		})
	}
}
