package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCIChangeClassification(t *testing.T) {
	cases := []struct {
		name            string
		files           map[string]string
		runtime, ubuntu bool
		missingBase     bool
	}{
		{"documentation_guide", map[string]string{"docs/guide.md": "Documentation update.\n"}, false, false, false},
		{"documentation_contributing", map[string]string{"CONTRIBUTING.md": "Contribution guide.\n"}, false, false, false},
		{"documentation_security", map[string]string{"SECURITY.md": "Security guide.\n"}, false, false, false},
		{"documentation_license", map[string]string{"LICENSE": "License update.\n"}, false, false, false},
		{"image_only", map[string]string{"img/social-preview.png": "Preview fixture.\n"}, false, false, false},
		{"mixed_documentation_image_runtime", map[string]string{"img/social-preview.png": "Preview fixture.\n", "CONTRIBUTING.md": "Contribution guide.\n", "install.sh": "#!/usr/bin/env bash\nprintf 'updated\\n'\n"}, true, true, false},
		{"neovim_configuration_only", map[string]string{"config/shared/nvim/init.lua": "print(\"updated\")\n"}, true, false, false},
		{"neovim_dependency_only", map[string]string{"dependencies.conf": classificationDeps("2222222222222222222222222222222222222222", "1.0")}, true, false, false},
		{"non_neovim_dependency", map[string]string{"dependencies.conf": classificationDeps("1111111111111111111111111111111111111111", "2.0")}, true, true, false},
		{"mixed_neovim_runtime", map[string]string{"config/shared/nvim/init.lua": "print(\"updated\")\n", "install.sh": "#!/usr/bin/env bash\nprintf 'updated\\n'\n"}, true, true, false},
		{"missing_base", nil, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, repo, base := policyRepo(t)
			for path, content := range tc.files {
				policyWrite(t, filepath.Join(repo, path), content)
			}
			head := base
			if !tc.missingBase {
				policyGit(t, home, repo, "add", ".")
				policyGit(t, home, repo, "commit", "-qm", "change")
				head = strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
			} else {
				base = "missing"
				head = "HEAD"
			}
			got, err := runCommandIn(home, repo, []string{"/bin/bash", filepath.Join(repoRoot(), "scripts", "classify-ci-changes.sh"), base, head}, nil, policyEnv(home), 5*time.Second)
			if err != nil || got.Status != 0 {
				t.Fatalf("classifier: status=%d err=%v stderr=%s", got.Status, err, got.Stderr)
			}
			want := fmt.Sprintf("runtime=%t\nubuntu_container_e2e=%t\n", tc.runtime, tc.ubuntu)
			if string(got.Stdout) != want {
				t.Errorf("classifier output: got %q want %q", got.Stdout, want)
			}
		})
	}
}

func classificationDeps(plugin, starship string) string {
	return "download starship " + starship + " linux amd64 https://example.invalid/starship old .local/bin/starship raw\nnvim-plugin example/plugin " + plugin + " all all https://example.invalid/plugin.git - - -\n"
}
func policyEnv(home string) []string {
	return append(maintenanceMiseEnv(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file", "GIT_TEMPLATE_DIR="+filepath.Join(home, "empty-git-template"))
}
func policyWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func policyGit(t *testing.T, home, dir string, args ...string) string {
	t.Helper()
	got, err := runCommandIn(home, dir, append([]string{"/usr/bin/git"}, args...), nil, policyEnv(home), 5*time.Second)
	if err != nil || got.Status != 0 {
		t.Fatalf("git %v: status=%d err=%v stderr=%s", args, got.Status, err, got.Stderr)
	}
	return string(got.Stdout)
}
func policyRepo(t *testing.T) (home, repo, base string) {
	t.Helper()
	home = t.TempDir()
	repo = filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "empty-git-template"), 0700); err != nil {
		t.Fatal(err)
	}
	policyGit(t, home, repo, "init", "-q", "-b", "main")
	policyGit(t, home, repo, "config", "user.name", "Selfishell Tests")
	policyGit(t, home, repo, "config", "user.email", "tests@selfishell.invalid")
	policyGit(t, home, repo, "config", "core.hooksPath", filepath.Join(home, "empty-git-template"))
	policyWrite(t, filepath.Join(repo, "README.md"), "# Selfishell\n")
	policyWrite(t, filepath.Join(repo, "config/shared/nvim/init.lua"), "print(\"initial\")\n")
	policyWrite(t, filepath.Join(repo, "install.sh"), "#!/usr/bin/env bash\n")
	policyWrite(t, filepath.Join(repo, "dependencies.conf"), classificationDeps("1111111111111111111111111111111111111111", "1.0"))
	policyGit(t, home, repo, "add", ".")
	policyGit(t, home, repo, "commit", "-qm", "initial")
	base = strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
	return
}

var pinRE = regexp.MustCompile(`(?m)^\s*(?:-\s*)?uses:\s*[^\s@]+@[0-9a-f]{40}\s+#\s+v[0-9]+\.[0-9]+\.[0-9]+\s*$`)

func TestWorkflowActionPins(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoRoot(), ".github/workflows/*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("workflow glob: %v, %d", err, len(files))
	}
	count := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "uses:") {
				count++
				if !pinRE.MatchString(line) {
					t.Errorf("%s: invalid action pin: %q", filepath.Base(file), line)
				}
			}
		}
	}
	if count == 0 {
		t.Error("no workflow actions found")
	}
}
func TestDependabotTracksGitHubActions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(), ".github/dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^  - package-ecosystem: ["']?github-actions["']?[ \t]*$`).Match(data) {
		t.Error("Dependabot does not track github-actions")
	}
}

// workflowSections parses only the job and step indentation used by this repository.
// Duplicate names, missing boundaries, or a changed layout fail the policy tests.
type workflowJob struct {
	name  string
	lines []string
	steps map[string][]string
}

func workflowSections(t *testing.T, file string) (string, map[string]workflowJob) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(), ".github/workflows", file))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	jobs := map[string]workflowJob{}
	var current string
	inJobs := false
	jobLine := regexp.MustCompile(`^  ([a-zA-Z0-9_-]+):\s*$`)
	for _, line := range strings.Split(raw, "\n") {
		if line == "jobs:" {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		if line != "" && !strings.HasPrefix(line, "  ") {
			inJobs = false
			current = ""
			continue
		}
		if match := jobLine.FindStringSubmatch(line); match != nil {
			if _, ok := jobs[match[1]]; ok {
				t.Fatalf("duplicate job %s in %s", match[1], file)
			}
			current = match[1]
			jobs[current] = workflowJob{name: current, steps: map[string][]string{}}
			continue
		}
		if current != "" {
			if line != "" && !strings.HasPrefix(line, "    ") {
				current = ""
				continue
			}
			j := jobs[current]
			j.lines = append(j.lines, line)
			jobs[current] = j
		}
	}
	if len(jobs) == 0 {
		t.Fatalf("no jobs parsed from %s", file)
	}
	stepLine := regexp.MustCompile(`^      - name: (.+)$`)
	for name, j := range jobs {
		var step string
		for _, line := range j.lines {
			if m := stepLine.FindStringSubmatch(line); m != nil {
				step = m[1]
				if _, ok := j.steps[step]; ok {
					t.Fatalf("duplicate step %s in %s/%s", step, file, name)
				}
				j.steps[step] = []string{}
				continue
			}
			if step != "" {
				j.steps[step] = append(j.steps[step], line)
			}
		}
		jobs[name] = j
	}
	return raw, jobs
}
func policyJob(t *testing.T, jobs map[string]workflowJob, name string) workflowJob {
	t.Helper()
	j, ok := jobs[name]
	if !ok {
		t.Fatalf("missing workflow job %s", name)
	}
	return j
}
func policyStep(t *testing.T, j workflowJob, name string) []string {
	t.Helper()
	s, ok := j.steps[name]
	if !ok {
		t.Fatalf("missing workflow step %s/%s", j.name, name)
	}
	return s
}
func policyRun(t *testing.T, step []string) string {
	t.Helper()
	start := -1
	for i, line := range step {
		if line == "        run: |" {
			if start >= 0 {
				t.Fatal("ambiguous run blocks")
			}
			start = i
		}
	}
	if start < 0 {
		t.Fatal("missing run block")
	}
	var lines []string
	for _, line := range step[start+1:] {
		if line != "" && !strings.HasPrefix(line, "          ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	if len(lines) == 0 {
		t.Fatal("empty run block")
	}
	return strings.Join(lines, "\n") + "\n"
}
func policyField(t *testing.T, lines []string, indent, key string) string {
	t.Helper()
	prefix := indent + key + ":"
	var vals []string
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			vals = append(vals, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		}
	}
	if len(vals) != 1 {
		t.Fatalf("want one %s field, got %v", key, vals)
	}
	return vals[0]
}
func TestReleaseWorkflowEventSHAAndGraph(t *testing.T) {
	raw, jobs := workflowSections(t, "release.yml")
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "permissions:") {
			t.Fatal("release workflow has top-level permissions")
		}
	}
	verify := policyJob(t, jobs, "verify")
	build := policyJob(t, jobs, "build")
	smoke := policyJob(t, jobs, "smoke")
	publish := policyJob(t, jobs, "publish")
	if policyField(t, verify.lines, "    ", "permissions") != "" || policyField(t, publish.lines, "    ", "permissions") != "" {
		t.Fatal("job permissions must be maps")
	}
	for _, tc := range []struct {
		job        workflowJob
		name, perm string
	}{{verify, "verify", "contents: read"}, {build, "build", "contents: read"}, {smoke, "smoke", "contents: read"}, {publish, "publish", "contents: write"}} {
		found := false
		for _, line := range tc.job.lines {
			if line == "      "+tc.perm {
				found = true
			}
		}
		if !found {
			t.Errorf("%s missing %s", tc.name, tc.perm)
		}
		checkout := policyStep(t, tc.job, "Check out release commit")
		text := strings.Join(checkout, "\n")
		for _, needle := range []string{"uses: actions/checkout@", "ref: ${{ github.sha }}", "fetch-depth: 0"} {
			if !strings.Contains(text, needle) {
				t.Errorf("%s checkout missing %s", tc.name, needle)
			}
		}
	}
	for _, j := range []workflowJob{verify, build, smoke} {
		for _, line := range j.lines {
			if strings.HasPrefix(line, "      ") && (strings.Contains(line, "attestations: write") || strings.Contains(line, "id-token: write") || strings.Contains(line, "artifact-metadata: write") || strings.Contains(line, "contents: write")) {
				t.Errorf("%s has publish permission: %s", j.name, line)
			}
		}
	}
	for _, perm := range []string{"attestations: write", "id-token: write", "artifact-metadata: write"} {
		if !strings.Contains(strings.Join(publish.lines, "\n"), "      "+perm) {
			t.Errorf("publish missing %s", perm)
		}
	}
	if !strings.Contains(strings.Join(verify.lines, "\n"), "os: [ubuntu-latest, macos-latest]") {
		t.Error("verification must cover Linux and macOS")
	}
	if !strings.Contains(policyRun(t, policyStep(t, build, "Require release commit on main")), `git merge-base --is-ancestor "$GITHUB_SHA" origin/main`) {
		t.Error("release does not check main ancestry")
	}
	guard := policyRun(t, policyStep(t, publish, "Publish immutable GitHub Release"))
	if !strings.Contains(guard, `gh release view "$TAG"`) || !strings.Contains(guard, `gh release create "$TAG" dist/* --verify-tag`) {
		t.Error("immutable release guard or verified publication missing")
	}
	for _, tc := range []struct {
		job           workflowJob
		before, after string
	}{{build, "Require release commit on main", "Build release artifacts"}, {build, "Resolve release version", "Build release artifacts"}, {publish, "Verify downloaded artifacts", "Generate signed build provenance"}, {publish, "Generate signed build provenance", "Publish immutable GitHub Release"}} {
		order := strings.Join(tc.job.lines, "\n")
		if strings.Index(order, "- name: "+tc.before) < 0 || strings.Index(order, "- name: "+tc.before) >= strings.Index(order, "- name: "+tc.after) {
			t.Errorf("wrong release step order: %s %s before %s", tc.job.name, tc.before, tc.after)
		}
	}
	// Traverse every prerequisite, rejecting unknown jobs and cycles. Publish must
	// depend transitively on verification even if intermediate jobs are added.
	visiting := map[string]bool{}
	done := map[string]bool{}
	reaches := map[string]map[string]bool{}
	var visit func(string) map[string]bool
	visit = func(name string) map[string]bool {
		if visiting[name] {
			t.Fatalf("release job cycle at %s", name)
		}
		if done[name] {
			return reaches[name]
		}
		j, ok := jobs[name]
		if !ok {
			t.Fatalf("unknown release prerequisite %s", name)
		}
		visiting[name] = true
		found := map[string]bool{name: true}
		for _, dep := range policyNeeds(t, j.lines) {
			for ancestor := range visit(dep) {
				found[ancestor] = true
			}
		}
		visiting[name] = false
		done[name] = true
		reaches[name] = found
		return found
	}
	for _, pair := range [][2]string{{"build", "verify"}, {"smoke", "build"}, {"publish", "verify"}, {"publish", "build"}, {"publish", "smoke"}} {
		if !visit(pair[0])[pair[1]] {
			t.Errorf("%s has no transitive dependency on %s", pair[0], pair[1])
		}
	}
}

func TestReleaseWorkflowArtifactHandoff(t *testing.T) {
	_, jobs := workflowSections(t, "release.yml")
	build, smoke, publish := policyJob(t, jobs, "build"), policyJob(t, jobs, "smoke"), policyJob(t, jobs, "publish")
	for _, stage := range []struct{ job, command string }{
		{"verify", "Run verification suite"},
		{"build", "Build release artifacts"},
		{"smoke", "Smoke exact prebuilt release"},
	} {
		job := policyJob(t, jobs, stage.job)
		setup := strings.Join(policyStep(t, job, "Set up pinned Go toolchain"), "\n")
		if !strings.Contains(setup, "uses: actions/setup-go@") || !strings.Contains(setup, "go-version-file: go.mod") {
			t.Errorf("%s must select the Go toolchain from go.mod", stage.job)
		}
		order := strings.Join(job.lines, "\n")
		if strings.Index(order, "- name: Set up pinned Go toolchain") >= strings.Index(order, "- name: "+stage.command) {
			t.Errorf("%s selects Go after using it", stage.job)
		}
	}
	var all string
	for _, j := range jobs {
		all += strings.Join(j.lines, "\n")
	}
	if strings.Count(all, "bash scripts/build-release.sh") != 1 || !strings.Contains(policyRun(t, policyStep(t, build, "Build release artifacts")), `--version "$VERSION" --output dist`) {
		t.Error("release must build once from validated version")
	}
	versionStep := strings.Join(policyStep(t, build, "Resolve release version"), "\n")
	for _, needle := range []string{"PUSHED_TAG: ${{ github.ref_name }}", "selfishell_version_is_valid", `printf 'version=%s\n' "$version" >>"$GITHUB_OUTPUT"`} {
		if !strings.Contains(versionStep, needle) {
			t.Errorf("version resolution missing %s", needle)
		}
	}
	if !strings.Contains(strings.Join(smoke.lines, "\n"), "os: [ubuntu-latest, macos-latest]") {
		t.Error("exact smoke must run on Linux and macOS")
	}
	upload := strings.Join(policyStep(t, build, "Upload release artifacts"), "\n")
	for _, needle := range []string{"actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a", "name: release-assets-${{ github.run_id }}-${{ github.run_attempt }}", "path: dist/", "if-no-files-found: error"} {
		if !strings.Contains(upload, needle) {
			t.Errorf("upload missing %s", needle)
		}
	}
	if !strings.Contains(strings.Join(build.lines, "\n"), "steps.upload.outputs.artifact-id") {
		t.Error("build does not expose upload ID")
	}
	for _, j := range []workflowJob{smoke, publish} {
		download := strings.Join(policyStep(t, j, "Download release artifacts"), "\n")
		for _, needle := range []string{"actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c", "artifact-ids: ${{ needs.build.outputs.artifact_id }}", "path: dist", "digest-mismatch: error"} {
			if !strings.Contains(download, needle) {
				t.Errorf("%s download missing %s", j.name, needle)
			}
		}
		if strings.Contains(strings.Join(j.lines, "\n"), "bash scripts/build-release.sh") {
			t.Errorf("%s rebuilds artifacts", j.name)
		}
	}
	smokeText := strings.Join(smoke.lines, "\n")
	for _, needle := range []string{"SELFISHELL_TEST_RELEASE_DIR: ${{ github.workspace }}/dist", "SELFISHELL_TEST_RELEASE_VERSION: ${{ needs.build.outputs.version }}", "go test ./tests/integration -run '^TestExactReleaseSmoke$' -count=1"} {
		if !strings.Contains(smokeText, needle) {
			t.Errorf("smoke missing %s", needle)
		}
	}
	if !strings.Contains(policyRun(t, policyStep(t, publish, "Verify downloaded artifacts")), `sha256sum -c SHA256SUMS`) {
		t.Error("publisher does not check downloaded bytes")
	}
	if !strings.Contains(strings.Join(policyStep(t, publish, "Generate signed build provenance"), "\n"), "subject-path: dist/*") {
		t.Error("attestation does not cover downloaded assets")
	}
	if !strings.Contains(strings.Join(policyStep(t, publish, "Publish immutable GitHub Release"), "\n"), "TAG: ${{ needs.build.outputs.tag }}") {
		t.Error("publication tag does not come from validated build")
	}
	if strings.Index(strings.Join(publish.lines, "\n"), "- name: Generate signed build provenance") > strings.Index(strings.Join(publish.lines, "\n"), "- name: Publish immutable GitHub Release") {
		t.Error("attestation follows publication")
	}
}

func TestCIExactPrebuiltSmoke(t *testing.T) {
	_, jobs := workflowSections(t, "ci.yml")
	shell := policyJob(t, jobs, "shell")
	if !strings.Contains(strings.Join(shell.lines, "\n"), `'["ubuntu-latest","macos-latest"]'`) {
		t.Error("runtime CI must run exact smoke on Linux and macOS")
	}
	build := policyRun(t, policyStep(t, shell, "Build disposable release artifacts"))
	smoke := strings.Join(policyStep(t, shell, "Smoke exact prebuilt release"), "\n")
	if !strings.Contains(build, `bash scripts/build-release.sh --version 0.0.0-ci --output "$RUNNER_TEMP/selfishell-release-assets"`) {
		t.Error("CI does not build disposable exact assets")
	}
	for _, needle := range []string{"SELFISHELL_TEST_RELEASE_DIR: ${{ runner.temp }}/selfishell-release-assets", "SELFISHELL_TEST_RELEASE_VERSION: 0.0.0-ci", "go test ./tests/integration -run '^TestExactReleaseSmoke$' -count=1"} {
		if !strings.Contains(smoke, needle) {
			t.Errorf("CI smoke missing %s", needle)
		}
	}
	order := strings.Join(shell.lines, "\n")
	if strings.Index(order, "- name: Run checks") >= strings.Index(order, "- name: Build disposable release artifacts") {
		t.Error("CI exact smoke must follow repository gate")
	}
}
func policyNeeds(t *testing.T, lines []string) []string {
	t.Helper()
	var values []string
	for _, line := range lines {
		if strings.HasPrefix(line, "    needs:") {
			values = append(values, strings.TrimSpace(strings.TrimPrefix(line, "    needs:")))
		}
	}
	if len(values) > 1 {
		t.Fatalf("ambiguous needs fields: %v", values)
	}
	if len(values) == 0 {
		return nil
	}
	value := values[0]
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] == "" {
				t.Fatal("empty release prerequisite")
			}
		}
		return parts
	}
	if value == "" || strings.ContainsAny(value, "[] ,") {
		t.Fatalf("unsupported release prerequisite form: %q", value)
	}
	return []string{value}
}

func TestDependencyWorkflowChanges(t *testing.T) {
	raw, jobs := workflowSections(t, "dependency-updates.yml")
	job := policyJob(t, jobs, "update")
	detect := policyStep(t, job, "Check dependency changes")
	if id := policyField(t, detect, "        ", "id"); id != "changes" {
		t.Fatalf("change output is not connected to downstream steps: %q", id)
	}
	previous := -1
	for _, name := range []string{"Update dependency manifest", "Check dependency changes", "Install shell tooling", "Validate updated manifest", "Create or refresh dependency update PR"} {
		index := strings.Index(raw, "      - name: "+name+"\n")
		if index <= previous {
			t.Fatalf("dependency step %q missing or out of order", name)
		}
		previous = index
	}
	for _, name := range []string{"Install shell tooling", "Validate updated manifest", "Create or refresh dependency update PR"} {
		if condition := policyField(t, policyStep(t, job, name), "        ", "if"); condition != "steps.changes.outputs.changed == 'true'" {
			t.Errorf("%s does not require dependency changes: %q", name, condition)
		}
	}
	block := policyRun(t, detect)
	for _, tc := range []struct {
		name, path string
		status     int
		output     string
	}{
		{"unchanged", "", 0, "changed=false\n"},
		{"manifest", "dependencies.conf", 0, "changed=true\n"},
		{"completion", "config/shared/zsh/completion.zsh", 0, "changed=true\n"},
		{"interactive", "config/shared/zsh/interactive.zsh", 0, "changed=true\n"},
		{"mise", "config/shared/mise.toml", 0, "changed=true\n"},
		{"unexpected_tracked_only", "README.md", 1, ""},
		{"unexpected_untracked_only", "unexpected.txt", 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, repo, _ := policyRepo(t)
			for _, path := range []string{"config/shared/zsh/completion.zsh", "config/shared/zsh/interactive.zsh", "config/shared/mise.toml"} {
				policyWrite(t, filepath.Join(repo, path), "# fixture\n")
			}
			policyGit(t, home, repo, "add", ".")
			policyGit(t, home, repo, "commit", "-qm", "tracked dependency files")
			if tc.path != "" {
				policyWrite(t, filepath.Join(repo, tc.path), "changed\n")
			}
			output := filepath.Join(home, "output")
			policyWrite(t, output, "")
			got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, append(policyEnv(home), "GITHUB_OUTPUT="+output), 5*time.Second)
			if err != nil || got.Status != tc.status {
				t.Fatalf("change detection: status=%d want=%d err=%v stderr=%s", got.Status, tc.status, err, got.Stderr)
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != tc.output {
				t.Fatalf("change output=%q want=%q err=%v", data, tc.output, err)
			}
		})
	}
}

func TestDependencyWorkflowPRBlock(t *testing.T) {
	_, jobs := workflowSections(t, "dependency-updates.yml")
	block := policyRun(t, policyStep(t, policyJob(t, jobs, "update"), "Create or refresh dependency update PR"))
	for _, tc := range []struct {
		name             string
		open, unexpected bool
	}{{"skip_when_open", true, false}, {"create_when_absent", false, false}, {"reject_unexpected_path", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			home, repo, _ := policyRepo(t)
			for _, path := range []string{"config/shared/zsh/completion.zsh", "config/shared/zsh/interactive.zsh", "config/shared/mise.toml"} {
				policyWrite(t, filepath.Join(repo, path), "# fixture\n")
			}
			policyGit(t, home, repo, "add", ".")
			policyGit(t, home, repo, "commit", "-qm", "tracked workflow files")
			policyWrite(t, filepath.Join(repo, "dependencies.conf"), classificationDeps("2222222222222222222222222222222222222222", "1.0"))
			if tc.unexpected {
				policyWrite(t, filepath.Join(repo, "README.md"), "unrelated change\n")
			}
			before := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
			fake := filepath.Join(home, "fakebin")
			if err := os.MkdirAll(fake, 0700); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(home, "gh.log")
			mode := "0"
			if tc.open {
				mode = "1"
			}
			gh := `#!/bin/bash
printf '%s\n' "$*" >>"$POLICY_GH_LOG"
if [[ "$1 $2" == 'pr list' ]]; then printf '%s\n' "$POLICY_OPEN_COUNT"; exit 0; fi
if [[ "$1 $2" == 'pr create' ]]; then exit 0; fi
exit 90
`
			git := `#!/bin/bash
if [[ "$1" == fetch || "$1" == push ]]; then printf 'git %s\n' "$*" >>"$POLICY_GH_LOG"; exit 0; fi
exec /usr/bin/git "$@"
`
			policyWrite(t, filepath.Join(fake, "gh"), gh)
			policyWrite(t, filepath.Join(fake, "git"), git)
			for _, tool := range []string{"gh", "git"} {
				if err := os.Chmod(filepath.Join(fake, tool), 0700); err != nil {
					t.Fatal(err)
				}
			}
			env := append(policyEnv(home), "PATH="+fake+":/usr/bin:/bin:/usr/sbin:/sbin", "BRANCH=automation/dependency-updates", "POLICY_GH_LOG="+log, "POLICY_OPEN_COUNT="+mode)
			got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, env, 10*time.Second)
			if tc.unexpected {
				if err != nil || got.Status != 1 || !strings.Contains(string(got.Stderr), "Dependency update touched unexpected files:") {
					t.Fatalf("unexpected path not rejected: status=%d err=%v stdout=%s stderr=%s", got.Status, err, got.Stdout, got.Stderr)
				}
				if after := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD")); after != before {
					t.Errorf("workflow committed unexpected path: before=%s after=%s", before, after)
				}
				if branch := strings.TrimSpace(policyGit(t, home, repo, "branch", "--show-current")); branch != "main" {
					t.Errorf("workflow switched branch before rejecting: %s", branch)
				}
				if staged := strings.TrimSpace(policyGit(t, home, repo, "diff", "--cached", "--name-only")); staged != "" {
					t.Errorf("workflow staged paths before rejecting: %s", staged)
				}
				if calls, readErr := os.ReadFile(log); readErr == nil {
					t.Errorf("workflow called fake git/gh before rejecting: %s", calls)
				} else if !os.IsNotExist(readErr) {
					t.Fatal(readErr)
				}
				t.Log("current PR block rejected unrelated tracked change with status 1 before commit, push, or gh")
				return
			}
			if err != nil || got.Status != 0 {
				t.Fatalf("PR block: status=%d err=%v stdout=%s stderr=%s", got.Status, err, got.Stdout, got.Stderr)
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			calls := string(data)
			if !strings.Contains(calls, "pr list --head automation/dependency-updates --state open --json number --jq length") {
				t.Errorf("wrong PR lookup: %s", calls)
			}
			created := strings.Contains(calls, "pr create ")
			if created == tc.open {
				t.Errorf("create=%t for open=%t: %s", created, tc.open, calls)
			}
			if !strings.Contains(calls, "git push --force-with-lease origin HEAD:automation/dependency-updates") {
				t.Errorf("missing guarded branch push: %s", calls)
			}
			if created {
				for _, s := range []string{"--base main", "--head automation/dependency-updates", "Review release notes and CI results before merging", "never merges or releases automatically"} {
					if !strings.Contains(calls, s) {
						t.Errorf("PR call missing %q: %s", s, calls)
					}
				}
			}
			if strings.Contains(calls, "pr merge") || strings.Contains(calls, "release create") {
				t.Errorf("unexpected publish operation: %s", calls)
			}
			staged := strings.TrimSpace(policyGit(t, home, repo, "show", "--format=", "--name-only", "HEAD"))
			if staged != "dependencies.conf" {
				t.Errorf("unexpected staged files: %q", staged)
			}
		})
	}
}

func TestCIWorkflowUsesTrustedBaseClassifier(t *testing.T) {
	_, jobs := workflowSections(t, "ci.yml")
	block := policyRun(t, policyStep(t, policyJob(t, jobs, "changes"), "Detect runtime changes"))
	home, repo, _ := policyRepo(t)
	script := filepath.Join(repo, "scripts/classify-ci-changes.sh")
	policyWrite(t, script, "#!/usr/bin/env bash\nprintf 'runtime=true\\nubuntu_container_e2e=true\\n'\n")
	policyGit(t, home, repo, "add", ".")
	policyGit(t, home, repo, "commit", "-qm", "trusted classifier")
	base := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
	policyWrite(t, script, "#!/usr/bin/env bash\nprintf 'runtime=false\\nubuntu_container_e2e=false\\n'\n")
	policyGit(t, home, repo, "add", ".")
	policyGit(t, home, repo, "commit", "-qm", "tampered classifier")
	head := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
	output := filepath.Join(home, "github-output")
	summary := filepath.Join(home, "github-summary")
	env := append(policyEnv(home), "EVENT_NAME=pull_request", "BASE_SHA="+base, "GITHUB_SHA="+head, "GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+summary)
	got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, env, 5*time.Second)
	if err != nil || got.Status != 0 {
		t.Fatalf("CI block: status=%d err=%v stderr=%s", got.Status, err, got.Stderr)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "runtime=true\nubuntu_container_e2e=true\n" {
		t.Errorf("untrusted classifier output: %q", data)
	}
	sum, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sum), string(data)) {
		t.Errorf("summary lacks trusted classification: %q", sum)
	}
}

func TestReleaseWorkflowMainAncestryAndImmutableGuard(t *testing.T) {
	_, jobs := workflowSections(t, "release.yml")
	build := policyJob(t, jobs, "build")
	publish := policyJob(t, jobs, "publish")
	home, repo, base := policyRepo(t)
	policyGit(t, home, repo, "update-ref", "refs/remotes/origin/main", base)
	policyWrite(t, filepath.Join(repo, "docs/off-main.md"), "off main\n")
	policyGit(t, home, repo, "add", ".")
	policyGit(t, home, repo, "commit", "-qm", "off main")
	offMain := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
	ancestry := policyRun(t, policyStep(t, build, "Require release commit on main"))
	for _, tc := range []struct {
		name, sha string
		status    int
	}{{"on_main", base, 0}, {"outside_main", offMain, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", ancestry}, nil, append(policyEnv(home), "GITHUB_SHA="+tc.sha), 5*time.Second)
			if err != nil || got.Status != tc.status {
				t.Fatalf("main ancestry: status=%d want=%d err=%v stderr=%s", got.Status, tc.status, err, got.Stderr)
			}
		})
	}
	fake := filepath.Join(home, "fakebin")
	if err := os.MkdirAll(fake, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(home, "gh.log")
	policyWrite(t, filepath.Join(fake, "gh"), `#!/bin/bash
printf '%s\n' "$*" >>"$POLICY_GH_LOG"
if [[ "$1 $2" == 'release view' ]]; then exit 0; fi
exit 90
`)
	if err := os.Chmod(filepath.Join(fake, "gh"), 0700); err != nil {
		t.Fatal(err)
	}
	guard := policyRun(t, policyStep(t, publish, "Publish immutable GitHub Release"))
	got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", guard}, nil, append(policyEnv(home), "PATH="+fake+":/usr/bin:/bin:/usr/sbin:/sbin", "TAG=v1.2.3", "POLICY_GH_LOG="+log), 5*time.Second)
	if err != nil || got.Status != 1 {
		t.Fatalf("existing release guard: status=%d err=%v stderr=%s", got.Status, err, got.Stderr)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "release view v1.2.3\n" {
		t.Errorf("existing release action calls: %q", calls)
	}
}
