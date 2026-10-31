package selfishell

import (
	"os"
	"strings"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
)

// The app can be installed directly, outside Homebrew's cask inventory.
func ghosttyInstalled(p process.Process) bool {
	if _, err := p.LookPath("ghostty"); err == nil {
		return true
	}
	env := p.Environment()
	for _, base := range []string{env.Default("SELFISHELL_TEST_APPLICATIONS_DIR", "/Applications"), env.Get("HOME") + "/Applications"} {
		info, err := os.Stat(base + "/Ghostty.app/Contents/MacOS/ghostty")
		if err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return true
		}
	}
	return false
}

// Ghostty applies config-file includes after the including file, so a key or
// same-trigger keybind outside the block loses to the managed defaults.
// Repeatable font keys combine with the defaults instead and are not reported.
func ghosttyOverridden(defaults, entrypoint []byte) []string {
	keys, triggers := map[string]bool{}, map[string]bool{}
	ghosttySettings(defaults, func(key, value string) {
		if key == "keybind" {
			triggers[ghosttyTrigger(value)] = true
		} else {
			keys[key] = true
		}
	})
	var found []string
	seen := map[string]bool{}
	ghosttySettings(entrypoint, func(key, value string) {
		name := key
		if key == "keybind" {
			if !triggers[ghosttyTrigger(value)] {
				return
			}
			name = "keybind " + ghosttyTrigger(value)
		} else if !keys[key] || ghosttyRepeatableFont(key) {
			return
		}
		if !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	})
	return found
}

func ghosttyRepeatableFont(key string) bool {
	return strings.HasPrefix(key, "font-family") || key == "font-feature" || key == "font-codepoint-map"
}

func ghosttySettings(data []byte, visit func(key, value string)) {
	block, _ := managed.BlockContent("user-ghostty", "")
	markers := strings.Split(strings.TrimSuffix(string(block), "\n"), "\n")
	begin, end := markers[0], markers[len(markers)-1]
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		switch {
		case line == begin:
			inBlock = true
		case line == end:
			inBlock = false
		case inBlock, line == "", strings.HasPrefix(line, "#"):
		default:
			if key, value, ok := strings.Cut(line, "="); ok {
				visit(strings.TrimSpace(key), strings.TrimSpace(value))
			}
		}
	}
}

func ghosttyTrigger(binding string) string {
	trigger, _, _ := strings.Cut(binding, "=")
	trigger = strings.TrimSpace(trigger)
	for stripped := true; stripped; {
		stripped = false
		for _, prefix := range []string{"global:", "all:", "unconsumed:", "performable:"} {
			if strings.HasPrefix(trigger, prefix) {
				trigger, stripped = trigger[len(prefix):], true
			}
		}
	}
	return trigger
}

// ghosttyOverrides lists entrypoint settings that the managed defaults override.
func ghosttyOverrides(resources []managed.Resource) (entrypoint string, found []string) {
	var defaults string
	for _, r := range resources {
		switch r.Name {
		case "ghostty-config":
			defaults = r.Source
		case "user-ghostty":
			entrypoint = r.Target
		}
	}
	defaultsData, err := os.ReadFile(defaults)
	if err != nil {
		return entrypoint, nil
	}
	user, err := os.ReadFile(entrypoint)
	if err != nil {
		return entrypoint, nil
	}
	return entrypoint, ghosttyOverridden(defaultsData, user)
}
