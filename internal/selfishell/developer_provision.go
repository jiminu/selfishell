package selfishell

import (
	"context"
	"fmt"
)

// ProvisionDeveloper installs the fixed tool subsets needed by development
// consumers. It does not alter managed configuration or install system packages.
func ProvisionDeveloper(ctx context.Context, root, mode string, process Process) error {
	var tools []string
	direct := []string{"mise"}
	switch mode {
	case "mise-only":
	case "benchmark-shell":
		tools = []string{"starship", "fzf", "zoxide"}
		direct = append(direct, "zsh-completions", "fzf-tab", "zsh-autosuggestions", "fast-syntax-highlighting")
	case "neovim-e2e":
		tools = []string{"neovim", "tree-sitter", "node"}
	default:
		return fmt.Errorf("unknown developer provisioning mode: %s", mode)
	}
	env := process.environment()
	paths, err := userPaths(env)
	if err != nil {
		return err
	}
	platform := detectPlatform(env)
	if !platformSupported(platform.Name) {
		return fmt.Errorf("unsupported developer platform: %s", platform.Name)
	}
	o := &PackageOperation{Process: process}
	manifest := root + "/dependencies.conf"
	for _, name := range direct {
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
func DeveloperToolEnv(root string, env Environment) []string {
	home := env.Get("HOME")
	return env.With(map[string]string{
		"PATH":                      home + "/.local/bin:" + env.Get("PATH"),
		"MISE_GLOBAL_CONFIG_FILE":   root + "/config/shared/mise.toml",
		"MISE_DATA_DIR":             env.Default("MISE_DATA_DIR", home+"/.local/share/mise"),
		"MISE_CACHE_DIR":            env.Default("MISE_CACHE_DIR", home+"/.cache/mise"),
		"MISE_STATE_DIR":            env.Default("MISE_STATE_DIR", home+"/.local/state/mise"),
		"MISE_TRUSTED_CONFIG_PATHS": root + "/config/shared",
	})
}
