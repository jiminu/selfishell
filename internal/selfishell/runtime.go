package selfishell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Paths struct{ Config, State, Resources, Cache, Data string }

// UserPaths only resolves names. Discovery must not create managed state.
func UserPaths() (Paths, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return Paths{}, fmt.Errorf("HOME must be set")
	}
	config := filepath.Join(envDefault("XDG_CONFIG_HOME", home+"/.config"), "selfishell")
	state := filepath.Join(envDefault("XDG_STATE_HOME", home+"/.local/state"), "selfishell")
	return Paths{Config: config, State: state, Resources: filepath.Join(state, "resources"), Cache: filepath.Join(envDefault("XDG_CACHE_HOME", home+"/.cache"), "selfishell"), Data: filepath.Join(envDefault("XDG_DATA_HOME", home+"/.local/share"), "selfishell")}, nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type Platform struct{ Name, Arch string }

func DetectPlatform() Platform {
	system := runtime.GOOS
	switch system {
	case "darwin":
		system = "Darwin"
	case "linux":
		system = "Linux"
	}
	system = envDefault("SELFISHELL_TEST_SYSTEM_NAME", system)
	arch := envDefault("SELFISHELL_TEST_MACHINE_ARCH", runtime.GOARCH)
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	name := "unsupported"
	switch system {
	case "Darwin":
		name = "macos"
	case "Linux":
		name = "unsupported-linux"
		if linuxDistribution(envDefault("SELFISHELL_TEST_OS_RELEASE_FILE", "/etc/os-release")) == "ubuntu" {
			name = "ubuntu"
		}
		proc, _ := os.ReadFile(envDefault("SELFISHELL_TEST_PROC_VERSION_FILE", "/proc/version"))
		text := strings.ToLower(string(proc))
		if strings.Contains(text, "microsoft") || strings.Contains(text, "wsl") {
			if name == "ubuntu" {
				name = "ubuntu-wsl"
			} else {
				name = "unsupported-wsl"
			}
		}
	}
	return Platform{Name: name, Arch: arch}
}

func linuxDistribution(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && key == "ID" {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "\""), "\"")
			value = strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
			if value != "" {
				return value
			}
			return "unknown"
		}
	}
	return "unknown"
}
