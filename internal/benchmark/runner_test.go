package benchmark

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseOptions(t *testing.T) {
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
	cli := buildTestCLI(t, root)
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
	env := []string{"HOME=" + filepath.Join(private, "outer-home"), "TMPDIR=" + private, "PATH=" + ambient + ":/usr/bin:/bin", "XDG_DATA_HOME=" + callerData, "SELFISHELL_BENCHMARK_CLI=" + cli, "SELFISHELL_BENCHMARK_ITERATIONS=1", "SELFISHELL_BENCHMARK_ZPROF_FILE=" + profile}
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
	env := []string{"HOME=" + filepath.Join(private, "outer-home"), "TMPDIR=" + private, "PATH=/usr/bin:/bin", "SELFISHELL_BENCHMARK_CLI=" + cli, "SELFISHELL_BENCHMARK_ITERATIONS=1", "SELFISHELL_BENCHMARK_RESULTS_FILE=" + results}
	var out, err bytes.Buffer
	if code := Run([]string{"--mode", "base", "--prompt", "--diagnostics"}, env, root, &out, &err); code != 0 {
		t.Fatalf("exit %d: %s", code, err.String())
	}
	for _, want := range []string{"Diagnostics: configured HOME", "cli-status exit=", "[SUMMARY] Managed paths:"} {
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

func TestFullProvisionUsesSourceHelperAndSelectedRoot(t *testing.T) {
	private := t.TempDir()
	source := filepath.Join(private, "go-source")
	selected := filepath.Join(private, "historical-root")
	home := filepath.Join(private, "home")
	bin := filepath.Join(private, "bin")
	log := filepath.Join(private, "helper.log")
	for _, p := range []string{source, selected, home, bin} {
		if e := os.MkdirAll(p, 0755); e != nil {
			t.Fatal(e)
		}
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD|$*|$HOME|$XDG_DATA_HOME|$MISE_OFFLINE\" >" + log + "\n"
	if e := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	f := &fixture{options: options{root: selected}, source: source, home: home, data: filepath.Join(home, ".local/share"), env: map[string]string{"PATH": "/usr/bin:/bin"}}
	if e := f.provision(context.Background()); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{source + "|run ./cmd/selfishell-dev " + selected + " benchmark-shell|" + home + "|" + filepath.Join(home, ".local/share") + "|0"} {
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
	cmd := exec.Command("go", "build", "-o", path, "./cmd/selfishell")
	cmd.Dir = root
	output, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("build CLI: %v: %s", e, output)
	}
	return path
}

func TestRunnerSelectedHistoricalRootAndCLI(t *testing.T) {
	source := repositoryRoot(t)
	private := t.TempDir()
	historical := filepath.Join(private, "bash-export")
	if e := os.MkdirAll(filepath.Join(historical, "bin"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(source, "config"), filepath.Join(historical, "config")); e != nil {
		t.Fatal(e)
	}
	log := filepath.Join(private, "legacy-cli-args")
	cli := filepath.Join(historical, "bin/selfishell")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >>" + log + "\nexit 0\n"
	if e := os.WriteFile(cli, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	env := []string{"HOME=" + filepath.Join(private, "outer-home"), "TMPDIR=" + private, "PATH=/usr/bin:/bin", "SELFISHELL_BENCHMARK_ROOT=" + historical, "SELFISHELL_BENCHMARK_CLI=" + cli, "SELFISHELL_BENCHMARK_ITERATIONS=1"}
	var out, stderr bytes.Buffer
	if code := Run(nil, env, source, &out, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "cli-version") || !strings.Contains(out.String(), "cli-help") {
		t.Fatal("selected CLI metrics missing")
	}
	data, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	if string(data) != "version\nhelp\n" {
		t.Fatalf("selected CLI calls: %q", data)
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
