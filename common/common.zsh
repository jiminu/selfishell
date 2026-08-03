# Selfishell shared interactive shell entrypoint.
# Keep ordering explicit: later modules depend on functions and bindings set up
# by earlier modules.
SELFISHELL_COMMON_DIR="${${(%):-%x}:A:h}"

_selfishell_command_path() {
  local command_name="$1"
  local directory

  if [[ -z "${WSL_DISTRO_NAME:-}" ]]; then
    for directory in $path; do
      if [[ -n "$directory" && "$directory" != /* ]]; then
        builtin whence -p -- "$command_name"
        return
      fi
    done
    if (( ${path[(I)]} )) && [[ -x "$command_name" && ! -d "$command_name" ]]; then
      print -r -- "$command_name"
      return 0
    fi
    if (( ${+commands[$command_name]} )); then
      print -r -- "${commands[$command_name]}"
      return 0
    fi
    return 1
  fi

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

# Selfishell provisions every pinned Zsh plugin during installation; startup
# never downloads one. Require a complete checkout rather than just the plugin
# directory, so an interrupted or partial clone stays quiet instead of making
# Zinit fail during startup. `selfishell doctor` reports the missing checkout.
_selfishell_zinit_plugin_ready() {
  local repository="$1"
  [[ -n "${ZINIT[PLUGINS_DIR]:-}" ]] || return 1
  [[ -d "$ZINIT[PLUGINS_DIR]/${repository//\//---}/.git" ]]
}

source "$SELFISHELL_COMMON_DIR/runtime.zsh"
source "$SELFISHELL_COMMON_DIR/completion.zsh"
source "$SELFISHELL_COMMON_DIR/interactive.zsh"
source "$SELFISHELL_COMMON_DIR/update-notice.zsh"

unset SELFISHELL_COMMON_DIR
