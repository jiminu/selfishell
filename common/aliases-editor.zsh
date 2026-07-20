# Editor aliases are only enabled when the corresponding editor exists.

if _selfishell_command_path nvim >/dev/null; then
  alias vim='nvim'
  alias view='nvim -R'
fi
