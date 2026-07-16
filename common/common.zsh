# Selfishell shared interactive shell entrypoint.
# Keep ordering explicit: later modules depend on functions and bindings set up
# by earlier modules.
SELFISHELL_COMMON_DIR="${${(%):-%x}:A:h}"

_selfishell_command_path() {
  local command_name="$1"
  local directory

  for directory in $path; do
    # Selfishell installs Linux tools inside WSL. Avoid slow filesystem probes
    # through inherited Windows PATH entries when checking optional tools.
    if [[ -n "${WSL_DISTRO_NAME:-}" && "$directory" == /mnt/[a-zA-Z]/* ]]; then
      continue
    fi
    if [[ -x "$directory/$command_name" && ! -d "$directory/$command_name" ]]; then
      print -r -- "$directory/$command_name"
      return 0
    fi
  done
  return 1
}

source "$SELFISHELL_COMMON_DIR/runtime.zsh"
source "$SELFISHELL_COMMON_DIR/completion.zsh"
source "$SELFISHELL_COMMON_DIR/interactive.zsh"
source "$SELFISHELL_COMMON_DIR/update-notice.zsh"

unset SELFISHELL_COMMON_DIR
