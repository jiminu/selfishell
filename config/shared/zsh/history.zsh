# Persist history with timestamps and durations across shell sessions.
HISTFILE="${ZDOTDIR:-$HOME}/.zsh_history"
HISTSIZE=10000
SAVEHIST=10000

setopt EXTENDED_HISTORY
setopt INC_APPEND_HISTORY_TIME
setopt HIST_IGNORE_SPACE

# Normalize whitespace for fzf Ctrl-R deduplication while retaining timestamps and durations.
setopt HIST_REDUCE_BLANKS
