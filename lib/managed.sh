#!/usr/bin/env bash

managed_checksum() {
  cksum <"$1" | awk '{print $1 ":" $2}'
}

managed_state_path() {
  printf '%s/%s.state\n' "$SELFISHELL_RESOURCE_STATE_DIR" "$1"
}

managed_state_exists() {
  [[ -e "$(managed_state_path "$1")" ]]
}

# Returns 1 for both a missing state file and a malformed one (short,
# truncated, or holding unrecognized field values); callers that must tell
# the two apart -- so a corrupted state can't be silently treated as "no
# state" and its resource mistaken for a fresh install -- check
# managed_state_exists() themselves after this returns false.
managed_read_state() {
  local state_file
  state_file="$(managed_state_path "$1")"

  [[ -r "$state_file" ]] || return 1

  if ! {
    IFS= read -r MANAGED_STATE_VERSION
    IFS= read -r MANAGED_STATE_TYPE
    IFS= read -r MANAGED_STATE_STATUS
    IFS= read -r MANAGED_STATE_TARGET
    IFS= read -r MANAGED_STATE_REFERENCE
    IFS= read -r MANAGED_STATE_BACKUP
    IFS= read -r MANAGED_STATE_CHECKSUM
  } <"$state_file"; then
    return 1
  fi

  case "$MANAGED_STATE_VERSION" in 1 | 2) ;; *) return 1 ;; esac
  case "$MANAGED_STATE_TYPE" in file | link | block) ;; *) return 1 ;; esac
  case "$MANAGED_STATE_STATUS" in pending | active) ;; *) return 1 ;; esac
  [[ -n "$MANAGED_STATE_TARGET" ]] || return 1
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

  mkdir -p "$SELFISHELL_RESOURCE_STATE_DIR" || return "$SELFISHELL_EXIT_ERROR"
  state_file="$(managed_state_path "$resource")"
  temporary_file="$(mktemp "${state_file}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"

  if ! printf '2\n%s\n%s\n%s\n%s\n%s\n%s\n' \
    "$type" "$status" "$target" "$reference" "$backup" "$checksum" >"$temporary_file"; then
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  mv "$temporary_file" "$state_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
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

  mkdir -p "$(dirname "$target_file")" || return "$SELFISHELL_EXIT_ERROR"
  temporary_file="$(mktemp "${target_file}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"

  cp "$source_file" "$temporary_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  chmod 0644 "$temporary_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  mv "$temporary_file" "$target_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
}

# Managed regular-file conflicts are handled from inside a
# `while ... done < <(selfishell_managed_resources)` loop, which redirects
# FD 0 to the resource list for the duration of the loop. FD 3 is a copy of
# the real stdin created once in lib/common.sh before that redirection takes
# effect, so conflict prompts must check and read FD 3, not FD 0.
managed_conflict_is_interactive() {
  selfishell_is_interactive
}

managed_read_conflict_answer() {
  local answer=""

  IFS= read -r answer <&3 || return "$SELFISHELL_EXIT_ERROR"
  printf '%s\n' "$answer"
}

managed_block_definition() {
  local resource="$1"

  case "$resource" in
    user-zshrc)
      MANAGED_BLOCK_LABEL='Selfishell initialize'
      # shellcheck disable=SC2016 # Literal for zsh to expand at its own startup, not now.
      MANAGED_BLOCK_BODY='if [[ -r "${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/zsh/zshrc" ]]; then
  source "${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/zsh/zshrc"
fi'
      ;;
    user-ghostty)
      MANAGED_BLOCK_LABEL='Selfishell ghostty'
      # config-file directives are processed in declaration order, but always
      # after every other key in this file. Declaring the optional override
      # second means user.ghostty (if present) is applied after, and so wins
      # over, the Selfishell defaults included first.
      MANAGED_BLOCK_BODY="config-file = $SELFISHELL_CONFIG_DIR/ghostty/config.ghostty
# To override a Selfishell default above, add it to user.ghostty instead.
config-file = ?user.ghostty"
      ;;
    *)
      cli_error "Unknown managed block resource: $resource"
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}

managed_block_begin() {
  printf '# >>> %s >>>\n' "$1"
}

managed_block_end() {
  printf '# <<< %s <<<\n' "$1"
}

managed_block_content() {
  local resource="$1"

  managed_block_definition "$resource" || return
  printf '%s\n%s\n%s\n' "$(managed_block_begin "$MANAGED_BLOCK_LABEL")" "$MANAGED_BLOCK_BODY" "$(managed_block_end "$MANAGED_BLOCK_LABEL")"
}

# Sets MANAGED_BLOCK_STATUS to absent/malformed/intact based purely on marker
# structure, and MANAGED_BLOCK_CHECKSUM to the live block bytes' checksum.
# "intact" means the markers are well-formed -- it deliberately does NOT
# compare against managed_block_content's current output, so a resource whose
# body has legitimately changed across a Selfishell release (unlike the
# hand-written checksum a user would produce by editing the block) is never
# mistaken for user tampering. Callers that need to know whether the content
# is up to date or was actually modified compare MANAGED_BLOCK_CHECKSUM
# against their own reference checksum themselves.
managed_inspect_block() {
  local resource="$1"
  local target_file="$2"
  local begin_marker end_marker metadata
  local begin_count end_count related_count start finish

  MANAGED_BLOCK_STATUS=absent
  MANAGED_BLOCK_START=0
  MANAGED_BLOCK_LENGTH=0
  MANAGED_BLOCK_CHECKSUM=""

  managed_block_definition "$resource" || return

  [[ -f "$target_file" && ! -L "$target_file" ]] || return 0
  begin_marker="$(managed_block_begin "$MANAGED_BLOCK_LABEL")"
  end_marker="$(managed_block_end "$MANAGED_BLOCK_LABEL")"
  metadata="$(LC_ALL=C awk -v begin="$begin_marker" -v end="$end_marker" -v label="$MANAGED_BLOCK_LABEL" '
    BEGIN { offset = 0; begin_count = 0; end_count = 0; related_count = 0; start = 0; finish = 0 }
    {
      if (index($0, label) > 0) related_count++
      if ($0 == begin) {
        begin_count++
        start = offset
      }
      offset += length($0) + 1
      if ($0 == end) {
        end_count++
        finish = offset
      }
    }
    END { print begin_count, end_count, related_count, start, finish }
  ' "$target_file")" || return 1
  read -r begin_count end_count related_count start finish <<<"$metadata"

  if [[ "$begin_count" == 0 && "$end_count" == 0 && "$related_count" == 0 ]]; then
    return 0
  fi
  if [[ "$begin_count" != 1 || "$end_count" != 1 || "$related_count" != 2 || "$finish" -le "$start" ]]; then
    MANAGED_BLOCK_STATUS=malformed
    return 0
  fi

  MANAGED_BLOCK_START="$start"
  MANAGED_BLOCK_LENGTH=$((finish - start))
  MANAGED_BLOCK_CHECKSUM="$(dd if="$target_file" bs=1 skip="$start" count="$MANAGED_BLOCK_LENGTH" 2>/dev/null | cksum | awk '{print $1 ":" $2}')"
  MANAGED_BLOCK_STATUS=intact
}

managed_block_error() {
  local resource="$1"
  local target_file="$2"

  cli_error "Cannot manage the Selfishell $resource block in: $target_file"
  cli_error "Preserving the file. Remove conflicting Selfishell markers and retry."
}

managed_preflight_block_target() {
  local resource="$1"
  local target_file="$2"

  if [[ -L "$target_file" ]]; then
    cli_error "Refusing to modify symbolic link: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi
  if [[ -e "$target_file" && ! -f "$target_file" ]]; then
    cli_error "Refusing to modify non-regular block path: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  managed_inspect_block "$resource" "$target_file" || return
  case "$MANAGED_BLOCK_STATUS" in
    malformed)
      managed_block_error "$resource" "$target_file"
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}

managed_preflight_zsh_loader() {
  local target_file="$HOME/.zshrc"
  local state_file legacy_version legacy_type

  state_file="$(managed_state_path user-zshrc)"
  if [[ -r "$state_file" ]]; then
    legacy_version="$(sed -n '1p' "$state_file")"
    legacy_type="$(sed -n '2p' "$state_file")"
    if [[ "$legacy_version" != 2 || "$legacy_type" != block ]]; then
      cli_error "Legacy Selfishell .zshrc management was detected."
      cli_error "Run 'selfishell uninstall --restore --yes', move any wanted local.zsh settings into ~/.zshrc, then reinstall."
      return "$SELFISHELL_EXIT_ERROR"
    fi
  fi

  if [[ -L "$target_file" ]]; then
    cli_error "Refusing to modify symbolic link: $target_file"
    cli_error "Replace it with a regular user-owned .zshrc, then retry."
    return "$SELFISHELL_EXIT_ERROR"
  fi
  if [[ -e "$target_file" && ! -f "$target_file" ]]; then
    cli_error "Refusing to modify non-regular startup path: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  managed_inspect_block user-zshrc "$target_file" || return
  case "$MANAGED_BLOCK_STATUS" in
    malformed)
      managed_block_error user-zshrc "$target_file"
      return "$SELFISHELL_EXIT_ERROR"
      ;;
    intact)
      if [[ ! -r "$state_file" ]]; then
        cli_error "An untracked Selfishell loader already exists in: $target_file"
        cli_error "Remove the loader block, then retry."
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
  esac
}

managed_install_block() {
  local resource="$1"
  local target_file="$2"
  local dry_run="$3"
  local expected_checksum temporary_file reference

  if [[ -L "$target_file" ]]; then
    cli_error "Refusing to modify symbolic link: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi
  if [[ -e "$target_file" && ! -f "$target_file" ]]; then
    cli_error "Refusing to modify non-regular block path: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  expected_checksum="$(managed_block_content "$resource" | cksum | awk '{print $1 ":" $2}')"
  managed_inspect_block "$resource" "$target_file" || return
  reference="selfishell-${resource}-block-v1"

  if managed_read_state "$resource"; then
    if [[ "$MANAGED_STATE_VERSION" != 2 || "$MANAGED_STATE_TYPE" != block || "$MANAGED_STATE_TARGET" != "$target_file" ]]; then
      cli_error "Legacy Selfishell state was detected for: $resource"
      cli_error "Run 'selfishell uninstall --restore --yes', then reinstall."
      return "$SELFISHELL_EXIT_ERROR"
    fi
    if [[ "$MANAGED_BLOCK_STATUS" == intact && "$MANAGED_BLOCK_CHECKSUM" == "$expected_checksum" ]]; then
      if [[ "$dry_run" == 0 ]]; then
        managed_write_state "$resource" block active "$target_file" "$reference" - "$expected_checksum" || return "$SELFISHELL_EXIT_ERROR"
      fi
      printf '%sUnchanged Selfishell block:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file"
      return 0
    fi
    if [[ "$MANAGED_BLOCK_STATUS" == intact && "$MANAGED_BLOCK_CHECKSUM" == "$MANAGED_STATE_CHECKSUM" ]]; then
      # Untouched since it was installed, but Selfishell's own content for
      # this resource changed since then (not user tampering) -- the block
      # rewrite below only knows how to add a fresh block, not splice an
      # updated one into an existing file, so ask for a clean reinstall
      # rather than risk creating a duplicate block.
      cli_error "Legacy Selfishell state was detected for: $resource"
      cli_error "Run 'selfishell uninstall --restore --yes', then reinstall."
      return "$SELFISHELL_EXIT_ERROR"
    fi
    if [[ "$MANAGED_STATE_STATUS" != pending || "$MANAGED_BLOCK_STATUS" != absent ]]; then
      managed_block_error "$resource" "$target_file"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  elif managed_state_exists "$resource"; then
    cli_error "Managed resource state is malformed: $(managed_state_path "$resource")"
    return "$SELFISHELL_EXIT_ERROR"
  elif [[ "$MANAGED_BLOCK_STATUS" != absent ]]; then
    managed_block_error "$resource" "$target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  if [[ "$dry_run" == 1 ]]; then
    printf '%sWould add Selfishell block:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file"
    return 0
  fi

  managed_write_state "$resource" block pending "$target_file" "$reference" - "$expected_checksum" || return "$SELFISHELL_EXIT_ERROR"
  mkdir -p "$(dirname "$target_file")" || return "$SELFISHELL_EXIT_ERROR"
  temporary_file="$(mktemp "${target_file}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"
  if [[ -f "$target_file" ]]; then
    cp -p "$target_file" "$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
    : >"$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
  else
    chmod 0644 "$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi
  managed_block_content "$resource" >"$temporary_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  if [[ -f "$target_file" ]]; then
    cat "$target_file" >>"$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi
  mv "$temporary_file" "$target_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  managed_write_state "$resource" block active "$target_file" "$reference" - "$expected_checksum" || return "$SELFISHELL_EXIT_ERROR"
  printf '%sAdded Selfishell block:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$target_file"
}

managed_remove_block() {
  local resource="$1"
  local target_file="$2"
  local temporary_file file_size suffix_start

  managed_inspect_block "$resource" "$target_file" || return
  if [[ "$MANAGED_BLOCK_STATUS" != intact || "$MANAGED_BLOCK_CHECKSUM" != "$MANAGED_STATE_CHECKSUM" ]]; then
    managed_block_error "$resource" "$target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  temporary_file="$(mktemp "${target_file}.tmp.XXXXXX")" || return "$SELFISHELL_EXIT_ERROR"
  cp -p "$target_file" "$temporary_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  : >"$temporary_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
  if ((MANAGED_BLOCK_START > 0)); then
    dd if="$target_file" bs=1 count="$MANAGED_BLOCK_START" 2>/dev/null >"$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi
  file_size="$(LC_ALL=C wc -c <"$target_file")"
  suffix_start=$((MANAGED_BLOCK_START + MANAGED_BLOCK_LENGTH))
  if ((suffix_start < file_size)); then
    dd if="$target_file" bs=1 skip="$suffix_start" 2>/dev/null >>"$temporary_file" || {
      rm -f "$temporary_file"
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi
  mv "$temporary_file" "$target_file" || {
    rm -f "$temporary_file"
    return "$SELFISHELL_EXIT_ERROR"
  }
}

managed_install_file() {
  local resource="$1"
  local source_file="$2"
  local target_file="$3"
  local dry_run="$4"
  local assume_yes="${5:-0}"
  local source_checksum
  local current_checksum=""
  local original_backup="-"
  local conflict_backup=""
  local answer=""

  source_checksum="$(managed_checksum "$source_file")"
  if managed_read_state "$resource"; then
    if [[ "$MANAGED_STATE_TYPE" != "file" || "$MANAGED_STATE_TARGET" != "$target_file" ]]; then
      cli_error "State conflict for managed file: $resource"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    original_backup="$MANAGED_STATE_BACKUP"

    if [[ -f "$target_file" ]]; then
      current_checksum="$(managed_checksum "$target_file")"
      if [[ "$current_checksum" != "$MANAGED_STATE_CHECKSUM" && "$current_checksum" != "$source_checksum" ]]; then
        if [[ "$MANAGED_STATE_STATUS" == "active" || "$original_backup" == "-" || -e "$original_backup" || -L "$original_backup" ]]; then
          if [[ "$dry_run" == "1" ]]; then
            printf '%sConflict: modified managed file:%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$target_file"
            printf '%sWould require an overwrite or skip decision.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
            return 0
          fi

          if [[ "$assume_yes" == "1" ]] || ! managed_conflict_is_interactive; then
            cli_error "Managed file was modified; preserving it: $target_file"
            return "$SELFISHELL_EXIT_ERROR"
          fi

          printf '%sManaged file was modified:%s %s. Overwrite with default config? [y/N] ' \
            "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$target_file"
          if ! answer="$(managed_read_conflict_answer)"; then
            cli_error "Managed file was modified; preserving it: $target_file"
            return "$SELFISHELL_EXIT_ERROR"
          fi

          case "$answer" in
            y | Y | yes | YES)
              conflict_backup="$(managed_unique_backup_path "$SELFISHELL_STATE_DIR/backups/$resource")"
              mkdir -p "$(dirname "$conflict_backup")" || return "$SELFISHELL_EXIT_ERROR"
              cp -p "$target_file" "$conflict_backup" || return "$SELFISHELL_EXIT_ERROR"
              printf '%sBacked up modified managed file:%s %s -> %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$target_file" "$conflict_backup"
              managed_atomic_copy "$source_file" "$target_file" || return "$SELFISHELL_EXIT_ERROR"
              managed_write_state "$resource" file active "$target_file" - "$original_backup" "$source_checksum" || return "$SELFISHELL_EXIT_ERROR"
              printf '%sInstalled managed file:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$target_file"
              return 0
              ;;
            *)
              printf '%sSkipped modified managed file:%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$target_file"
              return 0
              ;;
          esac
        else
          current_checksum=""
        fi
      fi
    elif [[ -e "$target_file" || -L "$target_file" ]]; then
      cli_error "Managed file path changed type; preserving it: $target_file"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  elif managed_state_exists "$resource"; then
    cli_error "Managed resource state is malformed: $(managed_state_path "$resource")"
    return "$SELFISHELL_EXIT_ERROR"
  elif [[ -e "$target_file" || -L "$target_file" ]]; then
    original_backup="$(managed_unique_backup_path "$target_file")"
  fi

  if [[ "$current_checksum" == "$source_checksum" ]]; then
    if [[ "$dry_run" == "0" ]]; then
      managed_write_state "$resource" file active "$target_file" - "$original_backup" "$source_checksum" || return "$SELFISHELL_EXIT_ERROR"
    fi
    printf '%sUnchanged:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file"
    return
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould install managed file:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file"
    return
  fi

  managed_write_state "$resource" file pending "$target_file" - "$original_backup" "$source_checksum" || return "$SELFISHELL_EXIT_ERROR"
  if [[ "$original_backup" != "-" && ! -e "$original_backup" && ! -L "$original_backup" && (-e "$target_file" || -L "$target_file") ]]; then
    mkdir -p "$(dirname "$original_backup")" || return "$SELFISHELL_EXIT_ERROR"
    mv "$target_file" "$original_backup" || return "$SELFISHELL_EXIT_ERROR"
  fi
  managed_atomic_copy "$source_file" "$target_file" || return "$SELFISHELL_EXIT_ERROR"
  managed_write_state "$resource" file active "$target_file" - "$original_backup" "$source_checksum" || return "$SELFISHELL_EXIT_ERROR"
  printf '%sInstalled managed file:%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$target_file"
}

managed_install_link() {
  local resource="$1"
  local target_file="$2"
  local source_file="$3"
  local dry_run="$4"
  local backup="-"
  local moved_to_backup=0

  if managed_read_state "$resource"; then
    if [[ "$MANAGED_STATE_TYPE" != "link" || "$MANAGED_STATE_TARGET" != "$target_file" ]]; then
      cli_error "State conflict for managed link: $resource"
      return "$SELFISHELL_EXIT_ERROR"
    fi
    backup="$MANAGED_STATE_BACKUP"

    if [[ -L "$target_file" && "$(readlink "$target_file")" == "$source_file" ]]; then
      if [[ "$dry_run" == "0" ]]; then
        managed_write_state "$resource" link active "$target_file" "$source_file" "$backup" - || return "$SELFISHELL_EXIT_ERROR"
      fi
      printf '%sUnchanged:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file"
      return
    fi

    if [[ "$MANAGED_STATE_STATUS" == "active" && (-e "$target_file" || -L "$target_file") ]]; then
      cli_error "Managed link was replaced; preserving it: $target_file"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  elif managed_state_exists "$resource"; then
    cli_error "Managed resource state is malformed: $(managed_state_path "$resource")"
    return "$SELFISHELL_EXIT_ERROR"
  elif [[ -e "$target_file" || -L "$target_file" ]]; then
    backup="$(managed_unique_backup_path "$target_file")"
  fi

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould link:%s %s -> %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$target_file" "$source_file"
    return
  fi

  managed_write_state "$resource" link pending "$target_file" "$source_file" "$backup" - || return "$SELFISHELL_EXIT_ERROR"
  mkdir -p "$(dirname "$target_file")" || return "$SELFISHELL_EXIT_ERROR"
  if [[ "$backup" != "-" && ! -e "$backup" && ! -L "$backup" && (-e "$target_file" || -L "$target_file") ]]; then
    mv "$target_file" "$backup" || return "$SELFISHELL_EXIT_ERROR"
    moved_to_backup=1
  fi
  if [[ ! -e "$target_file" && ! -L "$target_file" ]]; then
    if ! ln -s "$source_file" "$target_file"; then
      if [[ "$moved_to_backup" == 1 && ! -e "$target_file" && ! -L "$target_file" && (-e "$backup" || -L "$backup") ]]; then
        if mv "$backup" "$target_file"; then
          managed_remove_state "$resource"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        cli_error "Failed to restore original managed-link target from backup: $backup"
      fi
      return "$SELFISHELL_EXIT_ERROR"
    fi
  fi
  if [[ ! -L "$target_file" || "$(readlink "$target_file")" != "$source_file" ]]; then
    cli_error "Failed to install managed link: $target_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi
  managed_write_state "$resource" link active "$target_file" "$source_file" "$backup" - || return "$SELFISHELL_EXIT_ERROR"
  printf '%sLinked:%s %s -> %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$target_file" "$source_file"
}

managed_uninstall_resource() {
  local resource="$1"
  local restore="$2"
  local dry_run="$3"
  local current_checksum

  if ! managed_read_state "$resource"; then
    managed_state_exists "$resource" || return 0
    cli_error "Managed resource state is malformed: $(managed_state_path "$resource")"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  case "$MANAGED_STATE_TYPE" in
    block)
      if [[ "$dry_run" == 1 ]]; then
        printf '%sWould remove Selfishell block:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
      else
        managed_remove_block "$resource" "$MANAGED_STATE_TARGET" || return
      fi
      ;;
    link)
      if [[ -L "$MANAGED_STATE_TARGET" && "$(readlink "$MANAGED_STATE_TARGET")" == "$MANAGED_STATE_REFERENCE" ]]; then
        if [[ "$dry_run" == "1" ]]; then
          printf '%sWould remove managed link:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
        else
          rm "$MANAGED_STATE_TARGET" || return
        fi
      elif [[ -e "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        cli_error "Managed link was replaced; preserving it: $MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")" || return
        if [[ "$current_checksum" != "$MANAGED_STATE_CHECKSUM" ]]; then
          cli_error "Managed file was modified; preserving it: $MANAGED_STATE_TARGET"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        if [[ "$dry_run" == "1" ]]; then
          printf '%sWould remove managed file:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
        else
          rm "$MANAGED_STATE_TARGET" || return
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
      printf '%sWould restore:%s %s -> %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_BACKUP" "$MANAGED_STATE_TARGET"
    else
      mkdir -p "$(dirname "$MANAGED_STATE_TARGET")" || return
      mv "$MANAGED_STATE_BACKUP" "$MANAGED_STATE_TARGET" || return
    fi
  fi

  if [[ "$dry_run" == "0" ]]; then
    managed_remove_state "$resource" || return
  fi
}

managed_validate_uninstall_resource() {
  local resource="$1"
  local current_checksum

  if ! managed_read_state "$resource"; then
    managed_state_exists "$resource" || return 0
    cli_error "Managed resource state is malformed: $(managed_state_path "$resource")"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  case "$MANAGED_STATE_TYPE" in
    block)
      if [[ ! -f "$MANAGED_STATE_TARGET" || -L "$MANAGED_STATE_TARGET" ]]; then
        cli_error "Managed block path changed type; preserving it: $MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      managed_inspect_block "$resource" "$MANAGED_STATE_TARGET" || return
      if [[ "$MANAGED_BLOCK_STATUS" != intact || "$MANAGED_BLOCK_CHECKSUM" != "$MANAGED_STATE_CHECKSUM" ]]; then
        managed_block_error "$resource" "$MANAGED_STATE_TARGET"
        return "$SELFISHELL_EXIT_ERROR"
      fi
      ;;
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
