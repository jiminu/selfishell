package benchmark

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestParseOptions(t *testing.T) {
	o, help, err := parseOptions(nil, map[string]string{
		"SELFISHELL_BENCHMARK_ROOT": "/unrelated/root",
		"SELFISHELL_BENCHMARK_CLI":  "/unrelated/selfishell",
	}, "/checkout")
	if err != nil || help || o.root != "/checkout" || o.cli != "/checkout/.build/selfishell" {
		t.Fatalf("benchmark must use this checkout: %+v help=%v err=%v", o, help, err)
	}
	cases := []struct {
		name string
		args []string
		env  map[string]string
		code int
		want string
	}{
		{"unknown mode", []string{"--mode", "bogus"}, nil, 2, `must be "base" or "full"`},
		{"missing mode", []string{"--mode"}, nil, 2, "--mode requires base or full"},
		{"unknown option", []string{"--bogus-flag"}, nil, 2, "Unknown option: --bogus-flag"},
		{"help", []string{"--help"}, nil, 0, "[--mode base|full]"},
		{"short help", []string{"-h"}, nil, 0, "[--mode base|full]"},
		{"profile env", nil, map[string]string{"SELFISHELL_BENCHMARK_PROFILE": "bogus"}, 2, `must be "base" or "full"`},
		{"zero iterations", nil, map[string]string{"SELFISHELL_BENCHMARK_ITERATIONS": "0"}, 2, "must be a positive integer"},
		{"noninteger iterations", nil, map[string]string{"SELFISHELL_BENCHMARK_ITERATIONS": "a"}, 2, "must be a positive integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			var out, err bytes.Buffer
			env := []string{"TMPDIR=" + tmp}
			for k, v := range tc.env {
				env = append(env, k+"="+v)
			}
			code := Run(tc.args, env, "/missing/source", &out, &err)
			if code != tc.code || !strings.Contains(out.String()+err.String(), tc.want) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), err.String())
			}
			entries, e := os.ReadDir(tmp)
			if e != nil || len(entries) != 0 {
				t.Fatalf("temporary files on early exit: %v %v", entries, e)
			}
		})
	}
}

func TestMissingAliases(t *testing.T) {
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "config/shared/zsh"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(root, ".build"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, ".build/selfishell"), []byte("#!/bin/sh\nexit 0\n"), 0755); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"common", "runtime", "history", "completion", "interactive", "update-notice"} {
		if e := os.WriteFile(filepath.Join(root, "config/shared/zsh", name+".zsh"), []byte(""), 0644); e != nil {
			t.Fatal(e)
		}
	}
	var out, err bytes.Buffer
	code := Run([]string{"--mode", "base"}, []string{"SELFISHELL_BENCHMARK_ITERATIONS=1"}, root, &out, &err)
	if code == 0 || !strings.Contains(err.String(), "Missing benchmark Zsh module: aliases.zsh") {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
}

func TestRunnerBaseIsolationAndZprof(t *testing.T) {
	root := repositoryRoot(t)
	buildTestCLI(t, root)
	private := t.TempDir()
	ambient := filepath.Join(private, "ambient")
	if e := os.MkdirAll(ambient, 0755); e != nil {
		t.Fatal(e)
	}
	for _, tool := range []string{"starship", "fzf", "zoxide"} {
		if e := os.WriteFile(filepath.Join(ambient, tool), []byte("#!/bin/sh\nexit 99\n"), 0755); e != nil {
			t.Fatal(e)
		}
	}
	callerData := filepath.Join(private, "caller-data")
	zinit := filepath.Join(callerData, "zinit/zinit.git")
	if e := os.MkdirAll(zinit, 0755); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(private, "ambient-zinit-loaded")
	if e := os.WriteFile(filepath.Join(zinit, "zinit.zsh"), []byte("print loaded >"+sentinel+"\n"), 0644); e != nil {
		t.Fatal(e)
	}
	profile := filepath.Join(private, "startup.zprof")
	env := []string{"HOME=" + filepath.Join(private, "outer-home"), "TMPDIR=" + private, "PATH=" + ambient + ":/usr/bin:/bin", "XDG_DATA_HOME=" + callerData, "SELFISHELL_BENCHMARK_ITERATIONS=1", "SELFISHELL_BENCHMARK_ZPROF_FILE=" + profile}
	var out, err bytes.Buffer
	if code := Run([]string{"--mode", "base"}, env, root, &out, &err); code != 0 {
		t.Fatalf("exit %d: %s", code, err.String())
	}
	for _, want := range []string{"mode=base", "common-cached", "interactive-cached", "starship=absent fzf=absent zoxide=absent zinit=absent"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if _, e := os.Stat(sentinel); !os.IsNotExist(e) {
		t.Fatal("ambient Zinit was sourced")
	}
	data, e := os.ReadFile(profile)
	if e != nil {
		t.Fatal(e)
	}
	if len(data) == 0 || !strings.Contains(string(data), "num  calls") || strings.Contains(string(data), "no such file or directory") || strings.Contains(strings.ToLower(string(data)), "starship") {
		t.Fatalf("invalid zprof report: %s", data)
	}
	if strings.Contains(string(data), os.Getenv("HOME")+"/.config/mise") {
		t.Fatal("ambient mise config in zprof")
	}
}

func TestRunnerPromptDiagnosticsAndTSV(t *testing.T) {
	root := repositoryRoot(t)
	cli := buildTestCLI(t, root)
	private := t.TempDir()
	results := filepath.Join(private, "results.tsv")
	guardedCLI, guardLog := guardDiagnosticCLI(t, private, cli)
	env := []string{"HOME=" + filepath.Join(private, "outer-home"), "TMPDIR=" + private, "PATH=/usr/bin:/bin", "SELFISHELL_BENCHMARK_ITERATIONS=1", "SELFISHELL_BENCHMARK_RESULTS_FILE=" + results}
	var out, err bytes.Buffer
	benchmarkEnv := environment(env)
	opts, _, e := parseOptions([]string{"--mode", "base", "--prompt", "--diagnostics"}, benchmarkEnv, root)
	if e != nil {
		t.Fatal(e)
	}
	opts.cli = guardedCLI
	if e := run(context.Background(), opts, benchmarkEnv, &out, &err); e != nil {
		t.Fatalf("benchmark: %v: %s", e, err.String())
	}
	if data, e := os.ReadFile(guardLog); e == nil {
		t.Fatalf("diagnostic reached guarded command: %s", data)
	} else if !os.IsNotExist(e) {
		t.Fatal(e)
	}
	for _, want := range []string{"Diagnostics: configured HOME", "cli-status exit=", "[OK] Configuration:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	data, e := os.ReadFile(results)
	if e != nil {
		t.Fatal(e)
	}
	for _, metric := range []string{"prompt-first-empty", "prompt-command-empty", "prompt-first-repository", "prompt-command-repository", "cli-status", "cli-doctor"} {
		found := false
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) == 8 && fields[3] == metric {
				if fields[1] != nativeArchitecture(runtime.GOOS, runtime.GOARCH) {
					t.Fatalf("TSV architecture %q", fields[1])
				}
				for _, v := range fields[4:] {
					number, e := strconv.ParseFloat(v, 64)
					if e != nil || number <= 0 {
						t.Fatalf("nonpositive %s: %q", metric, line)
					}
				}
				found = true
			}
		}
		if !found {
			t.Errorf("missing eight-field result for %s", metric)
		}
	}
}

func TestFullProvisionUsesCheckoutHelper(t *testing.T) {
	private := t.TempDir()
	root := filepath.Join(private, "checkout")
	home := filepath.Join(private, "home")
	bin := filepath.Join(private, "bin")
	log := filepath.Join(private, "helper.log")
	for _, p := range []string{root, home, bin, filepath.Join(root, "scripts")} {
		if e := os.MkdirAll(p, 0755); e != nil {
			t.Fatal(e)
		}
	}
	policy, err := os.ReadFile(filepath.Join(repositoryRoot(t), "scripts/go-env.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/go-env.sh"), policy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/test\n\ngo 1.23.4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache, modules := filepath.Join(private, "go-cache"), filepath.Join(private, "go-modcache")
	script := "#!/bin/sh\nif [ \"$*\" = 'env GOVERSION' ]; then printf '%s\\n' 'go1.23.4'; exit 0; fi\nprintf '%s\\n' \"$PWD|$*|$HOME|$XDG_DATA_HOME|$MISE_OFFLINE|$GOTOOLCHAIN|$GOENV|$GOWORK|$GOFLAGS|$GOOS|$GOARCH|$GOCACHE|$GOMODCACHE\" >" + log + "\n"
	if e := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	f := &fixture{options: options{root: root}, home: home, data: filepath.Join(home, ".local/share"), env: map[string]string{"PATH": bin + ":/usr/bin:/bin", "GOCACHE": cache, "GOMODCACHE": modules}}
	if e := f.provision(context.Background()); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{root + "|run -buildvcs=false ./cmd/selfishell-dev " + root + " benchmark-shell|" + home + "|" + filepath.Join(home, ".local/share") + "|0|local|off|off||||" + cache + "|" + modules + "\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("helper routing: %s", data)
		}
	}
}

func TestSelectedPins(t *testing.T) {
	root := repositoryRoot(t)
	data, e := selectedPins(filepath.Join(root, "config/shared/mise.toml"))
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"starship", "fzf", "zoxide"} {
		if !strings.Contains(data, name+" = ") {
			t.Fatalf("missing %s pin: %s", name, data)
		}
	}
	if strings.Contains(data, "neovim") || !strings.Contains(data, "not_found_auto_install = false") {
		t.Fatalf("wrong config: %s", data)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	return filepath.Clean(filepath.Join(dir, "../.."))
}
func buildTestCLI(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, ".build/selfishell")
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if testutil.CopyCLI(t, path) {
		return path
	}
	cmd := exec.Command("go", "build", "-o", path, "./cmd/selfishell")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GOTOOLCHAIN=local", "GOCACHE="+testutil.GoCache(t), "GOPROXY=off")
	output, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("build CLI: %v: %s", e, output)
	}
	return path
}

func TestFullDiagnosticHomeSharesPrivateMise(t *testing.T) {
	private := t.TempDir()
	f := &fixture{options: options{mode: "full"}, home: filepath.Join(private, "shell"), data: filepath.Join(private, "shell/.local/share")}
	source := filepath.Join(f.home, ".local/bin/mise")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(private, "diagnostics")
	if err := f.prepareDiagnosticHome(home); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".local/bin/mise")
	if info, err := os.Stat(target); err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("diagnostic mise target unavailable: %v, %v", info, err)
	}
	if link, err := os.Readlink(target); err != nil || link != source {
		t.Fatalf("diagnostic mise link = %q, %v", link, err)
	}
}

func TestWrapperPreservesArgumentExitStatus(t *testing.T) {
	root := repositoryRoot(t)
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"invalid mode", []string{"--mode", "bogus"}, `must be "base" or "full"`},
		{"missing mode", []string{"--mode"}, "--mode requires base or full"},
		{"unknown option", []string{"--bogus-flag"}, "Unknown option: --bogus-flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private := t.TempDir()
			home := filepath.Join(private, "home")
			tmp := filepath.Join(private, "tmp")
			for _, p := range []string{home, tmp} {
				if e := os.MkdirAll(p, 0755); e != nil {
					t.Fatal(e)
				}
			}
			cmd := exec.Command("bash", append([]string{filepath.Join(root, "scripts/benchmark.sh")}, tc.args...)...)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "HOME="+home, "TMPDIR="+tmp)
			output, e := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 2 || !strings.Contains(string(output), tc.message) {
				t.Fatalf("wrapper exit=%v output=%s", e, output)
			}
			entries, e := os.ReadDir(tmp)
			if e != nil || len(entries) != 0 {
				t.Fatalf("temporary leftovers: %v %v", entries, e)
			}
		})
	}
}

func TestBudgetWarningAndEnforcement(t *testing.T) {
	var warning bytes.Buffer
	f := &fixture{options: options{mode: "base"}, env: map[string]string{}, stderr: &warning}
	if e := f.budget("cli-help", 3, "2"); e != nil || !strings.Contains(warning.String(), "WARNING: benchmark budget exceeded") {
		t.Fatalf("warning=%q err=%v", warning.String(), e)
	}
	f.enforce = true
	if e := f.budget("cli-help", 3, "2"); e == nil || !strings.Contains(e.Error(), "Benchmark budget exceeded") {
		t.Fatalf("enforcement=%v", e)
	}
}

func TestTimedChildCancellationCleansDescendants(t *testing.T) {
	private := t.TempDir()
	sentinel := filepath.Join(private, "escaped")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	script := `(/bin/sleep 1; /usr/bin/touch "$1") & wait`
	_, err := timedExecute(ctx, execSpec{"/bin/sh", []string{"-c", script, "sh", sentinel}, private, []string{"PATH=/usr/bin:/bin"}})
	if err == nil {
		t.Fatal("canceled child succeeded")
	}
	time.Sleep(1200 * time.Millisecond)
	if _, e := os.Stat(sentinel); !os.IsNotExist(e) {
		t.Fatal("descendant survived cancellation")
	}
}

func TestResultsAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.tsv")
	file, e := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		t.Fatal(e)
	}
	f := &fixture{options: options{mode: "base"}, out: io.Discard, resultFile: file}
	f.metric("cli-help", Stats{Mean: 1, P50: 1, P95: 1, Max: 1})
	f.metric("cli-version", Stats{Mean: 2, P50: 2, P95: 2, Max: 2})
	if e := file.Close(); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if f.resultErr != nil || len(lines) != 2 || !strings.Contains(lines[0], "\tcli-help\t1.000") || !strings.Contains(lines[1], "\tcli-version\t2.000") {
		t.Fatalf("append: %q err=%v", data, f.resultErr)
	}
}

func TestNativeArchitectureLabels(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, want string }{
		{"darwin", "amd64", "x86_64"}, {"darwin", "arm64", "arm64"},
		{"linux", "amd64", "x86_64"}, {"linux", "arm64", "aarch64"},
	} {
		if got := nativeArchitecture(tc.goos, tc.goarch); got != tc.want {
			t.Errorf("%s/%s = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestIntegrationQueryCancellationCleansDescendants(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local/bin")
	if e := os.MkdirAll(bin, 0755); e != nil {
		t.Fatal(e)
	}
	startedMarker := filepath.Join(home, "started")
	sentinel := filepath.Join(home, "escaped")
	script := fmt.Sprintf("#!/bin/sh\n/usr/bin/touch '%s'\n(/bin/sleep 1; /usr/bin/touch '%s') &\nwait\n", startedMarker, sentinel)
	if e := os.WriteFile(filepath.Join(bin, "mise"), []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	f := &fixture{options: options{mode: "full"}, home: home, data: filepath.Join(home, ".local/share"), interactivePath: bin, env: map[string]string{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := f.integrations(ctx); done <- e }()
	deadline := time.After(2 * time.Second)
	for {
		if _, e := os.Stat(startedMarker); e == nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("fake mise query did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("stuck query cancellation: %v", e)
		}
	case <-time.After(time.Second):
		t.Error("mise query did not cancel promptly")
		// Still check the sentinel: a surviving descendant may be holding the
		// output pipe open, rather than the runner merely being slow.
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("mise query remained blocked after cancellation")
		}
	}
	time.Sleep(1200 * time.Millisecond)
	if _, e := os.Stat(sentinel); !os.IsNotExist(e) {
		t.Fatal("mise query descendant survived cancellation")
	}
}

// Test-only CLI wrapper keeps the real executable while blocking package,
// privilege, login-shell, and download commands if --skip-packages regresses.
func guardDiagnosticCLI(t *testing.T, private, realCLI string) (string, string) {
	t.Helper()
	bin := filepath.Join(private, "guard-bin")
	if e := os.MkdirAll(bin, 0755); e != nil {
		t.Fatal(e)
	}
	log := filepath.Join(private, "guard-called")
	for _, name := range []string{"apt-get", "apt", "sudo", "chsh", "curl", "wget", "brew", "git"} {
		body := fmt.Sprintf(`#!/bin/sh
case %s in
 brew) case "$1" in install|upgrade|update|tap|uninstall|reinstall) ;; *) exit 1;; esac ;;
 git) case "$1" in --version) printf 'git version 2.50.0\n'; exit 0;; esac ;;
esac
printf '%%s\n' '%s' >> '%s'
exit 97
`, name, name, log)
		if e := os.WriteFile(filepath.Join(bin, name), []byte(body), 0755); e != nil {
			t.Fatal(e)
		}
	}
	wrapper := filepath.Join(private, "guarded-selfishell")
	body := fmt.Sprintf("#!/bin/sh\nexport PATH='%s':\"$PATH\"\nexec '%s' \"$@\"\n", bin, realCLI)
	if e := os.WriteFile(wrapper, []byte(body), 0755); e != nil {
		t.Fatal(e)
	}
	// Show the guard fails before the real CLI test uses it, then clear the log.
	probe := exec.Command(filepath.Join(bin, "apt-get"), "update")
	if e := probe.Run(); e == nil {
		t.Fatal("package guard allowed apt-get")
	}
	if data, e := os.ReadFile(log); e != nil || !strings.Contains(string(data), "apt-get") {
		t.Fatalf("guard probe: %q %v", data, e)
	}
	if e := os.Remove(log); e != nil {
		t.Fatal(e)
	}
	return wrapper, log
}

func TestRunnerRelativePathsFromSeparateCWD(t *testing.T) {
	source := repositoryRoot(t)
	caller := t.TempDir()
	buildTestCLI(t, source)
	for _, p := range []string{filepath.Join(caller, "tmp"), filepath.Join(caller, "out")} {
		if e := os.MkdirAll(p, 0755); e != nil {
			t.Fatal(e)
		}
	}
	outer := filepath.Join(caller, "outer-home")
	env := []string{"HOME=" + outer, "GOCACHE=" + testutil.GoCache(t), "TMPDIR=tmp", "PATH=" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "SELFISHELL_BENCHMARK_ITERATIONS=1", "SELFISHELL_BENCHMARK_RESULTS_FILE=out/results.tsv", "SELFISHELL_BENCHMARK_ZPROF_FILE=out/startup.zprof"}
	cmd := exec.Command("bash", filepath.Join(source, "scripts/benchmark.sh"), "--mode", "base")
	cmd.Dir = caller
	cmd.Env = env
	output, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("wrapper from caller cwd: %v %s", e, output)
	}
	verifyRelativeBenchmarkArtifacts(t, caller, "results.tsv", "startup.zprof")
	if e := os.Remove(filepath.Join(caller, "out/results.tsv")); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(filepath.Join(caller, "out/startup.zprof")); e != nil {
		t.Fatal(e)
	}
	original, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chdir(caller); e != nil {
		t.Fatal(e)
	}
	defer os.Chdir(original)
	var directOut, directErr bytes.Buffer
	if code := Run([]string{"--mode", "base"}, env, source, &directOut, &directErr); code != 0 {
		t.Fatalf("direct Run exit %d: %s", code, directErr.String())
	}
	verifyRelativeBenchmarkArtifacts(t, caller, "results.tsv", "startup.zprof")
}

func verifyRelativeBenchmarkArtifacts(t *testing.T, caller, results, profile string) {
	t.Helper()
	rows, e := os.ReadFile(filepath.Join(caller, "out", results))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(rows), "\tcli-version\t") || !strings.Contains(string(rows), "\tcli-help\t") {
		t.Fatalf("missing CLI results: %s", rows)
	}
	zprof, e := os.ReadFile(filepath.Join(caller, "out", profile))
	if e != nil || !strings.Contains(string(zprof), "num  calls") {
		t.Fatalf("missing caller-relative zprof: %v %s", e, zprof)
	}
	entries, e := os.ReadDir(filepath.Join(caller, "tmp"))
	if e != nil || len(entries) != 0 {
		t.Fatalf("temporary leftovers: %v %v", entries, e)
	}
}
