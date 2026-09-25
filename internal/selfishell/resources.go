package selfishell

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Resource order matches the Bash declaration table: internal defaults first,
// user-facing entrypoints last. Uninstall must retain the all-platform set.
type Resource struct{ Kind, Name, Target, Source string }

func ManagedResources(root string) ([]Resource, error) {
	if root == "" {
		return nil, fmt.Errorf("release root must be set")
	}
	paths, err := UserPaths()
	if err != nil {
		return nil, err
	}
	home := os.Getenv("HOME")
	configHome := envDefault("XDG_CONFIG_HOME", home+"/.config")
	return []Resource{
		{Kind: "file", Name: "zshrc-config", Target: paths.Config + "/zsh/zshrc", Source: root + "/config/macos/zshrc"},
		{Kind: "file", Name: "zsh-runtime", Target: paths.Config + "/zsh/runtime.zsh", Source: root + "/config/shared/zsh/runtime.zsh"},
		{Kind: "file", Name: "zsh-history", Target: paths.Config + "/zsh/history.zsh", Source: root + "/config/shared/zsh/history.zsh"},
		{Kind: "file", Name: "mise-config-file", Target: paths.Config + "/mise/selfishell.toml", Source: root + "/config/shared/mise.toml"},
		{Kind: "link", Name: "mise-config-link", Target: configHome + "/mise/conf.d/selfishell.toml", Source: paths.Config + "/mise/selfishell.toml"},
		{Kind: "file", Name: "zsh-completion", Target: paths.Config + "/zsh/completion.zsh", Source: root + "/config/shared/zsh/completion.zsh"},
		{Kind: "file", Name: "zsh-interactive", Target: paths.Config + "/zsh/interactive.zsh", Source: root + "/config/shared/zsh/interactive.zsh"},
		{Kind: "file", Name: "zsh-update-notice", Target: paths.Config + "/zsh/update-notice.zsh", Source: root + "/config/shared/zsh/update-notice.zsh"},
		{Kind: "file", Name: "zsh-common", Target: paths.Config + "/zsh/common.zsh", Source: root + "/config/shared/zsh/common.zsh"},
		{Kind: "file", Name: "aliases", Target: paths.Config + "/zsh/aliases.zsh", Source: root + "/config/shared/zsh/aliases.zsh"},
		{Kind: "file", Name: "vimrc", Target: paths.Config + "/vim/vimrc", Source: root + "/config/shared/vimrc"},
		{Kind: "file", Name: "starship-config", Target: paths.Config + "/starship.toml", Source: root + "/config/shared/starship.toml"},
		{Kind: "file", Name: "ghostty-config", Target: paths.Config + "/ghostty/config.ghostty", Source: root + "/config/macos/ghostty/config.ghostty"},
		{Kind: "file", Name: "nvim-init", Target: paths.Config + "/nvim/init.lua", Source: root + "/config/shared/nvim/init.lua"},
		{Kind: "file", Name: "nvim-lua-config-options", Target: paths.Config + "/nvim/lua/config/options.lua", Source: root + "/config/shared/nvim/lua/config/options.lua"},
		{Kind: "file", Name: "nvim-lua-config-keymaps", Target: paths.Config + "/nvim/lua/config/keymaps.lua", Source: root + "/config/shared/nvim/lua/config/keymaps.lua"},
		{Kind: "file", Name: "nvim-lua-config-autocmds", Target: paths.Config + "/nvim/lua/config/autocmds.lua", Source: root + "/config/shared/nvim/lua/config/autocmds.lua"},
		{Kind: "file", Name: "nvim-lua-config-lazy", Target: paths.Config + "/nvim/lua/config/lazy.lua", Source: root + "/config/shared/nvim/lua/config/lazy.lua"},
		{Kind: "file", Name: "nvim-lua-config-languages", Target: paths.Config + "/nvim/lua/config/languages.lua", Source: root + "/config/shared/nvim/lua/config/languages.lua"},
		{Kind: "file", Name: "nvim-lua-config-plugin-versions", Target: paths.Config + "/nvim/lua/config/plugin_versions.lua", Source: root + "/config/shared/nvim/lua/config/plugin_versions.lua"},
		{Kind: "file", Name: "nvim-plugin-versions", Target: paths.Config + "/nvim/plugin-versions.conf", Source: root + "/dependencies.conf"},
		{Kind: "file", Name: "nvim-lua-plugins-ui", Target: paths.Config + "/nvim/lua/plugins/ui.lua", Source: root + "/config/shared/nvim/lua/plugins/ui.lua"},
		{Kind: "file", Name: "nvim-lua-plugins-editor", Target: paths.Config + "/nvim/lua/plugins/editor.lua", Source: root + "/config/shared/nvim/lua/plugins/editor.lua"},
		{Kind: "file", Name: "nvim-lua-plugins-lsp", Target: paths.Config + "/nvim/lua/plugins/lsp.lua", Source: root + "/config/shared/nvim/lua/plugins/lsp.lua"},
		{Kind: "file", Name: "nvim-lua-plugins-completion", Target: paths.Config + "/nvim/lua/plugins/completion.lua", Source: root + "/config/shared/nvim/lua/plugins/completion.lua"},
		{Kind: "file", Name: "nvim-after-lsp-lua_ls", Target: paths.Config + "/nvim/after/lsp/lua_ls.lua", Source: root + "/config/shared/nvim/after/lsp/lua_ls.lua"},
		{Kind: "block", Name: "user-zshrc", Target: home + "/.zshrc", Source: "-"},
		{Kind: "block", Name: "user-zprofile", Target: home + "/.zprofile", Source: "-"},
		{Kind: "block", Name: "user-zshenv", Target: home + "/.zshenv", Source: "-"},
		{Kind: "link", Name: "user-starship", Target: configHome + "/starship.toml", Source: paths.Config + "/starship.toml"},
		{Kind: "block", Name: "user-vimrc", Target: home + "/.vimrc", Source: "-"},
		{Kind: "link", Name: "user-nvim", Target: configHome + "/nvim", Source: paths.Config + "/nvim"},
		{Kind: "block", Name: "user-ghostty", Target: configHome + "/ghostty/config.ghostty", Source: "-"},
	}, nil
}

// ResourcesForPlatform selects installation resources only. It must not be
// used to filter uninstall: another platform's recorded resources still count.
func ResourcesForPlatform(root, platform string, ghostty bool) ([]Resource, error) {
	switch platform {
	case "macos", "ubuntu", "ubuntu-wsl":
	default:
		return nil, fmt.Errorf("managed installation is unavailable on %s", platform)
	}
	all, err := ManagedResources(root)
	if err != nil {
		return nil, err
	}
	selected := make([]Resource, 0, len(all))
	for _, resource := range all {
		switch resource.Name {
		case "zshrc-config":
			if platform != "macos" {
				resource.Source = root + "/config/ubuntu/zshrc"
			}
		case "ghostty-config", "user-ghostty":
			if platform != "macos" || !ghostty {
				continue
			}
		case "user-zshenv":
			if platform == "macos" {
				continue
			}
		}
		selected = append(selected, resource)
	}
	return selected, nil
}

type ResourceState struct {
	Resource Resource
	State    State
}

// LoadResourceStates collects the complete declared set before a caller can
// begin preflight/removal. Any failure discards the entire list. This does not
// inspect managed targets or make them safe to remove; lifecycle preflight does.
func LoadResourceStates(directory string, resources []Resource) ([]ResourceState, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("managed resource declaration is empty")
	}
	seen := make(map[string]bool, len(resources))
	var records []ResourceState
	for _, resource := range resources {
		name := resource.Name
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\r\n\t\x00") || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate managed resource name: %q", name)
		}
		switch resource.Kind {
		case "file", "link", "block":
		default:
			return nil, fmt.Errorf("invalid managed resource kind: %q", resource.Kind)
		}
		if resource.Target == "" || resource.Source == "" {
			return nil, fmt.Errorf("incomplete managed resource: %s", name)
		}
		seen[name] = true
		state, err := ReadState(directory + "/" + name + ".state")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read managed resource %s: %w", name, err)
		}
		records = append(records, ResourceState{Resource: resource, State: state})
	}
	return records, nil
}
