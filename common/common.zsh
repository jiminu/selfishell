# Selfishell shared interactive shell entrypoint.
# Keep ordering explicit: later modules depend on functions and bindings set up
# by earlier modules.
SELFISHELL_COMMON_DIR="${${(%):-%x}:A:h}"

source "$SELFISHELL_COMMON_DIR/runtime.zsh"
source "$SELFISHELL_COMMON_DIR/completion.zsh"
source "$SELFISHELL_COMMON_DIR/interactive.zsh"
source "$SELFISHELL_COMMON_DIR/update-notice.zsh"

unset SELFISHELL_COMMON_DIR
