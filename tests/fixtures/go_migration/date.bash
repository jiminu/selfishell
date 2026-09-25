#!/usr/bin/env bash

# Fix only the clock used in backup names; retain every timestamp byte in captures.
if [[ "$*" == +%Y%m%d%H%M%S ]]; then
  printf '20000101000000\n'
else
  exec /bin/date "$@"
fi
