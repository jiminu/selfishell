package terminal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jiminu/selfishell/internal/managed"
)

// Font is the Nerd Font face the terminal integrations select.
const Font = "JetBrainsMonoNL Nerd Font Mono"

// FontStatus is the Windows font probe: whether the font is installed
// and the current user's font registrations by name.
type FontStatus struct {
	FontInstalled bool              `json:"fontInstalled"`
	Registrations map[string]string `json:"registrations"`
}

// FontTarget is dep's versioned path under the Windows user's fonts.
func FontTarget(choice *WindowsChoice, dep managed.Dependency) (string, error) {
	if choice == nil || !choice.Enabled {
		return "", fmt.Errorf("Windows Terminal font setup is not enabled")
	}
	if dep.Version == "." || dep.Version == ".." || filepath.Base(dep.Version) != dep.Version || strings.ContainsAny(dep.Version, "\\\r\n\x00") {
		return "", fmt.Errorf("invalid font version: %s", dep.Version)
	}
	return choice.AppDataPath + "/Microsoft/Windows/Fonts/Selfishell/" + dep.Version + "/" + filepath.Base(dep.Target), nil
}
