package selfishell

import (
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestMisePinsMatchPackageMembership(t *testing.T) {
	root := testRelease(t)
	packages, err := ReadPackages(filepath.Join(root, "packages.conf"))
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, p := range packages {
		if p.Manager == "mise" {
			declared = append(declared, p.Name)
		}
	}
	slices.Sort(declared)
	file := filepath.Join(root, "config/shared/mise.toml")
	versions, err := approvedMiseVersions(file)
	if err != nil {
		t.Fatal(err)
	}
	if pinned := slices.Sorted(maps.Keys(versions)); len(declared) == 0 || !slices.Equal(declared, pinned) {
		t.Fatalf("packages.conf mise tools %q, mise.toml pins %q", declared, pinned)
	}
	// Plans and installs pass name@version; a pin without a version fails here.
	if _, err := approvedMisePins(file, declared); err != nil {
		t.Fatal(err)
	}
}

func TestShippedDependencyRevisionsAreCommits(t *testing.T) {
	deps, err := ReadDependencies("../../dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	commit := regexp.MustCompile(`^[0-9a-f]{40}$`)
	kinds := map[string]int{}
	for _, dep := range deps {
		pin := dep.Version
		switch dep.Kind {
		case "git":
			pin = dep.Checksum
		case "zsh-plugin", "nvim-plugin":
		default:
			continue
		}
		kinds[dep.Kind]++
		if !commit.MatchString(pin) {
			t.Fatalf("%s %s is not pinned to a commit: %q", dep.Kind, dep.Name, pin)
		}
	}
	if kinds["git"] == 0 || kinds["zsh-plugin"] == 0 || kinds["nvim-plugin"] == 0 {
		t.Fatalf("pinned dependency kinds missing: %v", kinds)
	}
}
