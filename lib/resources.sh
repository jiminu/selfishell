#!/usr/bin/env bash

selfishell_managed_resources() {
  cat <<EOF
file	zshrc-config	$SELFISHELL_CONFIG_DIR/zsh/zshrc	$SELFISHELL_ROOT/mac/.zshrc
file	zshenv-config	$SELFISHELL_CONFIG_DIR/zsh/zshenv	$SELFISHELL_ROOT/common/zshenv
file	zsh-runtime	$SELFISHELL_CONFIG_DIR/zsh/runtime.zsh	$SELFISHELL_ROOT/common/runtime.zsh
file	mise-config	$SELFISHELL_CONFIG_DIR/mise/config.toml	$SELFISHELL_ROOT/common/mise.toml
file	zsh-completion	$SELFISHELL_CONFIG_DIR/zsh/completion.zsh	$SELFISHELL_ROOT/common/completion.zsh
file	zsh-interactive	$SELFISHELL_CONFIG_DIR/zsh/interactive.zsh	$SELFISHELL_ROOT/common/interactive.zsh
file	zsh-update-notice	$SELFISHELL_CONFIG_DIR/zsh/update-notice.zsh	$SELFISHELL_ROOT/common/update-notice.zsh
file	zsh-common	$SELFISHELL_CONFIG_DIR/zsh/common.zsh	$SELFISHELL_ROOT/common/common.zsh
file	aliases-common	$SELFISHELL_CONFIG_DIR/zsh/aliases-common.zsh	$SELFISHELL_ROOT/common/aliases-common.zsh
file	aliases-editor	$SELFISHELL_CONFIG_DIR/zsh/aliases-editor.zsh	$SELFISHELL_ROOT/common/aliases-editor.zsh
file	aliases-git	$SELFISHELL_CONFIG_DIR/zsh/aliases-git.zsh	$SELFISHELL_ROOT/common/aliases-git.zsh
file	aliases-kubectl	$SELFISHELL_CONFIG_DIR/zsh/aliases-kubectl.zsh	$SELFISHELL_ROOT/common/aliases-kubectl.zsh
file	vimrc	$SELFISHELL_CONFIG_DIR/vim/vimrc	$SELFISHELL_ROOT/common/vimrc
file	starship-config	$SELFISHELL_CONFIG_DIR/starship.toml	$SELFISHELL_ROOT/common/starship.toml
file	ghostty-config	$SELFISHELL_CONFIG_DIR/ghostty/config	$SELFISHELL_ROOT/mac/config.ghostty
file	nvim-init	$SELFISHELL_CONFIG_DIR/nvim/init.lua	$SELFISHELL_ROOT/common/nvim/init.lua
file	nvim-lua-config-options	$SELFISHELL_CONFIG_DIR/nvim/lua/config/options.lua	$SELFISHELL_ROOT/common/nvim/lua/config/options.lua
file	nvim-lua-config-keymaps	$SELFISHELL_CONFIG_DIR/nvim/lua/config/keymaps.lua	$SELFISHELL_ROOT/common/nvim/lua/config/keymaps.lua
file	nvim-lua-config-autocmds	$SELFISHELL_CONFIG_DIR/nvim/lua/config/autocmds.lua	$SELFISHELL_ROOT/common/nvim/lua/config/autocmds.lua
file	nvim-lua-config-lazy	$SELFISHELL_CONFIG_DIR/nvim/lua/config/lazy.lua	$SELFISHELL_ROOT/common/nvim/lua/config/lazy.lua
file	nvim-lua-config-languages	$SELFISHELL_CONFIG_DIR/nvim/lua/config/languages.lua	$SELFISHELL_ROOT/common/nvim/lua/config/languages.lua
file	nvim-lua-plugins-ui	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/ui.lua	$SELFISHELL_ROOT/common/nvim/lua/plugins/ui.lua
file	nvim-lua-plugins-editor	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/editor.lua	$SELFISHELL_ROOT/common/nvim/lua/plugins/editor.lua
file	nvim-lua-plugins-lsp	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/lsp.lua	$SELFISHELL_ROOT/common/nvim/lua/plugins/lsp.lua
file	nvim-lua-plugins-completion	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/completion.lua	$SELFISHELL_ROOT/common/nvim/lua/plugins/completion.lua
file	nvim-lua-plugins-telescope	$SELFISHELL_CONFIG_DIR/nvim/lua/plugins/telescope.lua	$SELFISHELL_ROOT/common/nvim/lua/plugins/telescope.lua
file	nvim-after-lsp-lua_ls	$SELFISHELL_CONFIG_DIR/nvim/after/lsp/lua_ls.lua	$SELFISHELL_ROOT/common/nvim/after/lsp/lua_ls.lua
link	user-zshrc	$HOME/.zshrc	$SELFISHELL_CONFIG_DIR/zsh/zshrc
link	user-zshenv	$HOME/.zshenv	$SELFISHELL_CONFIG_DIR/zsh/zshenv
link	user-starship	${XDG_CONFIG_HOME:-$HOME/.config}/starship.toml	$SELFISHELL_CONFIG_DIR/starship.toml
link	user-vimrc	${XDG_CONFIG_HOME:-$HOME/.config}/vim/vimrc	$SELFISHELL_CONFIG_DIR/vim/vimrc
link	user-nvim	${XDG_CONFIG_HOME:-$HOME/.config}/nvim	$SELFISHELL_CONFIG_DIR/nvim
link	user-ghostty	${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config	$SELFISHELL_CONFIG_DIR/ghostty/config
EOF
}

selfishell_managed_resource_names() {
  cat <<'EOF'
zshrc-config
zshenv-config
zsh-runtime
mise-config
zsh-completion
zsh-interactive
zsh-update-notice
zsh-common
aliases-common
aliases-editor
aliases-git
aliases-kubectl
starship-config
vimrc
ghostty-config
nvim-init
nvim-lua-config-options
nvim-lua-config-keymaps
nvim-lua-config-autocmds
nvim-lua-config-lazy
nvim-lua-config-languages
nvim-lua-plugins-ui
nvim-lua-plugins-editor
nvim-lua-plugins-lsp
nvim-lua-plugins-completion
nvim-lua-plugins-telescope
nvim-after-lsp-lua_ls
user-zshrc
user-zshenv
user-starship
user-vimrc
user-nvim
user-ghostty
EOF
}
