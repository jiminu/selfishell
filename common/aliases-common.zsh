# General-purpose aliases that are not tied to a dedicated tool alias file.

if _selfishell_command_path bat >/dev/null; then
  alias cat='bat'
elif _selfishell_command_path batcat >/dev/null; then
  alias cat='batcat'
fi

if _selfishell_command_path eza >/dev/null; then
  alias ls='eza --icons=auto'
fi
