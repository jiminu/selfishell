#!/usr/bin/env bash

command_help() {
  require_no_arguments help "$@" || return

  cat <<'EOF'
Selfishell manages a consistent Zsh development environment.

Usage:
  selfishell <command>

Commands:
  install    Install managed shell configuration
  status     Show managed configuration status
  uninstall  Remove managed configuration
  update     Update approved tools and managed configuration
  self-update Update the Selfishell CLI release
  rollback   Switch back to a retained CLI release
  doctor     Diagnose platform and required dependencies
  version    Print the Selfishell version
  help       Show this help

Exit codes:
  0  Command completed successfully
  1  Environment or operation error
  2  Invalid command usage

The optional 'sfs' command is a shorthand for 'selfishell'.
EOF
}
