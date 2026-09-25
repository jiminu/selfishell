#!/usr/bin/env bash

set -euo pipefail
SELFISHELL_ROOT="$1"
action="$2"
shift 2
source "$SELFISHELL_ROOT/lib/common.sh"
source "$SELFISHELL_ROOT/lib/paths.sh"
source "$SELFISHELL_ROOT/lib/managed.sh"
selfishell_initialize_paths

case "$action" in
  read)
    managed_read_state "$1"
    printf '%s\n' "$MANAGED_STATE_VERSION" "$MANAGED_STATE_TYPE" "$MANAGED_STATE_STATUS" \
      "$MANAGED_STATE_TARGET" "$MANAGED_STATE_REFERENCE" "$MANAGED_STATE_BACKUP" "$MANAGED_STATE_CHECKSUM"
    ;;
  install)
    kind="$1"
    if [[ "${SELFISHELL_TEST_INTERRUPT:-}" == 1 ]]; then
      case "$kind" in
        file) managed_atomic_copy() { exit 19; } ;;
        link) ln() { exit 19; } ;;
        block)
          mktemp() {
            [[ "$1" != "$HOME/.zshrc.tmp."* ]] || return 19
            command mktemp "$@"
          }
          ;;
      esac
    fi
    case "$kind" in
      file) managed_install_file fixture-file "$HOME/source" "$HOME/target" 0 1 ;;
      link) managed_install_link fixture-link "$HOME/target" "$HOME/source" 0 ;;
      block) managed_install_block user-zshrc "$HOME/.zshrc" 0 1 ;;
      *) exit 2 ;;
    esac
    ;;
  uninstall)
    managed_validate_uninstall_resource "$1"
    managed_uninstall_resource "$1" 1 0
    ;;
  resources)
    source "$SELFISHELL_ROOT/lib/resources.sh"
    selfishell_managed_resources
    ;;
  selected-resources)
    source "$SELFISHELL_ROOT/lib/resources.sh"
    source "$SELFISHELL_ROOT/lib/commands/install.sh"
    managed_install_file() { printf 'file\t%s\t%s\t%s\n' "$1" "$3" "$2"; }
    managed_install_link() { printf 'link\t%s\t%s\t%s\n' "$1" "$2" "$3"; }
    managed_install_block() { printf 'block\t%s\t%s\t-\n' "$1" "$2"; }
    install_managed_configuration "$1" 1 "$2"
    ;;
  *) exit 2 ;;
esac
