# General-purpose aliases enabled only when their corresponding command exists.

if _selfishell_command_path bat >/dev/null; then
  alias cat='bat'
elif _selfishell_command_path batcat >/dev/null; then
  alias cat='batcat'
fi

if _selfishell_command_path eza >/dev/null; then
  alias ls='eza --group-directories-first'
  alias ll='eza -l --group-directories-first --git'
fi

if _selfishell_command_path nvim >/dev/null; then
  alias vim='nvim'
  alias view='nvim -R'
fi

# Git is a required dependency in every profile, so these aliases need no
# _selfishell_command_path guard.
alias gst='git status -s'
alias gf='git fetch'
alias gl='git pull'
alias glog="git log --graph --decorate --pretty=format:'%C(auto)%h%d %s %C(dim white)<%an>%C(reset)'"
alias glogd="git log --graph --decorate --pretty=format:'%C(auto)%h%d %s %C(dim white)<%an> (%cr)%C(reset)'"
alias gcmsg='git commit -m'
alias gaa='git add -A'
alias gp='git push'
alias gco='git checkout'
alias gcb='git checkout -b'
alias gd='git diff'
alias gsta='git stash push'
alias gstp='git stash pop'
