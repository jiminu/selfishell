#!/usr/bin/env bash

selfishell_managed_resources() {
  cat <<EOF
file	zshrc-config	$SELFISHELL_CONFIG_DIR/zsh/zshrc	$SELFISHELL_ROOT/config/macos/zshrc
file	zsh-runtime	$SELFISHELL_CONFIG_DIR/zsh/runtime.zsh	$SELFISHELL_ROOT/config/shared/zsh/runtime.zsh
file	zsh-history	$SELFISHELL_CONFIG_DIR/zsh/history.zsh	$SELFISHELL_ROOT/config/shared/zsh/history.zsh
file	mise-config-file	$SELFISHELL_CONFIG_DIR/mise/selfishell.toml	$SELFISHELL_ROOT/config/shared/mise.toml
link	mise-config-link	${XDG_CONFIG_HOME:-$HOME/.config}/mise/conf.d/selfishell.toml	$SELFISHELL_CONFIG_DIR/mise/selfishell.toml
file	zsh-completion	$SELFISHELL_CONFIG_DIR/zsh/completion.zsh	$SELFISHELL_ROOT/config/shared/zsh/completion.zsh
file	zsh-interactive	$SELFISHELL_CONFIG_DIR/zsh/interactive.zsh	$SELFISHELL_ROOT/config/shared/zsh/interactive.zsh
file	zsh-update-notice	$SELFISHELL_CONFIG_DIR/zsh/update-notice.zsh	$SELFISHELL_ROOT/config/shared/zsh/update-notice.zsh
file	zsh-common	$SELFISHELL_CONFIG_DIR/zsh/common.zsh	$SELFISHELL_ROOT/config/shared/zsh/common.zsh
file	aliases	$SELFISHELL_CONFIG_DIR/zsh/aliases.zsh	$SELFISHELL_ROOT/config/shared/zsh/aliases.zsh
file	vimrc	$SELFISHELL_CONFIG_DIR/vim/vimrc	$SELFISHELL_ROOT/config/shared/vimrc
file	starship-config	$SELFISHELL_CONFIG_DIR/starship.toml	$SELFISHELL_ROOT/config/shared/starship.toml
file	ghostty-config	$SELFISHELL_CONFIG_DIR/ghostty/config.ghostty	$SELFISHELL_ROOT/config/macos/ghostty/config.ghostty
file	nvim-init	$SELFISHELL_CONFIG_DIR/nvim/init.lua	$SELFISHELL_ROOT/config/shared/nvim/init.lua
file	nvim-lua-config-options	$SELFISHELL_CONFIG_DIR/nvim/lua/config/options.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/options.lua
file	nvim-lua-config-keymaps	$SELFISHELL_CONFIG_DIR/nvim/lua/config/keymaps.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/keymaps.lua
file	nvim-lua-config-autocmds	$SELFISHELL_CONFIG_DIR/nvim/lua/config/autocmds.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/autocmds.lua
file	nvim-lua-config-lazy	$SELFISHELL_CONFIG_DIR/nvim/lua/config/lazy.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/lazy.lua
file	nvim-lua-config-languages	$SELFISHELL_CONFIG_DIR/nvim/lua/config/languages.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/languages.lua
file	nvim-lua-config-plugin-versions	$SELFISHELL_CONFIG_DIR/nvim/lua/config/plugin_versions.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/config/plugin_versions.lua
file	nvim-plugin-versions	$SELFISHELL_CONFIG_DIR/nvim/plugin-versions.conf	$SELFISHELL_ROOT/dependencies.conf
file	nvim-lua-plugins-ui	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/ui.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/plugins/ui.lua
file	nvim-lua-plugins-editor	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/editor.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/plugins/editor.lua
file	nvim-lua-plugins-lsp	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/lsp.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/plugins/lsp.lua
file	nvim-lua-plugins-completion	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/completion.lua	$SELFISHELL_ROOT/config/shared/nvim/lua/plugins/completion.lua
file	nvim-after-lsp-lua_ls	$SELFISHELL_CONFIG_DIR/nvim/after/lsp/lua_ls.lua	$SELFISHELL_ROOT/config/shared/nvim/after/lsp/lua_ls.lua
block	user-zshrc	$HOME/.zshrc	-
block	user-zprofile	$HOME/.zprofile	-
block	user-zshenv	$HOME/.zshenv	-
link	user-starship	${XDG_CONFIG_HOME:-$HOME/.config}/starship.toml	$SELFISHELL_CONFIG_DIR/starship.toml
block	user-vimrc	$HOME/.vimrc	-
link	user-nvim	${XDG_CONFIG_HOME:-$HOME/.config}/nvim	$SELFISHELL_CONFIG_DIR/nvim
block	user-ghostty	${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty	-
EOF
}

# Every caller consumes this through a process substitution of its own, so a
# shell-level loop here would be reading from one pipe while writing into
# another. A signal landing mid-write -- SIGCHLD from the producer exiting is
# enough -- makes that write fail with EINTR on macOS, and Bash 3.2 reports it
# instead of retrying: the list is then silently short, and `uninstall
# --restore` walks a set missing whatever came after the truncation while still
# reporting success. cut does the field selection in one process that retries
# for itself.
#
# -s keeps a delimiter-free row out of the generated list rather than turning it
# into a name. Declarations are all tab-separated, so this only bites on a
# malformed one, and it does not bury it either: the regression test requires
# these names to be the declarations' name column exactly, and a row that
# yields no name fails there.
selfishell_managed_resource_names() {
  selfishell_managed_resources | cut -s -f2
}
