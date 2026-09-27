package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const usage = `Usage: scripts/benchmark.sh [--mode base|full] [--prompt] [--diagnostics]

  base  Selfishell's own startup cost, independent of external integrations
        (mise/starship/zinit/fzf/zoxide are excluded). This is the default.

  full  Installs pinned mise, starship, fzf, zoxide, and zinit with plugins
        into an isolated HOME. Does not install Apt/Homebrew packages.

  --prompt       Measure first and command-to-prompt latency in a PTY.
  --diagnostics  Measure status and doctor after isolated configuration setup.

SELFISHELL_BENCHMARK_PROFILE=base|full is equivalent to --mode.
The benchmark uses this checkout and its built .build/selfishell executable.
`

type options struct {
	mode                      string
	iterations                int
	prompt, diagnostics       bool
	root, cli, results, zprof string
	enforce                   bool
}

func environment(pairs []string) map[string]string {
	m := map[string]string{}
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}
func getenv(m map[string]string, k, defaultValue string) string {
	if v := m[k]; v != "" {
		return v
	}
	return defaultValue
}

func parseOptions(args []string, env map[string]string, source string) (options, bool, error) {
	o := options{mode: getenv(env, "SELFISHELL_BENCHMARK_PROFILE", "base"), iterations: 30, root: source, results: env["SELFISHELL_BENCHMARK_RESULTS_FILE"], zprof: env["SELFISHELL_BENCHMARK_ZPROF_FILE"], enforce: env["SELFISHELL_BENCHMARK_ENFORCE"] == "1"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			i++
			if i == len(args) {
				return o, false, errors.New("--mode requires base or full")
			}
			o.mode = args[i]
		case "--prompt":
			o.prompt = true
		case "--diagnostics":
			o.diagnostics = true
		case "--help", "-h":
			return o, true, nil
		default:
			return o, false, fmt.Errorf("Unknown option: %s", args[i])
		}
	}
	if o.mode != "base" && o.mode != "full" {
		return o, false, fmt.Errorf("--mode/SELFISHELL_BENCHMARK_PROFILE must be \"base\" or \"full\" (got: %s)", o.mode)
	}
	if s, ok := env["SELFISHELL_BENCHMARK_ITERATIONS"]; ok {
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				return o, false, errors.New("SELFISHELL_BENCHMARK_ITERATIONS must be a positive integer")
			}
		}
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 {
			return o, false, errors.New("SELFISHELL_BENCHMARK_ITERATIONS must be a positive integer")
		}
		o.iterations = n
	}
	o.cli = filepath.Join(o.root, ".build/selfishell")
	return o, false, nil
}

// Run benchmarks the current checkout and its built CLI. source is the checkout root.
func Run(args, inherited []string, source string, out, stderr io.Writer) int {
	env := environment(inherited)
	o, help, err := parseOptions(args, env, source)
	if help {
		fmt.Fprint(out, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprint(stderr, usage)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "benchmark: resolve invocation directory:", err)
		return 1
	}
	absolute := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(cwd, p)
	}
	o.root = absolute(o.root)
	o.cli = absolute(o.cli)
	o.results = absolute(o.results)
	o.zprof = absolute(o.zprof)
	if value := env["TMPDIR"]; value != "" {
		env["TMPDIR"] = absolute(value)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = run(ctx, o, env, out, stderr); err != nil {
		fmt.Fprintln(stderr, "benchmark:", err)
		return 1
	}
	return 0
}

type fixture struct {
	options
	dir, home, data, platform, commonPath, interactivePath string
	env                                                    map[string]string
	out, stderr                                            io.Writer
	resultFile                                             *os.File
	resultErr                                              error
}

func run(ctx context.Context, o options, env map[string]string, out, stderr io.Writer) error {
	if _, e := os.Stat(o.cli); e != nil {
		return fmt.Errorf("built CLI %s unavailable: run bash scripts/build-cli.sh: %w", o.cli, e)
	}
	tmp := getenv(env, "TMPDIR", os.TempDir())
	dir, e := os.MkdirTemp(tmp, "selfishell-benchmark.")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	f := &fixture{options: o, dir: dir, home: filepath.Join(dir, "home"), env: env, out: out, stderr: stderr}
	f.data = filepath.Join(f.home, ".local/share")
	if runtime.GOOS == "darwin" {
		f.platform = filepath.Join(o.root, "config/macos/zshrc")
	} else {
		f.platform = filepath.Join(o.root, "config/ubuntu/zshrc")
	}
	if e = f.setup(ctx); e != nil {
		return e
	}
	if o.results != "" {
		f.resultFile, e = os.OpenFile(o.results, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if e != nil {
			return fmt.Errorf("results file: %w", e)
		}
		defer f.resultFile.Close()
	}
	fmt.Fprintf(out, "Selfishell benchmark (mode=%s, %d iterations, milliseconds per run)\nmetric\tmean\tp50\tp95\tmax\n", o.mode, o.iterations)
	integrationSummary, e := f.integrations(ctx)
	if e != nil {
		return e
	}
	f.recordIntegration(integrationSummary)
	if f.resultErr != nil {
		return f.resultErr
	}
	shellEnv := f.shellEnv(false)
	if _, e = f.measure(ctx, "baseline-zsh", o.iterations, execSpec{"/bin/zsh", []string{"-f", "-c", ":"}, f.home, shellEnv}); e != nil {
		return e
	}
	common := execSpec{"/bin/zsh", []string{"-f", "-c", `source "$1"`, "zsh", filepath.Join(o.root, "config/shared/zsh/common.zsh")}, f.home, f.shellEnv(true)}
	if _, e = f.measure(ctx, "common-first", 1, common); e != nil {
		return e
	}
	cs, e := f.measure(ctx, "common-cached", o.iterations, common)
	if e != nil {
		return e
	}
	interactive := execSpec{"/bin/zsh", []string{"-d", "-i", "-c", "exit"}, f.home, shellEnv}
	if _, e = execute(ctx, interactive); e != nil {
		return fmt.Errorf("interactive warmup: %w", e)
	}
	if e = f.verifyFull(ctx, f.home); e != nil {
		return e
	}
	is, e := f.measure(ctx, "interactive-cached", o.iterations, interactive)
	if e != nil {
		return e
	}
	// CLI probes use system tools while startup probes keep their masked PATH.
	cliEnv := append(append([]string{}, shellEnv...), "PATH=/usr/bin:/bin")
	version, e := f.measure(ctx, "cli-version", o.iterations, execSpec{o.cli, []string{"version"}, f.home, cliEnv})
	if e != nil {
		return e
	}
	help, e := f.measure(ctx, "cli-help", o.iterations, execSpec{o.cli, []string{"help"}, f.home, cliEnv})
	if e != nil {
		return e
	}
	for _, b := range []struct {
		label, key string
		stats      Stats
	}{{"common-cached", "SELFISHELL_BENCHMARK_COMMON_P95_MAX_MS", cs}, {"interactive-cached", "SELFISHELL_BENCHMARK_INTERACTIVE_P95_MAX_MS", is}, {"cli-version", "SELFISHELL_BENCHMARK_VERSION_P95_MAX_MS", version}, {"cli-help", "SELFISHELL_BENCHMARK_HELP_P95_MAX_MS", help}} {
		if e = f.budget(b.label, b.stats.P95, env[b.key]); e != nil {
			return e
		}
	}
	if o.zprof != "" {
		if e = f.profile(ctx); e != nil {
			return e
		}
	}
	if o.prompt {
		if e = f.prompts(ctx); e != nil {
			return e
		}
	}
	if o.diagnostics {
		if e = f.diagnosticsRun(ctx); e != nil {
			return e
		}
	}
	return f.resultErr
}

func mkdir(path string) error                         { return os.MkdirAll(path, 0755) }
func write(path, data string, mode os.FileMode) error { return os.WriteFile(path, []byte(data), mode) }
func (f *fixture) setup(ctx context.Context) error {
	for _, p := range []string{filepath.Join(f.home, ".cache/selfishell"), filepath.Join(f.home, ".config/mise"), filepath.Join(f.home, ".config/selfishell/zsh"), filepath.Join(f.home, ".local/bin"), f.data} {
		if e := mkdir(p); e != nil {
			return e
		}
	}
	miseConfig := filepath.Join(f.home, ".config/mise/config.toml")
	if e := write(miseConfig, "", 0644); e != nil {
		return e
	}
	for _, name := range []string{"common", "runtime", "history", "completion", "interactive", "update-notice", "aliases"} {
		target := filepath.Join(f.root, "config/shared/zsh", name+".zsh")
		if info, e := os.Stat(target); e != nil || info.IsDir() {
			return fmt.Errorf("Missing benchmark Zsh module: %s.zsh", name)
		}
		if e := os.Symlink(target, filepath.Join(f.home, ".config/selfishell/zsh", name+".zsh")); e != nil {
			return e
		}
	}
	if _, e := os.Stat(f.platform); e != nil {
		return fmt.Errorf("platform Zsh entrypoint %s: %w", f.platform, e)
	}
	if e := os.Symlink(f.platform, filepath.Join(f.home, ".zshrc")); e != nil {
		return e
	}
	if e := write(filepath.Join(f.home, ".cache/selfishell/update-checked-at"), strconv.FormatInt(time.Now().Unix(), 10)+"\n", 0644); e != nil {
		return e
	}
	local := filepath.Join(f.home, ".local/bin")
	if f.mode == "base" {
		if runtime.GOOS == "darwin" {
			if e := write(filepath.Join(local, "brew"), "#!/bin/sh\nexit 0\n", 0755); e != nil {
				return e
			}
		}
		for name, target := range map[string]string{"mv": "/bin/mv", "touch": "/usr/bin/touch"} {
			if e := os.Symlink(target, filepath.Join(local, name)); e != nil {
				return e
			}
		}
		f.commonPath = local
		f.interactivePath = filepath.Join(f.root, "bin") + ":" + local
	} else {
		if e := f.provision(ctx); e != nil {
			return e
		}
		f.commonPath = local + ":/usr/bin:/bin"
		f.interactivePath = filepath.Join(f.root, "bin") + ":" + local + ":" + getenv(f.env, "PATH", "/usr/bin:/bin")
		pins, e := selectedPins(filepath.Join(f.root, "config/shared/mise.toml"))
		if e != nil {
			return e
		}
		if e = write(miseConfig, pins, 0644); e != nil {
			return e
		}
	}
	return nil
}

func selectedPins(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	var lines []string
	inTools := false
	found := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			inTools = trim == "[tools]"
			continue
		}
		if !inTools {
			continue
		}
		for _, name := range []string{"starship", "fzf", "zoxide"} {
			if strings.HasPrefix(trim, name+" ") || strings.HasPrefix(trim, name+"=") {
				lines = append(lines, line)
				found[name] = true
			}
		}
	}
	if len(found) != 3 {
		return "", errors.New("selected mise config lacks a required shell integration pin")
	}
	return "[tools]\n" + strings.Join(lines, "\n") + "\n\n[settings]\nnot_found_auto_install = false\n", nil
}
func (f *fixture) provision(ctx context.Context) error {
	// selfishell-dev reads process-global HOME/XDG; keep it in a private child.
	env := f.baseEnv(filepath.Join(f.home, ".local/bin") + ":" + getenv(f.env, "PATH", "/usr/bin:/bin"))
	env = append(env, "MISE_CEILING_PATHS="+f.root, "MISE_OFFLINE=0")
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "GOCACHE", "GOMODCACHE"} {
		if v := f.env[key]; v != "" {
			env = append(env, key+"="+v)
		}
	}
	// Apply the same source-build policy inside the private child. This setup
	// never enters a timed sample or reads the caller's Go workspace/config.
	script := `source "$1/scripts/go-env.sh"; selfishell_prepare_go "$1"; exec go run -buildvcs=false ./cmd/selfishell-dev "$1" benchmark-shell`
	spec := execSpec{"/bin/bash", []string{"-euc", script, "bash", f.root}, f.root, env}
	_, e := execute(ctx, spec)
	if e != nil {
		return fmt.Errorf("private full provisioning failed: %w", e)
	}
	return nil
}

type execSpec struct {
	name string
	args []string
	dir  string
	env  []string
}

func execute(ctx context.Context, s execSpec) (string, error) {
	cmd := exec.CommandContext(ctx, s.name, s.args...)
	cleanup := configureCancellation(cmd)
	defer cleanup()
	cmd.Dir = s.dir
	cmd.Env = s.env
	output, e := cmd.CombinedOutput()
	if e != nil {
		return string(output), e
	}
	return string(output), nil
}

func configureCancellation(cmd *exec.Cmd) func() {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	canceled := false
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		canceled = err == nil
		return err
	}
	cmd.WaitDelay = 200 * time.Millisecond
	return func() {
		// Wait joins the cancellation callback before this cleanup runs.
		// A fork can race the first group signal, so sweep after reaping the
		// parent. WaitDelay bounds pipes inherited by a surviving child.
		if canceled {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
}

func (f *fixture) baseEnv(path string) []string {
	return []string{
		"HOME=" + f.home, "ZDOTDIR=" + f.home, "XDG_CONFIG_HOME=" + filepath.Join(f.home, ".config"), "XDG_DATA_HOME=" + f.data,
		"XDG_STATE_HOME=" + filepath.Join(f.home, ".local/state"), "XDG_CACHE_HOME=" + filepath.Join(f.home, ".cache"),
		"MISE_DATA_DIR=" + filepath.Join(f.data, "mise"), "MISE_CACHE_DIR=" + filepath.Join(f.home, ".cache/mise"), "MISE_STATE_DIR=" + filepath.Join(f.home, ".local/state/mise"),
		"MISE_GLOBAL_CONFIG_FILE=" + filepath.Join(f.home, ".config/mise/config.toml"), "MISE_SHELL=", "MISE_OFFLINE=1", "SELFISHELL_UPDATE_NOTICE=0",
		"PATH=" + path, "TERM=xterm-256color", "SHELL=/bin/zsh", "LANG=" + getenv(f.env, "LANG", "en_US.UTF-8"), "TMPDIR=" + f.dir,
	}
}
func (f *fixture) shellEnv(common bool) []string {
	path := f.interactivePath
	if common {
		path = f.commonPath
	}
	env := f.baseEnv(path)
	if v := f.env["WSL_DISTRO_NAME"]; v != "" {
		env = append(env, "WSL_DISTRO_NAME="+v)
	}
	return env
}
func (f *fixture) appendResult(line string) {
	if f.resultFile == nil || f.resultErr != nil {
		return
	}
	_, f.resultErr = fmt.Fprintf(f.resultFile, "%s\t%s\t%s\t%s\n", platformName(), nativeArchitecture(runtime.GOOS, runtime.GOARCH), f.mode, line)
}
func (f *fixture) record(line string) {
	fmt.Fprintln(f.out, line)
	f.appendResult(line)
}
func nativeArchitecture(goos, goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		if goos == "linux" {
			return "aarch64"
		}
		return "arm64"
	default:
		return goarch
	}
}
func platformName() string {
	if runtime.GOOS == "darwin" {
		return "Darwin"
	}
	return "Linux"
}
func (f *fixture) recordComment(c string) { f.record("# " + c) }
func (f *fixture) recordIntegration(c string) {
	fmt.Fprintln(f.out, c)
	f.appendResult("# " + c)
}
func (f *fixture) metric(label string, s Stats) {
	f.record(fmt.Sprintf("%s\t%.3f\t%.3f\t%.3f\t%.3f", label, s.Mean, s.P50, s.P95, s.Max))
}
func (f *fixture) integrations(ctx context.Context) (string, error) {
	names := []string{"starship", "fzf", "zoxide"}
	parts := []string{"Interactive integrations:"}
	for _, name := range names {
		status := "absent"
		if f.mode == "full" {
			_, e := execute(ctx, execSpec{filepath.Join(f.home, ".local/bin/mise"), []string{"-C", f.home, "which", name}, f.home, f.shellEnv(false)})
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if e == nil {
				status = "enabled"
			}
		}
		parts = append(parts, name+"="+status)
	}
	zinit := "absent"
	if _, e := os.Stat(filepath.Join(f.data, "zinit/zinit.git/zinit.zsh")); e == nil {
		zinit = "enabled"
	}
	parts = append(parts, "zinit="+zinit)
	return strings.Join(parts, " "), nil
}
func timedExecute(ctx context.Context, s execSpec) (float64, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, s.name, s.args...)
	cleanup := configureCancellation(cmd)
	defer cleanup()
	cmd.Dir = s.dir
	cmd.Env = s.env
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return float64(time.Since(start)) / float64(time.Millisecond), err
}
func (f *fixture) measure(ctx context.Context, label string, n int, s execSpec) (Stats, error) {
	samples := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		elapsed, e := timedExecute(ctx, s)
		if e != nil {
			return Stats{}, fmt.Errorf("%s sample %d: %w", label, i+1, e)
		}
		samples = append(samples, elapsed)
	}
	stats, e := Summarize(samples)
	if e == nil {
		f.metric(label, stats)
	}
	if f.resultErr != nil {
		return Stats{}, f.resultErr
	}
	return stats, e
}
func (f *fixture) budget(label string, actual float64, raw string) error {
	if raw == "" {
		return nil
	}
	max, e := strconv.ParseFloat(raw, 64)
	if e != nil {
		return fmt.Errorf("invalid budget %s: %w", label, e)
	}
	if actual <= max {
		return nil
	}
	if f.enforce {
		return fmt.Errorf("Benchmark budget exceeded: %s p95 %.3fms > %.3fms", label, actual, max)
	}
	if f.env["GITHUB_ACTIONS"] == "true" {
		fmt.Fprintf(f.stderr, "::warning title=Shell performance budget::%s p95 %.3fms exceeds %.3fms\n", label, actual, max)
	} else {
		fmt.Fprintf(f.stderr, "WARNING: benchmark budget exceeded: %s p95 %.3fms > %.3fms\n", label, actual, max)
	}
	return nil
}
func (f *fixture) verifyFull(ctx context.Context, cwd string) error {
	if f.mode != "full" {
		return nil
	}
	script := `for tool in starship fzf zoxide; do [[ "${commands[$tool]}" == "$MISE_DATA_DIR/installs/"* ]] || exit 1; done; (( $+functions[prompt_starship_precmd] && $+functions[fzf-file-widget] && $+functions[__zoxide_z] ))`
	env := append(f.shellEnv(false), "MISE_CEILING_PATHS="+f.root)
	output, e := execute(ctx, execSpec{"/bin/zsh", []string{"-d", "-i", "-c", script}, cwd, env})
	if e != nil {
		return fmt.Errorf("Pinned shell integrations did not initialize in the benchmark HOME: %w: %s", e, output)
	}
	return nil
}
func (f *fixture) profile(ctx context.Context) error {
	zshenv := filepath.Join(f.home, ".zshenv")
	if e := write(zshenv, "zmodload zsh/zprof\n", 0644); e != nil {
		return e
	}
	defer os.Remove(zshenv)
	output, e := execute(ctx, execSpec{"/bin/zsh", []string{"-d", "-i", "-c", "zprof"}, f.home, f.shellEnv(false)})
	if e != nil {
		return e
	}
	return write(f.zprof, output, 0644)
}
func (f *fixture) prompts(ctx context.Context) error {
	pdir := filepath.Join(f.dir, "prompt")
	if e := mkdir(pdir); e != nil {
		return e
	}
	const config = `source "$SELFISHELL_BENCHMARK_PLATFORM_CONFIG"
autoload -Uz add-zsh-hook
setopt promptsubst
typeset -gi _selfishell_benchmark_prompt=0
_selfishell_benchmark_precmd() { (( ++_selfishell_benchmark_prompt )); }
add-zsh-hook precmd _selfishell_benchmark_precmd
RPROMPT+='__SFS_READY_${_selfishell_benchmark_prompt}__'
`
	if e := write(filepath.Join(pdir, ".zshrc"), config, 0644); e != nil {
		return e
	}
	if e := f.verifyFull(ctx, f.root); e != nil {
		return e
	}
	for _, scenario := range []struct{ label, cwd string }{{"empty", f.dir}, {"repository", f.root}} {
		if scenario.label == "empty" {
			scenario.cwd = filepath.Join(f.dir, "empty")
			if e := mkdir(scenario.cwd); e != nil {
				return e
			}
		}
		env := PromptEnvironment(f.root, f.home, pdir, f.platform, f.interactivePath, f.shellEnv(false))
		samples, e := MeasurePrompt(ctx, scenario.cwd, env, f.iterations)
		if e != nil {
			return e
		}
		first, e := Summarize(samples.First)
		if e != nil {
			return e
		}
		commands, e := Summarize(samples.Command)
		if e != nil {
			return e
		}
		f.metric("prompt-first-"+scenario.label, first)
		f.metric("prompt-command-"+scenario.label, commands)
	}
	return f.resultErr
}
func (f *fixture) diagnosticEnv(home, path string) []string {
	data := filepath.Join(home, ".local/share")
	env := []string{"HOME=" + home, "PATH=" + path, "SHELL=/bin/zsh", "TMPDIR=" + f.dir, "TERM=dumb", "NO_COLOR=1",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + data, "XDG_STATE_HOME=" + filepath.Join(home, ".local/state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"MISE_DATA_DIR=" + filepath.Join(f.data, "mise"), "MISE_CACHE_DIR=" + filepath.Join(home, ".cache/mise"), "MISE_STATE_DIR=" + filepath.Join(home, ".local/state/mise"),
		"MISE_OFFLINE=1", "MISE_CEILING_PATHS=" + f.root, "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1"}
	if v := f.env["WSL_DISTRO_NAME"]; v != "" {
		env = append(env, "WSL_DISTRO_NAME="+v)
	}
	return env
}
func (f *fixture) prepareDiagnosticHome(home string) error {
	if e := mkdir(filepath.Join(home, ".local/share")); e != nil {
		return e
	}
	if _, e := os.Stat(filepath.Join(f.data, "zinit")); e == nil {
		if e = os.Symlink(filepath.Join(f.data, "zinit"), filepath.Join(home, ".local/share/zinit")); e != nil {
			return e
		}
	}
	if f.mode == "full" {
		bin := filepath.Join(home, ".local/bin")
		if e := mkdir(bin); e != nil {
			return e
		}
		if e := os.Symlink(filepath.Join(f.home, ".local/bin/mise"), filepath.Join(bin, "mise")); e != nil {
			return e
		}
	}
	return nil
}
func (f *fixture) diagnosticsRun(ctx context.Context) error {
	home := filepath.Join(f.dir, "diagnostics-home")
	if e := f.prepareDiagnosticHome(home); e != nil {
		return e
	}
	path := "/usr/bin:/bin"
	if f.mode == "full" {
		path = filepath.Join(f.home, ".local/bin") + ":" + getenv(f.env, "PATH", "/usr/bin:/bin")
	}
	spec := execSpec{f.cli, []string{"install", "--skip-packages", "--yes"}, home, f.diagnosticEnv(home, path)}
	output, e := execute(ctx, spec)
	if e != nil {
		return fmt.Errorf("diagnostic setup: %w: %s", e, output)
	}
	f.recordComment("Diagnostics: configured HOME; no system packages installed; missing tools may yield exit 1.")
	for _, name := range []string{"status", "doctor"} {
		spec.args = []string{name}
		output, e = execute(ctx, spec)
		expected := exitStatus(e)
		if expected != 0 && expected != 1 {
			return fmt.Errorf("diagnostic %s: %w: %s", name, e, output)
		}
		f.recordComment(fmt.Sprintf("cli-%s exit=%d", name, expected))
		fmt.Fprint(f.out, output)
		samples := make([]float64, 0, f.iterations)
		for i := 0; i < f.iterations; i++ {
			elapsed, sampleErr := timedExecute(ctx, spec)
			if exitStatus(sampleErr) != expected {
				return fmt.Errorf("Diagnostic %s exit changed from %d to %d", name, expected, exitStatus(sampleErr))
			}
			samples = append(samples, elapsed)
		}
		stats, e := Summarize(samples)
		if e != nil {
			return e
		}
		f.metric("cli-"+name, stats)
	}
	return f.resultErr
}
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
