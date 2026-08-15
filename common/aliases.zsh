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
