#!/usr/bin/env bash

managed_checksum() {
  cksum <"$1" | awk '{print $1 ":" $2}'
}

managed_state_path() {
  printf '%s/%s.state\n' "$SELFISHELL_RESOURCE_STATE_DIR" "$1"
}

managed_read_state() {
  local state_file
  state_file="$(managed_state_path "$1")"

  [[ -r "$state_file" ]] || return 1

  {
    IFS= read -r MANAGED_STATE_VERSION
    IFS= read -r MANAGED_STATE_TYPE
    IFS= read -r MANAGED_STATE_STATUS
    IFS= read -r MANAGED_STATE_TARGET
    IFS= read -r MANAGED_STATE_REFERENCE
    IFS= read -r MANAGED_STATE_BACKUP
    IFS= read -r MANAGED_STATE_CHECKSUM
  } <"$state_file"

  [[ "$MANAGED_STATE_VERSION" == "1" ]]
}

managed_write_state() {
  local resource="$1"
  local type="$2"
  local status="$3"
  local target="$4"
  local reference="$5"
  local backup="$6"
  local checksum="$7"
  local state_file
  local temporary_file

  mkdir -p "$SELFISHELL_RESOURCE_STATE_DIR"
  state_file="$(managed_state_path "$resource")"
  temporary_file="$(mktemp "${state_file}.tmp.XXXXXX")"

  {
    printf '1\n'
    printf '%s\n' "$type"
    printf '%s\n' "$status"
    printf '%s\n' "$target"
    printf '%s\n' "$reference"
    printf '%s\n' "$backup"
    printf '%s\n' "$checksum"
  } >"$temporary_file"

  mv "$temporary_file" "$state_file"
}

managed_remove_state() {
  rm -f "$(managed_state_path "$1")"
}

managed_unique_backup_path() {
  local target="$1"
  local backup_base
  local backup
  local suffix=0

  backup_base="${target}.backup.$(date +%Y%m%d%H%M%S)"
  backup="$backup_base"

  while [[ -e "$backup" || -L "$backup" ]]; do
    suffix=$((suffix + 1))
    backup="${backup_base}.${suffix}"
  done

  printf '%s\n' "$backup"
}

managed_atomic_copy() {
  local source_file="$1"
  local target_file="$2"
  local temporary_file

  mkdir -p "$(dirname "$target_file")"
  temporary_file="$(mktemp "${target_file}.tmp.XXXXXX")"
  cp "$source_file" "$temporary_file"
  chmod 0644 "$temporary_file"
  mv "$temporary_file" "$target_file"
}

managed_install_file() {
  local resource="$1"
  local source_file="$2"
  local target_file="$3"
  local dry_run="$4"
  local source_checksum
  local current_checksum=""
  local backup="-"

  source_checksum="$(managed_checksum "$source_file")"

  if managed_read_state "$resource"; then
    if [[ "$MANAGED_STATE_TYPE" != "file" || "$MANAGED_STATE_TARGET" != "$target_file" ]]; then
      cli_error "State conflict for managed file: $resource"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    backup="$MANAGED_STATE_BACKUP"

    if [[ -f "$target_file" ]]; then
      current_checksum="$(managed_checksum "$target_file")"
      if [[ "$current_checksum" != "$MANAGED_STATE_CHECKSUM" && "$current_checksum" != "$source_checksum" ]]; then
        if [[ "$MANAGED_STATE_STATUS" == "active" || "$backup" == "-" || -e "$backup" || -L "$backup" ]]; then
          cli_error "Managed file was modified; preserving it: $target_file"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        current_checksum=""
      fi
    elif [[ -e "$target_file" || -L "$target_file" ]]; then
      cli_error "Managed file path changed type; preserving it: $target_file"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  elif [[ -e "$target_file" || -L "$target_file" ]]; then
    backup="$(managed_unique_backup_path "$target_file")"
  fi

  if [[ "$current_checksum" == "$source_checksum" ]]; then
    if [[ "$dry_run" == "0" ]]; then
      managed_write_state "$resource" file active "$target_file" - "$backup" "$source_checksum"
    fi
    printf 'Unchanged: %s\n' "$target_file"
    return
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install managed file: %s\n' "$target_file"
    return
  fi

  managed_write_state "$resource" file pending "$target_file" - "$backup" "$source_checksum"
  if [[ "$backup" != "-" && ! -e "$backup" && ! -L "$backup" && (-e "$target_file" || -L "$target_file") ]]; then
    mv "$target_file" "$backup"
  fi
  managed_atomic_copy "$source_file" "$target_file"
  managed_write_state "$resource" file active "$target_file" - "$backup" "$source_checksum"
  printf 'Installed managed file: %s\n' "$target_file"
}

managed_install_link() {
  local resource="$1"
  local target_file="$2"
  local source_file="$3"
  local dry_run="$4"
  local backup="-"

  if managed_read_state "$resource"; then
    if [[ "$MANAGED_STATE_TYPE" != "link" || "$MANAGED_STATE_TARGET" != "$target_file" ]]; then
      cli_error "State conflict for managed link: $resource"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    backup="$MANAGED_STATE_BACKUP"

    if [[ -L "$target_file" && "$(readlink "$target_file")" == "$source_file" ]]; then
      if [[ "$dry_run" == "0" ]]; then
        managed_write_state "$resource" link active "$target_file" "$source_file" "$backup" -
      fi
      printf 'Unchanged: %s\n' "$target_file"
      return
    fi

    if [[ "$MANAGED_STATE_STATUS" == "active" && (-e "$target_file" || -L "$target_file") ]]; then
      cli_error "Managed link was replaced; preserving it: $target_file"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  elif [[ -e "$target_file" || -L "$target_file" ]]; then
    backup="$(managed_unique_backup_path "$target_file")"
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would link: %s -> %s\n' "$target_file" "$source_file"
    return
  fi

  managed_write_state "$resource" link pending "$target_file" "$source_file" "$backup" -
  mkdir -p "$(dirname "$target_file")"
  if [[ "$backup" != "-" && ! -e "$backup" && ! -L "$backup" && (-e "$target_file" || -L "$target_file") ]]; then
    mv "$target_file" "$backup"
  fi
  if [[ ! -e "$target_file" && ! -L "$target_file" ]]; then
    ln -s "$source_file" "$target_file"
  fi
  managed_write_state "$resource" link active "$target_file" "$source_file" "$backup" -
  printf 'Linked: %s -> %s\n' "$target_file" "$source_file"
}

managed_uninstall_resource() {
  local resource="$1"
  local restore="$2"
  local dry_run="$3"
  local current_checksum

  managed_read_state "$resource" || return 0

  case "$MANAGED_STATE_TYPE" in
    link)
      if [[ -L "$MANAGED_STATE_TARGET" && "$(readlink "$MANAGED_STATE_TARGET")" == "$MANAGED_STATE_REFERENCE" ]]; then
        if [[ "$dry_run" == "1" ]]; then
          printf 'Would remove managed link: %s\n' "$MANAGED_STATE_TARGET"
        else
          rm "$MANAGED_STATE_TARGET"
        fi
      elif [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        cli_error "Managed link was replaced; preserving it: $MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")"
        if [[ "$current_checksum" != "$MANAGED_STATE_CHECKSUM" ]]; then
          cli_error "Managed file was modified; preserving it: $MANAGED_STATE_TARGET"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        if [[ "$dry_run" == "1" ]]; then
          printf 'Would remove managed file: %s\n' "$MANAGED_STATE_TARGET"
        else
          rm "$MANAGED_STATE_TARGET"
        fi
      elif [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        cli_error "Managed file path changed type; preserving it: $MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    *)
      cli_error "Unknown managed resource type: $MANAGED_STATE_TYPE"
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  if [[ "$restore" == "1" && "$MANAGED_STATE_BACKUP" != "-" && (-e "$MANAGED_STATE_BACKUP" || -L "$MANAGED_STATE_BACKUP") ]]; then
    if [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
      cli_error "Restore target is occupied; preserving backup: $MANAGED_STATE_BACKUP"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    if [[ "$dry_run" == "1" ]]; then
      printf 'Would restore: %s -> %s\n' "$MANAGED_STATE_BACKUP" "$MANAGED_STATE_TARGET"
    else
      mkdir -p "$(dirname "$MANAGED_STATE_TARGET")"
      mv "$MANAGED_STATE_BACKUP" "$MANAGED_STATE_TARGET"
    fi
  fi

  if [[ "$dry_run" == "0" ]]; then
    managed_remove_state "$resource"
  fi
}

managed_validate_uninstall_resource() {
  local resource="$1"
  local current_checksum

  managed_read_state "$resource" || return 0

  case "$MANAGED_STATE_TYPE" in
    link)
      if [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        if [[ ! -L "$MANAGED_STATE_TARGET" || "$(readlink "$MANAGED_STATE_TARGET")" != "$MANAGED_STATE_REFERENCE" ]]; then
          cli_error "Managed link was replaced; preserving it: $MANAGED_STATE_TARGET"
          return "$SELFISHELL_EXIT_ERROR"
        fi
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")"
        if [[ "$current_checksum" != "$MANAGED_STATE_CHECKSUM" ]]; then
          cli_error "Managed file was modified; preserving it: $MANAGED_STATE_TARGET"
          return "$SELFISHELL_EXIT_ERROR"
        fi
      elif [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        cli_error "Managed file path changed type; preserving it: $MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    *)
      cli_error "Unknown managed resource type: $MANAGED_STATE_TYPE"
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}
