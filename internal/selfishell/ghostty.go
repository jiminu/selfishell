package selfishell

import (
	"os"
	"strings"
)

// Ghostty applies config-file includes after the including file, so a key or
// same-trigger keybind outside the block loses to the managed defaults.
// font-family values append fallbacks instead and are never overridden.
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
		} else if !keys[key] || strings.HasPrefix(key, "font-family") {
			return
		}
		if !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	})
	return found
}

func ghosttySettings(data []byte, visit func(key, value string)) {
	block, _ := blockContent("user-ghostty", "")
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

func (c CLI) noteGhosttyOverrides(resources []Resource) {
	var defaults, entrypoint string
	for _, r := range resources {
		switch r.Name {
		case "ghostty-config":
			defaults = r.Source
		case "user-ghostty":
			entrypoint = r.Target
		}
	}
	managed, err := os.ReadFile(defaults)
	if err != nil {
		return
	}
	user, err := os.ReadFile(entrypoint)
	if err != nil {
		return
	}
	if found := ghosttyOverridden(managed, user); len(found) > 0 {
		c.report("Notes", reportWarning, "Selfishell's Ghostty defaults override these settings in %s: %s. Move them to user.ghostty to keep them.", entrypoint, strings.Join(found, ", "))
	}
}
