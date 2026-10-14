#!/usr/bin/env bash
# `make hub` asks before it creates anything billable.
#
#   CONFIRM=yes scripts/confirm.sh    answer yes without asking (CI, scripts)
#   scripts/confirm.sh                ask on the terminal; refuse when there is none
set -euo pipefail

prompt="This creates billable AWS resources (about 25 minutes). Continue? [y/N] "

case "${CONFIRM:-}" in
  yes)
    echo "CONFIRM=yes: continuing without asking."
    exit 0
    ;;
  "") ;;
  *)
    echo "CONFIRM must be empty or yes, not '${CONFIRM}'. This run changed nothing." >&2
    exit 1
    ;;
esac

if [[ ! -t 0 ]]; then
  echo "make hub needs a yes and there is no terminal to ask on." >&2
  echo "Run it in a terminal, or run: make hub CONFIRM=yes" >&2
  exit 1
fi

read -r -p "$prompt" answer || answer=""
case "$answer" in
  y|Y|yes|YES|Yes) exit 0 ;;
  *)
    echo "Stopped. This run changed nothing."
    exit 1
    ;;
esac
