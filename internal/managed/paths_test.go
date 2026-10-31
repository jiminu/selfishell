package managed

import (
	"os"
	"testing"
)

func TestPaths(t *testing.T) {
	t.Parallel()
	env := map[string]string{}
	getenv := func(name string) string { return env[name] }
	home := t.TempDir()
	env["HOME"] = home
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		env[k] = ""
	}
	paths, err := UserPaths(getenv)
	if err != nil || paths.Config != home+"/.config/selfishell" || paths.State != home+"/.local/state/selfishell" || paths.Cache != home+"/.cache/selfishell" || paths.Data != home+"/.local/share/selfishell" || paths.Resources != paths.State+"/resources" {
		t.Fatalf("paths=%+v, err=%v", paths, err)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		env[k] = home + "/custom dir"
	}
	paths, err = UserPaths(getenv)
	if err != nil || paths.Config != home+"/custom dir/selfishell" || paths.State != paths.Config || paths.Data != paths.Config || paths.Cache != paths.Config {
		t.Fatalf("override paths=%+v, err=%v", paths, err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("path discovery wrote files")
	}

	// A symlink followed by .. must be resolved by the filesystem, not cleaned
	// lexically: alias/.. selects physical/, not the lexical parent of alias.
	if err := os.MkdirAll(home+"/physical/child", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home+"/physical/child", home+"/alias"); err != nil {
		t.Fatal(err)
	}
	prefix := home + "/alias/.."
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		env[k] = prefix + "/custom"
	}
	paths, err = UserPaths(getenv)
	expected := Paths{Config: prefix + "/custom/selfishell", State: prefix + "/custom/selfishell", Resources: prefix + "/custom/selfishell/resources", Cache: prefix + "/custom/selfishell", Data: prefix + "/custom/selfishell"}
	if err != nil || paths != expected {
		t.Fatalf("XDG path meaning changed: got %+v want %+v (err=%v)", paths, expected, err)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		env[k] = ""
	}
	env["HOME"] = prefix
	paths, err = UserPaths(getenv)
	expected = Paths{Config: prefix + "/.config/selfishell", State: prefix + "/.local/state/selfishell", Resources: prefix + "/.local/state/selfishell/resources", Cache: prefix + "/.cache/selfishell", Data: prefix + "/.local/share/selfishell"}
	if err != nil || paths != expected {
		t.Fatalf("HOME path meaning changed: got %+v want %+v (err=%v)", paths, expected, err)
	}
	children, err := os.ReadDir(home + "/physical")
	if err != nil || len(children) != 1 {
		t.Fatalf("path discovery created targets: %v %v", children, err)
	}
	env["HOME"] = ""
	if _, err := UserPaths(getenv); err == nil {
		t.Fatal("empty HOME accepted")
	}
}
