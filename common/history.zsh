# Persistent Zsh command history.
# Keep history available across shell sessions and store timestamps/durations
# so execution time can be inspected on demand without occupying the prompt.
HISTFILE="${ZDOTDIR:-$HOME}/.zsh_history"
HISTSIZE=10000
SAVEHIST=10000

setopt EXTENDED_HISTORY
setopt INC_APPEND_HISTORY_TIME
