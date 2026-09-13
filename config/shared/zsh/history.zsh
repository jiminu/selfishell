# Persistent Zsh command history.
# Keep history available across shell sessions and store timestamps/durations
# so execution time can be inspected on demand without occupying the prompt.
HISTFILE="${ZDOTDIR:-$HOME}/.zsh_history"
HISTSIZE=10000
SAVEHIST=10000

setopt EXTENDED_HISTORY
setopt INC_APPEND_HISTORY_TIME
setopt HIST_IGNORE_SPACE

# fzf's Ctrl-R drops duplicates by exact string match, so a stray double space
# makes a second entry. Normalizing feeds that dedup rather than adding another
# layer, and unlike the dup-pruning options it keeps timestamp and duration.
setopt HIST_REDUCE_BLANKS
