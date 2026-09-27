package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jiminu/selfishell/internal/selfishell"
)

var numericToolVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
var pluginCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var downloadChecksum = regexp.MustCompile(`^[0-9a-f]{64}$`)

func runDependencyUpdate(ctx context.Context, root string, args []string, p selfishell.Process) int {
	flags := flag.NewFlagSet("update-dependencies", flag.ContinueOnError)
	flags.SetOutput(p.Err)
	manifest := flags.String("manifest", filepath.Join(root, "dependencies.conf"), "dependency manifest")
	metadata := flags.String("metadata", "", "apply saved metadata without network access")
	zshRoot := flags.String("zsh-root", root, "checkout containing configuration pins")
	flags.Usage = func() {
		fmt.Fprintln(p.Err, "Usage: scripts/update-dependencies.sh [--manifest FILE] [--metadata FILE] [--zsh-root DIR]")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	emptyOption := false
	flags.Visit(func(option *flag.Flag) { emptyOption = emptyOption || option.Value.String() == "" })
	if flags.NArg() != 0 || emptyOption {
		flags.Usage()
		return 2
	}
	if err := updateDependencies(ctx, p, *manifest, *metadata, *zshRoot); err != nil {
		fmt.Fprintln(p.Err, "selfishell-dev:", err)
		return 1
	}
	return 0
}

type dependencyEdit struct {
	path, before, after, staged string
	mode                        os.FileMode
}

func readDependencyEdit(path string) (*dependencyEdit, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("dependency update target must be a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &dependencyEdit{path: path, before: string(data), after: string(data), mode: info.Mode()}, nil
}

// Keep source formatting until a particular manifest record changes.
func dependencyKey(fields []string) string {
	key := strings.Join(fields[:2], " ")
	if fields[0] == "download" {
		key += " " + strings.Join(fields[3:5], " ")
	}
	return key
}

func parseDependencyMetadata(data string) ([][]string, error) {
	var updates [][]string
	seen := map[string]bool{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		valid := false
		switch fields[0] {
		case "git":
			valid = len(fields) == 3 || len(fields) == 4 && pluginCommit.MatchString(fields[3])
		case "nvim-plugin", "zsh-plugin":
			valid = len(fields) == 3 && pluginCommit.MatchString(fields[2])
		case "download":
			valid = len(fields) == 7 && downloadChecksum.MatchString(fields[6])
		case "mise-tool":
			valid = len(fields) == 3 && numericToolVersion.MatchString(fields[2])
		}
		if !valid || strings.ContainsRune(line, '\x00') {
			return nil, fmt.Errorf("invalid dependency metadata record: %s", fields[0])
		}
		key := dependencyKey(fields)
		if seen[key] {
			return nil, fmt.Errorf("duplicate dependency metadata: %s", key)
		}
		seen[key] = true
		updates = append(updates, fields)
	}
	return updates, nil
}

func updateDependencies(ctx context.Context, p selfishell.Process, manifest, metadata, root string) error {
	manifestEdit, err := readDependencyEdit(manifest)
	if err != nil {
		return err
	}
	dependencies, err := selfishell.ReadDependencies(manifest)
	if err != nil {
		return err
	}
	var updates [][]string
	if metadata == "" {
		updates, err = discoverDependencyUpdates(ctx, p, dependencies)
	} else {
		var data []byte
		data, err = os.ReadFile(metadata)
		if err == nil {
			updates, err = parseDependencyMetadata(string(data))
		}
	}
	if err != nil {
		return err
	}
	edits := map[string]*dependencyEdit{manifestEdit.path: manifestEdit}
	configuration := func(relative string) (*dependencyEdit, error) {
		path, err := filepath.Abs(filepath.Join(root, relative))
		if err != nil {
			return nil, err
		}
		if edit := edits[path]; edit != nil {
			return edit, nil
		}
		edit, err := readDependencyEdit(path)
		if err == nil {
			edits[path] = edit
		}
		return edit, err
	}
	lines := strings.SplitAfter(manifestEdit.before, "\n")
	for _, update := range updates {
		if update[0] == "mise-tool" {
			edit, err := configuration("config/shared/mise.toml")
			if err != nil {
				return err
			}
			if edit.after, err = updateMisePin(edit.after, update[1], update[2]); err != nil {
				return err
			}
			continue
		}
		matched := -1
		for index, line := range lines {
			fields := strings.Fields(line)
			if len(fields) != 9 || strings.HasPrefix(fields[0], "#") || dependencyKey(fields) != dependencyKey(update) {
				continue
			}
			if matched != -1 {
				return fmt.Errorf("ambiguous manifest entry: %s", dependencyKey(update))
			}
			matched = index
		}
		if matched == -1 {
			return fmt.Errorf("dependency metadata did not match manifest entry: %s", dependencyKey(update))
		}
		fields := strings.Fields(lines[matched])
		if update[0] == "zsh-plugin" {
			file := map[string]string{
				"zsh-users/zsh-completions":                  "config/shared/zsh/completion.zsh",
				"Aloxaf/fzf-tab":                             "config/shared/zsh/interactive.zsh",
				"zsh-users/zsh-autosuggestions":              "config/shared/zsh/interactive.zsh",
				"zdharma-continuum/fast-syntax-highlighting": "config/shared/zsh/interactive.zsh",
			}[update[1]]
			if file == "" || !pluginCommit.MatchString(fields[2]) {
				return fmt.Errorf("invalid recorded Zsh plugin pin or mapping: %s", update[1])
			}
			edit, err := configuration(file)
			if err != nil {
				return err
			}
			old := "ver'" + fields[2] + "'"
			if strings.Count(edit.after, old) != 1 {
				return fmt.Errorf("expected exactly one %s pin in %s", update[1], file)
			}
			edit.after = strings.Replace(edit.after, old, "ver'"+update[2]+"'", 1)
		}
		previous := strings.Join(fields, " ")
		fields[2] = update[2]
		switch update[0] {
		case "git":
			if len(update) == 4 {
				fields[6] = update[3]
			}
		case "download":
			fields[5], fields[6] = update[5], update[6]
		}
		if next := strings.Join(fields, " "); next != previous {
			ending := ""
			if strings.HasSuffix(lines[matched], "\n") {
				ending = "\n"
			}
			lines[matched] = next + ending
		}
	}
	manifestEdit.after = strings.Join(lines, "")
	return commitDependencyEdits(ctx, edits)
}

func updateMisePin(data, tool, candidate string) (string, error) {
	pattern := regexp.MustCompile(`^([ \t]*` + regexp.QuoteMeta(tool) + `[ \t]*=[ \t]*")([^"]*)("[^\n]*)$`)
	lines := strings.SplitAfter(data, "\n")
	inTools, matched := false, -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inTools = trimmed == "[tools]"
			continue
		}
		if !inTools {
			continue
		}
		parts := pattern.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
		if parts == nil {
			continue
		}
		if matched != -1 || !numericToolVersion.MatchString(parts[2]) {
			return "", fmt.Errorf("invalid or duplicate current mise pin: %s", tool)
		}
		matched = index
		if newerToolVersion(candidate, parts[2]) {
			lines[index] = strings.Replace(line, parts[1]+parts[2]+parts[3], parts[1]+candidate+parts[3], 1)
		}
	}
	if matched == -1 {
		return "", fmt.Errorf("no current mise pin: %s", tool)
	}
	return strings.Join(lines, ""), nil
}

func newerToolVersion(candidate, current string) bool {
	a, b := strings.Split(candidate, "."), strings.Split(current, ".")
	for index := 0; index < len(a) || index < len(b); index++ {
		left, right := "", ""
		if index < len(a) {
			left = strings.TrimLeft(a[index], "0")
		}
		if index < len(b) {
			right = strings.TrimLeft(b[index], "0")
		}
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		if left != right {
			return left > right
		}
	}
	return false
}

// Validate and stage all changes before publishing any. Renames are atomic per
// file; this is not a filesystem transaction across multiple configuration files.
func commitDependencyEdits(ctx context.Context, edits map[string]*dependencyEdit) error {
	var paths []string
	for path, edit := range edits {
		if edit.before != edit.after {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	defer func() {
		for _, path := range paths {
			if staged := edits[path].staged; staged != "" {
				os.Remove(staged)
			}
		}
	}()
	for _, path := range paths {
		edit := edits[path]
		file, err := os.CreateTemp(filepath.Dir(path), ".selfishell-dependency-update-*")
		if err != nil {
			return err
		}
		edit.staged = file.Name()
		_, writeErr := file.WriteString(edit.after)
		modeErr := file.Chmod(edit.mode)
		if err := errors.Join(writeErr, modeErr, file.Close()); err != nil {
			return err
		}
	}
	// Do not replace a target edited or relinked while metadata was being fetched.
	for _, edit := range edits {
		current, err := readDependencyEdit(edit.path)
		if err != nil {
			return err
		}
		if current.before != edit.before || current.mode != edit.mode {
			return fmt.Errorf("dependency update target changed: %s", edit.path)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Rename(edits[path].staged, path); err != nil {
			return err
		}
	}
	return nil
}
