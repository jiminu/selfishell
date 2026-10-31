package selfishell

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/jiminu/selfishell/internal/process"
)

type Platform struct{ Name, Arch string }

func detectPlatform(env process.Environment) Platform {
	system := runtime.GOOS
	switch system {
	case "darwin":
		system = "Darwin"
	case "linux":
		system = "Linux"
	}
	system = env.Default("SELFISHELL_TEST_SYSTEM_NAME", system)
	arch := env.Default("SELFISHELL_TEST_MACHINE_ARCH", runtime.GOARCH)
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
		if linuxDistribution(env.Default("SELFISHELL_TEST_OS_RELEASE_FILE", "/etc/os-release")) == "ubuntu" {
			name = "ubuntu"
		}
		proc, _ := os.ReadFile(env.Default("SELFISHELL_TEST_PROC_VERSION_FILE", "/proc/version"))
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
