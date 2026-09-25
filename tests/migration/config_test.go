package migration_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var configPlatforms = []string{"macos", "ubuntu", "ubuntu-wsl"}
var configCases = []string{"empty", "existing", "custom", "changed-file", "changed-link", "changed-block", "pending", "late-preflight", "malformed-package"}

func configScenarios(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	mustFS(t, os.Mkdir(filepath.Join(release, ".git"), 0700))
	cli := filepath.Join(release, "bin/selfishell")
	reference, err := os.ReadFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := os.ReadFile(filepath.Join(release, "packages.conf"))
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := os.ReadFile(filepath.Join(release, "dependencies.conf"))
	if err != nil {
		t.Fatal(err)
	}
	tools := fixtureTools(t, root)
	osRelease := filepath.Join(root, "os-release")
	proc := filepath.Join(root, "proc-version")
	procWSL := filepath.Join(root, "proc-version-wsl")
	mustFS(t, os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600))
	mustFS(t, os.WriteFile(proc, []byte("Linux\n"), 0600))
	mustFS(t, os.WriteFile(procWSL, []byte("Linux microsoft WSL2\n"), 0600))
	for _, platform := range configPlatforms {
		for _, scenario := range configCases {
			t.Run(platform+"/"+scenario, func(t *testing.T) {
				var want map[string]capture
				for _, implementation := range []string{"bash", "go"} {
					home := filepath.Join(root, "home")
					mustFS(t, os.RemoveAll(home))
					mustFS(t, os.MkdirAll(home, 0700))
					mustFS(t, os.WriteFile(filepath.Join(release, "packages.conf"), packages, 0644))
					mustFS(t, os.WriteFile(filepath.Join(release, "dependencies.conf"), dependencies, 0644))
					if implementation == "bash" {
						mustFS(t, os.WriteFile(cli, reference, 0755))
					} else {
						mustFS(t, copyFile(candidate, cli))
					}
					env := configEnv(platform, scenario, home, tools, osRelease, proc, procWSL)
					got := runConfigScenario(t, home, cli, release, scenario, env)
					if implementation == "bash" {
						want = got
					} else {
						if len(want) != len(got) {
							t.Fatalf("capture count: bash %d go %d", len(want), len(got))
						}
						for name, expected := range want {
							actual, ok := got[name]
							if !ok {
								t.Fatalf("missing %s", name)
							}
							requireEqual(t, name, expected, actual)
						}
					}
				}
			})
		}
	}
}

func configEnv(platform, scenario, home, tools, osRelease, proc, procWSL string) []string {
	system := "Darwin"
	if platform != "macos" {
		system = "Linux"
	}
	if platform == "ubuntu-wsl" {
		proc = procWSL
	}
	env := []string{"PATH=" + tools, "SELFISHELL_TEST_SYSTEM_NAME=" + system, "SELFISHELL_TEST_OS_RELEASE_FILE=" + osRelease, "SELFISHELL_TEST_PROC_VERSION_FILE=" + proc}
	if scenario == "custom" {
		env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg/config"), "XDG_DATA_HOME="+filepath.Join(home, "xdg/data"), "XDG_STATE_HOME="+filepath.Join(home, "xdg/state"), "XDG_CACHE_HOME="+filepath.Join(home, "xdg/cache"))
	}
	return env
}

func configPaths(home, scenario string) (config, state string) {
	if scenario == "custom" {
		return filepath.Join(home, "xdg/config"), filepath.Join(home, "xdg/state")
	}
	return filepath.Join(home, ".config"), filepath.Join(home, ".local/state")
}

func runConfigScenario(t *testing.T, home, cli, release, scenario string, env []string) map[string]capture {
	t.Helper()
	config, state := configPaths(home, scenario)
	if scenario != "empty" {
		mustFS(t, os.MkdirAll(filepath.Join(config, "nvim"), 0700))
		mustFS(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export PERSONAL=kept\r\n"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(home, ".vimrc"), []byte("set number"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(config, "nvim/init.lua"), []byte("personal editor\x00bytes\n"), 0600))
		mustFS(t, os.WriteFile(filepath.Join(config, "starship.toml"), nil, 0600))
	}
	initial := mustSnapshot(t, home)
	captures := map[string]capture{}
	run := func(name string, status int, args ...string) capture {
		got, err := captureCommand(home, cli, args, env)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		requireStatus(t, name, got, status)
		captures[name] = got
		return got
	}
	run("help", 0, "help")
	run("version", 0, "version")
	if !bytes.Equal(initial, mustSnapshot(t, home)) {
		t.Fatal("help or version mutated HOME")
	}
	if scenario == "malformed-package" {
		file := filepath.Join(release, "packages.conf")
		f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("execute unsafe\n")
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		mustFS(t, err)
		got := run("malformed-install", 1, "install", "--skip-packages", "--yes")
		if !bytes.Equal(initial, got.Home) {
			t.Fatal("malformed package mutated HOME")
		}
		return captures
	}
	if scenario == "late-preflight" {
		f, err := os.OpenFile(filepath.Join(home, ".vimrc"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("\" >>> Selfishell vimrc >>>\n")
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		mustFS(t, err)
		before := mustSnapshot(t, home)
		got := run("blocked-install", 1, "install", "--skip-packages", "--yes")
		if !bytes.Equal(before, got.Home) {
			t.Fatal("late preflight mutated HOME")
		}
		return captures
	}
	dry := run("dry-run", 0, "install", "--skip-packages", "--dry-run", "--yes")
	if !bytes.Equal(initial, dry.Home) {
		t.Fatal("install dry-run mutated HOME")
	}
	installed := run("install", 0, "install", "--skip-packages", "--yes")
	for _, p := range []string{filepath.Join(state, "selfishell/configured"), filepath.Join(state, "selfishell/resources/user-zshrc.state")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Lstat(filepath.Join(config, "nvim")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("nvim link: %v %v", info, err)
	}
	reinstalled := run("reinstall", 0, "install", "--skip-packages", "--yes")
	if !bytes.Equal(installed.Home, reinstalled.Home) {
		t.Fatal("reinstall changed HOME")
	}
	if scenario == "purge" {
		t.Fatal("purge needs dedicated prefix fixture")
	}
	changed := false
	switch scenario {
	case "changed-file":
		mustFS(t, os.WriteFile(filepath.Join(config, "selfishell/zsh/history.zsh"), []byte("user changed managed content\n"), 0600))
		changed = true
	case "changed-link":
		mustFS(t, os.Remove(filepath.Join(config, "nvim")))
		mustFS(t, os.Symlink(filepath.Join(config, "starship.toml"), filepath.Join(config, "nvim")))
		changed = true
	case "changed-block":
		mustFS(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("user changed shell config\n"), 0600))
		changed = true
	case "pending":
		p := filepath.Join(state, "selfishell/resources/user-nvim.state")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		fields := bytes.Split(raw, []byte("\n"))
		if len(fields) < 4 || string(fields[2]) != "active" {
			t.Fatalf("unexpected state: %q", raw)
		}
		fields[2] = []byte("pending")
		next := p + ".next"
		mustFS(t, os.WriteFile(next, bytes.Join(fields, []byte("\n")), 0600))
		mustFS(t, os.Rename(next, p))
		mustFS(t, os.Remove(filepath.Join(config, "nvim")))
		run("recover", 0, "install", "--skip-packages", "--yes")
	}
	if changed {
		before := mustSnapshot(t, home)
		got := run("changed-uninstall", 1, "uninstall", "--restore", "--yes")
		if !bytes.Equal(before, got.Home) {
			t.Fatal("failed uninstall mutated HOME")
		}
		return captures
	}
	run("restore", 0, "uninstall", "--restore", "--yes")
	if scenario == "empty" {
		assertEmptyConfigRestored(t, home, config, state)
	} else {
		assertPersonalFilesRestored(t, home, config)
	}
	return captures
}

func assertPersonalFilesRestored(t *testing.T, home, config string) {
	t.Helper()
	for path, want := range map[string][]byte{
		filepath.Join(home, ".zshrc"):          []byte("export PERSONAL=kept\r\n"),
		filepath.Join(home, ".vimrc"):          []byte("set number"),
		filepath.Join(config, "nvim/init.lua"): []byte("personal editor\x00bytes\n"),
		filepath.Join(config, "starship.toml"): nil,
	} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("personal %s: %v %q", path, err, got)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			t.Fatalf("personal mode %s: %v %v", path, info, err)
		}
	}
}

func assertEmptyConfigRestored(t *testing.T, home, config, state string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(state, "selfishell/configured"),
		filepath.Join(state, "selfishell/ghostty"),
		filepath.Join(config, "nvim"),
		filepath.Join(config, "starship.toml"),
		filepath.Join(config, "mise/conf.d/selfishell.toml"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("managed path remains after restore: %s: %v", path, err)
		}
	}
	for _, dir := range []string{filepath.Join(state, "selfishell/resources"), filepath.Join(config, "selfishell")} {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || len(entries) != 0 {
			t.Fatalf("managed directory remains populated: %s: %v %v", dir, entries, err)
		}
	}
	for _, path := range []string{".zshrc", ".zprofile", ".zshenv", ".vimrc"} {
		file := filepath.Join(home, path)
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || len(data) != 0 {
			t.Fatalf("loader remains after restore: %s: %v %q", file, err, data)
		}
	}
}

func TestConfig(t *testing.T) { configScenarios(t) }

func TestConfigInvalidDependencies(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	mustFS(t, os.Mkdir(filepath.Join(release, ".git"), 0700))
	cli := filepath.Join(release, "bin/selfishell")
	mustFS(t, copyFile(candidate, cli))
	tools := fixtureTools(t, root)
	file := filepath.Join(release, "dependencies.conf")
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("execute unsafe\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	mustFS(t, err)
	home := filepath.Join(root, "home")
	mustFS(t, os.Mkdir(home, 0700))
	env := configEnv("macos", "empty", home, tools, "/unused", "/unused", "/unused")
	before := mustSnapshot(t, home)
	got, err := captureCommand(home, cli, []string{"install", "--skip-packages", "--yes"}, env)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, "invalid dependencies", got, 1)
	if !bytes.Contains(got.Stderr, []byte("invalid manifest record")) {
		t.Fatalf("missing manifest error: %q", got.Stderr)
	}
	if !bytes.Equal(before, got.Home) {
		t.Fatal("invalid dependencies mutated HOME")
	}
}

func TestConfigIdenticalExistingFile(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	release := filepath.Join(root, "release")
	mustFS(t, exportCommit(repoRoot(), referenceCommit, release))
	mustFS(t, os.Mkdir(filepath.Join(release, ".git"), 0700))
	cli := filepath.Join(release, "bin/selfishell")
	mustFS(t, copyFile(candidate, cli))
	tools := fixtureTools(t, root)
	home := filepath.Join(root, "home")
	target := filepath.Join(home, ".config/selfishell/zsh/history.zsh")
	source := filepath.Join(release, "config/shared/zsh/history.zsh")
	state := filepath.Join(home, ".local/state/selfishell/resources/zsh-history.state")
	mustFS(t, os.MkdirAll(filepath.Dir(target), 0700))
	mustFS(t, copyFile(source, target))
	mustFS(t, os.Chmod(target, 0600))
	env := configEnv("macos", "empty", home, tools, "/unused", "/unused", "/unused")
	run := func(args ...string) {
		got, err := captureCommand(home, cli, args, env)
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, strings.Join(args, " "), got, 0)
	}
	run("install", "--skip-packages", "--yes")
	raw, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(string(raw), "\n")
	if len(fields) < 7 || fields[5] == "-" {
		t.Fatalf("missing original backup: %q", raw)
	}
	backup := fields[5]
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(saved, sourceBytes) {
		t.Fatalf("backup: %v %q", err, saved)
	}
	run("install", "--skip-packages", "--yes")
	raw, err = os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(string(raw), "\n")[5]; got != backup {
		t.Fatalf("backup path changed: %q => %q", backup, got)
	}
	run("uninstall", "--restore", "--yes")
	restored, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(restored, sourceBytes) {
		t.Fatalf("restore: %v %q", err, restored)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("restore mode: %v %v", info, err)
	}
}

func TestConfigPurge(t *testing.T) {
	candidate, err := candidateCLI(t)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	tools := fixtureTools(t, root)
	prefix := filepath.Join(root, "prefix")
	home := filepath.Join(root, "home")
	export := filepath.Join(root, "export")
	var want map[string]capture
	for _, implementation := range []string{"bash", "go"} {
		mustFS(t, os.RemoveAll(home))
		mustFS(t, os.RemoveAll(prefix))
		mustFS(t, os.RemoveAll(export))
		mustFS(t, os.MkdirAll(home, 0700))
		mustFS(t, exportCommit(repoRoot(), referenceCommit, export))
		release := filepath.Join(prefix, "share/selfishell/releases/1.0")
		mustFS(t, copyTree(export, release))
		mustFS(t, os.RemoveAll(export))
		if _, err := os.Lstat(export); !os.IsNotExist(err) {
			t.Fatalf("source export remains: %v", err)
		}
		mustFS(t, os.Mkdir(filepath.Join(release, ".git"), 0700))
		if implementation == "go" {
			mustFS(t, copyFile(candidate, filepath.Join(release, "bin/selfishell")))
		}
		mustFS(t, os.MkdirAll(filepath.Join(prefix, "bin"), 0700))
		mustFS(t, os.Symlink("releases/1.0", filepath.Join(prefix, "share/selfishell/current")))
		mustFS(t, os.Symlink("../share/selfishell/current/bin/selfishell", filepath.Join(prefix, "bin/selfishell")))
		mustFS(t, os.Symlink("selfishell", filepath.Join(prefix, "bin/sfs")))
		cli := filepath.Join(prefix, "bin/selfishell")
		env := configEnv("macos", "existing", home, tools, "/unused", "/unused", "/unused")
		captures := runPurgeScenario(t, home, cli, prefix, env)
		if implementation == "bash" {
			want = captures
		} else {
			for name, expected := range want {
				requireEqual(t, name, expected, captures[name])
			}
		}
	}
}

func runPurgeScenario(t *testing.T, home, cli, prefix string, env []string) map[string]capture {
	t.Helper()
	config, _ := configPaths(home, "existing")
	mustFS(t, os.MkdirAll(filepath.Join(config, "nvim"), 0700))
	mustFS(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export PERSONAL=kept\r\n"), 0600))
	mustFS(t, os.WriteFile(filepath.Join(home, ".vimrc"), []byte("set number"), 0600))
	mustFS(t, os.WriteFile(filepath.Join(config, "nvim/init.lua"), []byte("personal editor\x00bytes\n"), 0600))
	mustFS(t, os.WriteFile(filepath.Join(config, "starship.toml"), nil, 0600))
	result := map[string]capture{}
	run := func(name string, args ...string) {
		got, err := captureCommand(home, cli, args, env)
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, name, got, 0)
		result[name] = got
	}
	run("help", "help")
	run("version", "version")
	initial := mustSnapshot(t, home)
	run("dry-run", "install", "--skip-packages", "--dry-run", "--yes")
	if !bytes.Equal(initial, result["dry-run"].Home) {
		t.Fatal("dry-run mutated HOME")
	}
	run("install", "install", "--skip-packages", "--yes")
	run("reinstall", "install", "--skip-packages", "--yes")
	if !bytes.Equal(result["install"].Home, result["reinstall"].Home) {
		t.Fatal("reinstall changed HOME")
	}
	before := mustSnapshot(t, prefix)
	run("purge-dry-run", "uninstall", "--restore", "--purge", "--dry-run", "--yes")
	after := mustSnapshot(t, prefix)
	if !bytes.Equal(before, after) {
		t.Fatal("purge dry-run changed prefix")
	}
	run("purge", "uninstall", "--restore", "--purge", "--yes")
	for _, p := range []string{filepath.Join(prefix, "bin/selfishell"), filepath.Join(prefix, "share/selfishell")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("purge retained %s: %v", p, err)
		}
	}
	prefixAfter := mustSnapshot(t, prefix)
	purged := result["purge"]
	purged.Home = append(purged.Home, prefixAfter...)
	result["purge"] = purged
	assertPersonalFilesRestored(t, home, config)
	return result
}
