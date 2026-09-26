package selfishell

import (
	"context"
	"os"
	"strings"
	"testing"
)

func miseFixture(t *testing.T) (*PackageOperation, Paths, string, string) {
	t.Helper()
	op, paths, _, home := dependencyFixture(t)
	root := home + "/release"
	writeTestFile(t, root+"/config/shared/mise.toml", "[tools]\nnode = \"24.18.0\"\n", 0600)
	bin := home + "/bin"
	writeTestFile(t, bin+"/mise", `#!/bin/sh
printf '%s|%s|%s|%s|%s\n' "$PWD" "$MISE_GLOBAL_CONFIG_FILE" "${MISE_OFFLINE-unset}" "$MISE_TRUSTED_CONFIG_PATHS" "$*" >> "$MISE_LOG"
case "$*" in
  *'install --dry-run-code'*) exit "${DRY_CODE_EXIT:-1}" ;;
  *'install '* ) exit "${INSTALL_EXIT:-0}" ;;
esac
`, 0755)
	t.Setenv("MISE_LOG", home+"/mise.log")
	op.Process.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin", "MISE_LOG="+home+"/mise.log")
	return op, paths, root, home
}

func TestMiseInstallUsesReleasePinsAndOfflineCheck(t *testing.T) {
	op, paths, root, home := miseFixture(t)
	if err := op.InstallMise(context.Background(), root, paths, "required", false, "node@24.18.0", "python@3.13.14"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(home + "/mise.log")
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("mise calls: %s", data)
	}
	for _, line := range lines {
		if !strings.Contains(line, "|"+root+"/config/shared/mise.toml|") || !strings.HasSuffix(strings.Split(line, "|")[0], "/release/config/shared") {
			t.Fatalf("wrong release scope: %s", line)
		}
	}
	if !strings.Contains(lines[0], "|1|") || !strings.HasSuffix(lines[0], "-C "+root+"/config/shared -q install --dry-run-code node@24.18.0 python@3.13.14") {
		t.Fatalf("check: %s", lines[0])
	}
	if !strings.Contains(lines[1], "|unset|") || !strings.HasSuffix(lines[1], "-C "+root+"/config/shared install node@24.18.0 python@3.13.14") {
		t.Fatalf("install: %s", lines[1])
	}
}

func TestMiseInstallSkipOptionalAndDryRun(t *testing.T) {
	op, paths, root, home := miseFixture(t)
	if err := op.InstallMise(context.Background(), root, paths, "required", true, "node"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("dry-run invoked mise: %v", err)
	}
	op.Process.Env = append(op.Process.Env, "INSTALL_EXIT=1")
	if err := op.InstallMise(context.Background(), root, paths, "optional", false, "node", "python"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(op.SkippedOptional, ",") != "node,python" {
		t.Fatalf("skipped: %v", op.SkippedOptional)
	}
	if err := os.Remove(home + "/mise.log"); err != nil {
		t.Fatal(err)
	}
	op.SkippedOptional = nil
	op.Process.Env = append(op.Process.Env, "DRY_CODE_EXIT=0", "INSTALL_EXIT=0")
	if err := op.InstallMise(context.Background(), root, paths, "required", false, "node"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(home + "/mise.log")
	if strings.Count(string(data), "\n") != 1 {
		t.Fatalf("installed after successful check: %s", data)
	}
}

func TestMiseInstallFallsBackToManagedBinary(t *testing.T) {
	op, paths, root, home := miseFixture(t)
	if err := os.MkdirAll(home+"/.local/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(home+"/bin/mise", home+"/.local/bin/mise"); err != nil {
		t.Fatal(err)
	}
	op.Process.Env = append(os.Environ(), "HOME="+home, "PATH=/usr/bin:/bin", "MISE_LOG="+home+"/mise.log")
	if err := op.InstallMise(context.Background(), root, paths, "required", false, "node"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/mise.log"); err != nil {
		t.Fatal(err)
	}
}

func TestMiseMissingAndCancellation(t *testing.T) {
	op, paths, root, home := miseFixture(t)
	if err := os.Remove(home + "/bin/mise"); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallMise(context.Background(), root, paths, "optional", false, "node", "python"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(op.SkippedOptional, ",") != "node,python" {
		t.Fatalf("missing optional: %v", op.SkippedOptional)
	}
	if err := op.InstallMise(context.Background(), root, paths, "required", false, "node"); err == nil {
		t.Fatal("missing required mise accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := op.InstallMise(ctx, root, paths, "optional", false, "node"); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("mise called: %v", err)
	}
}

func TestMiseTrustsExistingManagedConfigLink(t *testing.T) {
	op, paths, root, home := miseFixture(t)
	link := home + "/.config/mise/conf.d/selfishell.toml"
	writeTestFile(t, link, "[tools]\n", 0600)
	if err := op.InstallMise(context.Background(), root, paths, "required", false, "node"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(home + "/mise.log")
	if !strings.Contains(string(data), "trust "+link) {
		t.Fatalf("managed config not trusted: %s", data)
	}
}
