package selfishell

import (
	"context"
	"fmt"
	"os"
)

// ProvisionDeveloper installs the fixed tool subsets needed by development
// consumers. It does not alter managed configuration or install system packages.
func ProvisionDeveloper(ctx context.Context, root, mode string, process Process) error {
	var tools []string
	switch mode {
	case "mise-only":
	case "benchmark-shell":
		tools = []string{"starship", "fzf", "zoxide"}
	case "neovim-e2e":
		tools = []string{"neovim", "tree-sitter", "node"}
	default:
		return fmt.Errorf("unknown developer provisioning mode: %s", mode)
	}
	paths, err := UserPaths()
	if err != nil {
		return err
	}
	platform := DetectPlatform()
	if platform.Name != "macos" && platform.Name != "ubuntu" && platform.Name != "ubuntu-wsl" {
		return fmt.Errorf("unsupported developer platform: %s", platform.Name)
	}
	o := &PackageOperation{Process: process}
	manifest := root + "/dependencies.conf"
	for _, name := range []string{"mise", "zinit"} {
		if mode != "benchmark-shell" && name == "zinit" {
			continue
		}
		if err := o.InstallDirect(ctx, paths, manifest, "required", name, platform.Name, platform.Arch, false); err != nil {
			return err
		}
	}
	if len(tools) == 0 {
		return nil
	}
	pins, err := approvedMisePins(root+"/config/shared/mise.toml", tools)
	if err != nil {
		return err
	}
	return o.InstallMise(ctx, root, paths, "required", false, pins...)
}

// DeveloperToolEnv retains an isolated HOME while selecting its pinned mise bin.
func DeveloperToolEnv(root string) []string {
	home := os.Getenv("HOME")
	return withEnvironment(Process{Env: os.Environ()}, map[string]string{
		"PATH":                      home + "/.local/bin:" + os.Getenv("PATH"),
		"MISE_GLOBAL_CONFIG_FILE":   root + "/config/shared/mise.toml",
		"MISE_DATA_DIR":             envDefault("MISE_DATA_DIR", home+"/.local/share/mise"),
		"MISE_CACHE_DIR":            envDefault("MISE_CACHE_DIR", home+"/.cache/mise"),
		"MISE_STATE_DIR":            envDefault("MISE_STATE_DIR", home+"/.local/state/mise"),
		"MISE_TRUSTED_CONFIG_PATHS": root + "/config/shared",
	}).Env
}
