# General-purpose aliases that are not tied to a dedicated tool alias file.

if (( $+commands[bat] )); then
  alias cat='bat'
elif (( $+commands[batcat] )); then
  alias cat='batcat'
fi

if (( $+commands[eza] )); then
  alias ls='eza --icons=auto'
fi
