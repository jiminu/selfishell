package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestCIChangeClassification(t *testing.T) {
	t.Parallel()
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
	if err := testutil.WriteFile(path, []byte(content), 0600); err != nil {
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

// A local reusable workflow is read from the same commit as its caller.
var localWorkflowRE = regexp.MustCompile(`^    uses: \./\.github/workflows/[a-z0-9-]+\.yml$`)

func TestWorkflowActionPins(t *testing.T) {
	t.Parallel()
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
				if !pinRE.MatchString(line) && !localWorkflowRE.MatchString(line) {
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
	t.Parallel()
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
	t.Parallel()
	raw, jobs := workflowSections(t, "release.yml")
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "permissions:") {
			t.Fatal("release workflow has top-level permissions")
		}
	}
	ci := policyJob(t, jobs, "ci")
	build := policyJob(t, jobs, "build")
	smoke := policyJob(t, jobs, "smoke")
	publish := policyJob(t, jobs, "publish")
	if policyField(t, ci.lines, "    ", "permissions") != "" || policyField(t, publish.lines, "    ", "permissions") != "" {
		t.Fatal("job permissions must be maps")
	}
	for _, tc := range []struct {
		job        workflowJob
		name, perm string
	}{{build, "build", "contents: read"}, {smoke, "smoke", "contents: read"}, {publish, "publish", "contents: write"}} {
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
	for _, j := range []workflowJob{ci, build, smoke} {
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
	if policyField(t, ci.lines, "    ", "uses") != "./.github/workflows/ci.yml" || !strings.Contains(strings.Join(ci.lines, "\n"), "      contents: read") {
		t.Error("release must call the full CI workflow with read-only contents")
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
	// depend transitively on full CI even if intermediate jobs are added.
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
	for _, pair := range [][2]string{{"build", "ci"}, {"smoke", "build"}, {"publish", "ci"}, {"publish", "build"}, {"publish", "smoke"}} {
		if !visit(pair[0])[pair[1]] {
			t.Errorf("%s has no transitive dependency on %s", pair[0], pair[1])
		}
	}
}

func TestReleaseWorkflowArtifactHandoff(t *testing.T) {
	t.Parallel()
	_, jobs := workflowSections(t, "release.yml")
	build, smoke, publish := policyJob(t, jobs, "build"), policyJob(t, jobs, "smoke"), policyJob(t, jobs, "publish")
	for _, stage := range []struct{ job, command string }{
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
	for _, needle := range []string{"actions/upload-artifact@", "name: release-assets-${{ github.run_id }}-${{ github.run_attempt }}", "path: dist/", "if-no-files-found: error"} {
		if !strings.Contains(upload, needle) {
			t.Errorf("upload missing %s", needle)
		}
	}
	if !strings.Contains(strings.Join(build.lines, "\n"), "steps.upload.outputs.artifact-id") {
		t.Error("build does not expose upload ID")
	}
	for _, j := range []workflowJob{smoke, publish} {
		download := strings.Join(policyStep(t, j, "Download release artifacts"), "\n")
		for _, needle := range []string{"actions/download-artifact@", "artifact-ids: ${{ needs.build.outputs.artifact_id }}", "path: dist", "digest-mismatch: error"} {
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
	t.Parallel()
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

// dependencyRepo commits a fixture for every file the dependency workflow may change.
func dependencyRepo(t *testing.T) (home, repo string) {
	t.Helper()
	home, repo, _ = policyRepo(t)
	for _, path := range []string{"config/shared/zsh/completion.zsh", "config/shared/zsh/interactive.zsh", "config/shared/mise.toml", "config/shared/nvim/lua/config/languages.lua", "go.mod", "mise.toml"} {
		policyWrite(t, filepath.Join(repo, path), "# fixture\n")
	}
	policyGit(t, home, repo, "add", ".")
	policyGit(t, home, repo, "commit", "-qm", "tracked dependency files")
	return home, repo
}

func TestDependencyWorkflowPermissions(t *testing.T) {
	t.Parallel()
	raw, jobs := workflowSections(t, "dependency-updates.yml")
	if !strings.Contains(raw, "\npermissions:\n  contents: read\n\n") {
		t.Error("dependency workflow must default to a read-only token")
	}
	for _, needle := range []string{"pr merge", "--auto", "release create", "git tag"} {
		if strings.Contains(raw, needle) {
			t.Errorf("dependency workflow must never merge or release: %s", needle)
		}
	}
	update, pr := policyJob(t, jobs, "update"), policyJob(t, jobs, "pull-request")
	updateText := strings.Join(update.lines, "\n")
	if strings.Contains(updateText, ": write") || !strings.Contains(updateText, "\n      contents: read\n") {
		t.Error("discovery and verification must run with read-only contents")
	}
	if !strings.Contains(strings.Join(policyStep(t, update, "Check out repository"), "\n"), "persist-credentials: false") {
		t.Error("discovery checkout must not persist the token")
	}
	for _, needle := range []string{"changed: ${{ steps.export.outputs.changed }}", "artifact_id: ${{ steps.upload.outputs.artifact-id }}"} {
		if !strings.Contains(updateText, needle) {
			t.Errorf("discovery job output missing %s", needle)
		}
	}
	upload := strings.Join(policyStep(t, update, "Upload verified dependency changes"), "\n")
	for _, needle := range []string{"actions/upload-artifact@", "path: ${{ runner.temp }}/dependency-updates/", "if-no-files-found: error"} {
		if !strings.Contains(upload, needle) {
			t.Errorf("upload missing %s", needle)
		}
	}
	prText := strings.Join(pr.lines, "\n")
	for _, perm := range []string{"contents: write", "pull-requests: write"} {
		if !strings.Contains(prText, "\n      "+perm+"\n") {
			t.Errorf("PR job missing %s", perm)
		}
	}
	if needs := policyNeeds(t, pr.lines); len(needs) != 1 || needs[0] != "update" {
		t.Errorf("PR job must wait for verification: %v", needs)
	}
	if condition := policyField(t, pr.lines, "    ", "if"); condition != "needs.update.outputs.changed == 'true'" {
		t.Errorf("PR job does not require verified changes: %q", condition)
	}
	// The write job only applies the handed-off patch; it must not run repository code.
	if len(pr.steps) != 3 || strings.Contains(prText, "scripts/") || strings.Contains(prText, "setup-go") {
		t.Errorf("PR job runs more than checkout, download, and publish: %v", pr.steps)
	}
	download := strings.Join(policyStep(t, pr, "Download verified dependency changes"), "\n")
	for _, needle := range []string{"actions/download-artifact@", "artifact-ids: ${{ needs.update.outputs.artifact_id }}", "path: ${{ runner.temp }}/dependency-updates", "digest-mismatch: error"} {
		if !strings.Contains(download, needle) {
			t.Errorf("download missing %s", needle)
		}
	}
	if !strings.Contains(strings.Join(policyStep(t, pr, "Create or refresh dependency update PR"), "\n"), "PATCH: ${{ runner.temp }}/dependency-updates/dependency-updates.patch") {
		t.Error("PR step does not read the downloaded patch")
	}
}

func TestDependencyWorkflowChanges(t *testing.T) {
	t.Parallel()
	_, jobs := workflowSections(t, "dependency-updates.yml")
	job := policyJob(t, jobs, "update")
	detect := policyStep(t, job, "Check dependency changes")
	export := policyStep(t, job, "Export verified dependency changes")
	for _, tc := range []struct{ step, id string }{{"Check dependency changes", "changes"}, {"Export verified dependency changes", "export"}, {"Upload verified dependency changes", "upload"}} {
		if id := policyField(t, policyStep(t, job, tc.step), "        ", "id"); id != tc.id {
			t.Fatalf("%s output is not connected to downstream steps: %q", tc.step, id)
		}
	}
	order := strings.Join(job.lines, "\n") + "\n"
	previous := -1
	for _, name := range []string{"Update dependency manifest", "Check dependency changes", "Set up updated Go toolchain", "Install shell tooling", "Validate updated manifest", "Export verified dependency changes", "Upload verified dependency changes"} {
		index := strings.Index(order, "      - name: "+name+"\n")
		if index <= previous {
			t.Fatalf("dependency step %q missing or out of order", name)
		}
		previous = index
	}
	for _, name := range []string{"Set up updated Go toolchain", "Install shell tooling", "Validate updated manifest", "Export verified dependency changes"} {
		if condition := policyField(t, policyStep(t, job, name), "        ", "if"); condition != "steps.changes.outputs.changed == 'true'" {
			t.Errorf("%s does not require dependency changes: %q", name, condition)
		}
	}
	if condition := policyField(t, policyStep(t, job, "Upload verified dependency changes"), "        ", "if"); condition != "steps.export.outputs.changed == 'true'" {
		t.Errorf("upload does not require an exported patch: %q", condition)
	}
	// Detection and the post-verification export share the same outcomes; the
	// export also writes a patch holding only the approved files.
	for _, block := range []struct {
		name  string
		lines []string
	}{{"detect", detect}, {"export", export}} {
		script := policyRun(t, block.lines)
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
			{"lsp", "config/shared/nvim/lua/config/languages.lua", 0, "changed=true\n"},
			{"go_toolchain", "go.mod", 0, "changed=true\n"},
			{"development_mise", "mise.toml", 0, "changed=true\n"},
			{"unexpected_tracked_only", "README.md", 1, ""},
			{"unexpected_untracked_only", "unexpected.txt", 1, ""},
		} {
			t.Run(block.name+"/"+tc.name, func(t *testing.T) {
				home, repo := dependencyRepo(t)
				if tc.path != "" {
					policyWrite(t, filepath.Join(repo, tc.path), "changed\n")
				}
				output := filepath.Join(home, "output")
				policyWrite(t, output, "")
				runner := filepath.Join(home, "runner")
				got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", script}, nil, append(policyEnv(home), "GITHUB_OUTPUT="+output, "RUNNER_TEMP="+runner), 5*time.Second)
				if err != nil || got.Status != tc.status {
					t.Fatalf("change detection: status=%d want=%d err=%v stderr=%s", got.Status, tc.status, err, got.Stderr)
				}
				data, err := os.ReadFile(output)
				if err != nil || string(data) != tc.output {
					t.Fatalf("change output=%q want=%q err=%v", data, tc.output, err)
				}
				patch := filepath.Join(runner, "dependency-updates/dependency-updates.patch")
				_, statErr := os.Stat(patch)
				if block.name == "detect" || tc.output != "changed=true\n" {
					if !os.IsNotExist(statErr) {
						t.Fatalf("unexpected patch: %v", statErr)
					}
					return
				}
				policyGit(t, home, repo, "checkout", "--", ".")
				policyGit(t, home, repo, "apply", patch)
				if changed := strings.TrimSpace(policyGit(t, home, repo, "diff", "--name-only")); changed != tc.path {
					t.Errorf("patch changed %q want %q", changed, tc.path)
				}
			})
		}
	}
}

func TestGoSecurityTargetsAndFailure(t *testing.T) {
	t.Parallel()
	raw, jobs := workflowSections(t, "go-security.yml")
	if strings.Contains(raw, "pull_request:") || strings.Contains(raw, "  push:") {
		t.Fatal("scheduled security scanning must not add work to ordinary PR/push CI")
	}
	block := policyRun(t, policyStep(t, policyJob(t, jobs, "vulnerabilities"), "Check supported targets"))
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%t", fail), func(t *testing.T) {
			home := t.TempDir()
			scanner := filepath.Join(home, "go-security-bin/govulncheck")
			policyWrite(t, scanner, "#!/bin/sh\nprintf '%s/%s %s %s\\n' \"$GOOS\" \"$GOARCH\" \"$CGO_ENABLED\" \"$*\" >>\"$SCAN_LOG\"\n[ \"$FAIL_SCAN\" != true ] || exit 3\n")
			if err := os.Chmod(scanner, 0700); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(home, "scans")
			env := append(policyEnv(home), "RUNNER_TEMP="+home, "SCAN_LOG="+log, fmt.Sprintf("FAIL_SCAN=%t", fail))
			got, err := runCommandIn(home, home, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, env, 5*time.Second)
			wantStatus := 0
			want := "linux/amd64 0 ./...\nlinux/arm64 0 ./...\ndarwin/amd64 0 ./...\ndarwin/arm64 0 ./...\n"
			if fail {
				wantStatus, want = 3, "linux/amd64 0 ./...\n"
			}
			data, readErr := os.ReadFile(log)
			if err != nil || got.Status != wantStatus || readErr != nil || string(data) != want {
				t.Fatalf("scan status=%d want=%d err=%v log=%q read=%v", got.Status, wantStatus, err, data, readErr)
			}
		})
	}
}

func TestDependencyWorkflowPRBlock(t *testing.T) {
	t.Parallel()
	_, jobs := workflowSections(t, "dependency-updates.yml")
	block := policyRun(t, policyStep(t, policyJob(t, jobs, "pull-request"), "Create or refresh dependency update PR"))
	const branch = "automation/dependency-updates"
	const bot = "github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>"
	for _, tc := range []struct {
		name             string
		remote           []string // authors of existing branch commits, oldest first
		open, unexpected bool
		status           int
	}{
		{"create_when_absent", nil, false, false, 0},
		{"refresh_bot_branch_when_open", []string{bot, bot}, true, false, 0},
		{"keep_maintainer_commit", []string{bot, "Maintainer <maintainer@selfishell.invalid>"}, true, false, 1},
		{"reject_unexpected_path", nil, false, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, repo := dependencyRepo(t)
			base := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
			remote := filepath.Join(home, "remote.git")
			policyGit(t, home, home, "init", "-q", "--bare", "-b", "main", remote)
			policyGit(t, home, repo, "remote", "add", "origin", remote)
			policyGit(t, home, repo, "push", "-q", "origin", "main")
			policyGit(t, home, repo, "fetch", "-q", "origin")
			if len(tc.remote) > 0 {
				policyGit(t, home, repo, "switch", "-q", "-c", "previous")
				for i, author := range tc.remote {
					name, email, _ := strings.Cut(strings.TrimSuffix(author, ">"), " <")
					policyWrite(t, filepath.Join(repo, "go.mod"), fmt.Sprintf("# previous %d\n", i))
					policyGit(t, home, repo, "add", "go.mod")
					policyGit(t, home, repo, "-c", "user.name="+name, "-c", "user.email="+email, "commit", "-qm", "previous update")
				}
				policyGit(t, home, repo, "push", "-q", "origin", "previous:refs/heads/"+branch)
				policyGit(t, home, repo, "switch", "-q", "main")
				policyGit(t, home, repo, "branch", "-q", "-D", "previous")
			}
			remoteBefore := policyGit(t, home, repo, "ls-remote", "origin", "refs/heads/"+branch)
			// The patch stands in for the artifact from the read-only job.
			policyWrite(t, filepath.Join(repo, "dependencies.conf"), classificationDeps("2222222222222222222222222222222222222222", "1.0"))
			policyWrite(t, filepath.Join(repo, "config/shared/nvim/lua/config/languages.lua"), "return { lsp = { \"lua_ls@3.19.1\" } }\n")
			if tc.unexpected {
				policyWrite(t, filepath.Join(repo, "README.md"), "unrelated change\n")
			}
			patch := filepath.Join(home, "dependency-updates.patch")
			policyWrite(t, patch, policyGit(t, home, repo, "diff", "--binary"))
			policyGit(t, home, repo, "checkout", "--", ".")
			fake := filepath.Join(home, "fakebin")
			log := filepath.Join(home, "gh.log")
			policyWrite(t, filepath.Join(fake, "gh"), `#!/bin/bash
printf '%s\n' "$*" >>"$POLICY_GH_LOG"
if [[ "$1 $2" == 'pr list' ]]; then printf '%s\n' "$POLICY_OPEN_COUNT"; exit 0; fi
if [[ "$1 $2" == 'pr create' ]]; then exit 0; fi
exit 90
`)
			if err := os.Chmod(filepath.Join(fake, "gh"), 0700); err != nil {
				t.Fatal(err)
			}
			mode := "0"
			if tc.open {
				mode = "1"
			}
			env := append(policyEnv(home), "PATH="+fake+":/usr/bin:/bin:/usr/sbin:/sbin", "BRANCH="+branch, "PATCH="+patch, "POLICY_GH_LOG="+log, "POLICY_OPEN_COUNT="+mode)
			got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, env, 10*time.Second)
			if err != nil || got.Status != tc.status {
				t.Fatalf("PR block: status=%d want=%d err=%v stdout=%s stderr=%s", got.Status, tc.status, err, got.Stdout, got.Stderr)
			}
			remoteAfter := policyGit(t, home, repo, "ls-remote", "origin", "refs/heads/"+branch)
			calls, readErr := os.ReadFile(log)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if strings.Contains(string(calls), "pr merge") || strings.Contains(string(calls), "release create") {
				t.Errorf("unexpected publish operation: %s", calls)
			}
			if tc.status != 0 {
				want := "Dependency update touched unexpected files:"
				if !tc.unexpected {
					want = branch + " has commits not authored by github-actions[bot]:"
					if !strings.Contains(string(got.Stderr), "Maintainer <maintainer@selfishell.invalid>") || !strings.Contains(string(got.Stderr), "Merge the dependency update PR, or close it and delete the branch") {
						t.Errorf("maintainer guidance missing: %s", got.Stderr)
					}
				}
				if !strings.Contains(string(got.Stderr), want) {
					t.Errorf("stderr missing %q: %s", want, got.Stderr)
				}
				if remoteAfter != remoteBefore {
					t.Errorf("remote branch overwritten: before=%q after=%q", remoteBefore, remoteAfter)
				}
				if head := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD")); head != base {
					t.Errorf("workflow committed before rejecting: %s", head)
				}
				if current := strings.TrimSpace(policyGit(t, home, repo, "branch", "--show-current")); current != "main" {
					t.Errorf("workflow switched branch before rejecting: %s", current)
				}
				if len(calls) != 0 {
					t.Errorf("workflow called gh before rejecting: %s", calls)
				}
				return
			}
			head := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD"))
			if remoteAfter != head+"\trefs/heads/"+branch+"\n" {
				t.Errorf("remote branch=%q want %s", remoteAfter, head)
			}
			if parent := strings.TrimSpace(policyGit(t, home, repo, "rev-parse", "HEAD^")); parent != base {
				t.Errorf("refresh is not based on main: parent=%s base=%s", parent, base)
			}
			if author := strings.TrimSpace(policyGit(t, home, repo, "log", "-1", "--format=%an <%ae>")); author != bot {
				t.Errorf("commit author=%q", author)
			}
			if staged := strings.TrimSpace(policyGit(t, home, repo, "show", "--format=", "--name-only", "HEAD")); staged != "config/shared/nvim/lua/config/languages.lua\ndependencies.conf" {
				t.Errorf("unexpected committed files: %q", staged)
			}
			if !strings.Contains(string(calls), "pr list --head "+branch+" --state open --json number --jq length") {
				t.Errorf("wrong PR lookup: %s", calls)
			}
			created := strings.Contains(string(calls), "pr create ")
			if created == tc.open {
				t.Errorf("create=%t for open=%t: %s", created, tc.open, calls)
			}
			if created {
				for _, s := range []string{"--base main", "--head " + branch, "Review release notes and CI results before merging", "never merges or releases automatically"} {
					if !strings.Contains(string(calls), s) {
						t.Errorf("PR call missing %q: %s", s, calls)
					}
				}
			}
		})
	}
}

func TestCIWorkflowUsesTrustedBaseClassifier(t *testing.T) {
	t.Parallel()
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
	// Base is the trusted pull_request case; head, whose classifier reports no
	// runtime changes, stands in for a documentation-only commit that release calls.
	for _, tc := range []struct{ name, full, base string }{{"trusted_base", "false", base}, {"release_full", "true", head}} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(home, tc.name+"-output")
			summary := filepath.Join(home, tc.name+"-summary")
			env := append(policyEnv(home), "FULL_CHECKS="+tc.full, "BASE_SHA="+tc.base, "GITHUB_SHA="+head, "GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+summary)
			got, err := runCommandIn(home, repo, []string{"/bin/bash", "-e", "-o", "pipefail", "-c", block}, nil, env, 5*time.Second)
			if err != nil || got.Status != 0 {
				t.Fatalf("CI block: status=%d err=%v stderr=%s", got.Status, err, got.Stderr)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "runtime=true\nubuntu_container_e2e=true\n" {
				t.Errorf("classification: %q", data)
			}
			sum, err := os.ReadFile(summary)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(sum), string(data)) {
				t.Errorf("summary lacks classification: %q", sum)
			}
		})
	}
	filter := strings.Join(policyStep(t, policyJob(t, jobs, "changes"), "Detect runtime changes"), "\n")
	if !strings.Contains(filter, "FULL_CHECKS: ${{ github.event_name == 'workflow_dispatch' || inputs.full == true }}") {
		t.Error("manual and called CI must run every check")
	}
}

func TestReleaseWorkflowMainAncestryAndImmutableGuard(t *testing.T) {
	t.Parallel()
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

func TestReleaseBuildDisablesGoCache(t *testing.T) {
	t.Parallel()
	_, jobs := workflowSections(t, "release.yml")
	setup := strings.Join(policyStep(t, policyJob(t, jobs, "build"), "Set up pinned Go toolchain"), "\n")
	if !strings.Contains(setup, "cache: false") {
		t.Error("release artifact build must disable Go cache restore/save")
	}
}
