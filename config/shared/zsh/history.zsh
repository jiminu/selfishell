# Persistent Zsh command history.
# Keep history available across shell sessions and store timestamps/durations
# so execution time can be inspected on demand without occupying the prompt.
HISTFILE="${ZDOTDIR:-$HOME}/.zsh_history"
HISTSIZE=10000
SAVEHIST=10000

setopt EXTENDED_HISTORY
setopt INC_APPEND_HISTORY_TIME
setopt HIST_IGNORE_SPACE

# Normalize spacing before a line is recorded. Ctrl-R is fzf's widget and it
# drops duplicates by exact string match, so `git status` typed with a stray
# double space is a second entry competing for the same slot. This makes that
# deduplication work rather than adding another layer of it: the entry, its
# timestamp and its duration are all still kept, which the dup-pruning options
# cannot say.
setopt HIST_REDUCE_BLANKS
