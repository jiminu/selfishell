#!/usr/bin/env bash

case "$SELFISHELL_TEST_CKSUM_CASE" in
  failed)
    printf '123 5\n'
    exit 9
    ;;
  malformed) printf 'not-a-checksum\n' ;;
  overflow) printf '4294967296 5\n' ;;
  *) exit 2 ;;
esac
