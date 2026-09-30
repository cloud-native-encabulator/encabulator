#!/usr/bin/env bash
# Run one make target in every service and library directory that implements it.
#
#   scripts/iterate.sh build
#   scripts/iterate.sh test
#   scripts/iterate.sh lint
#
# Directories without a Makefile, and Makefiles without the requested target, are reported
# and skipped rather than failing the run: this repository is written by several people at
# once, and a half-finished service should not block everyone else. A target that exists and
# fails is a real failure.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root" || exit 1

target="${1:-build}"
: "${MAKE:=make}"

bold=$'\033[1m'; dim=$'\033[2m'; red=$'\033[31m'; off=$'\033[0m'

mapfile -t dirs < <("$repo_root/scripts/services.sh" dirs)
# Every library that has its own Makefile: libs/go/kit, libs/ts/kit, libs/python/encabulator-kit.
while IFS= read -r d; do dirs+=("$d"); done < <(find libs -mindepth 2 -maxdepth 2 -type d 2>/dev/null | sort)

failed=(); skipped=()
for dir in "${dirs[@]}"; do
  if [ ! -f "$dir/Makefile" ]; then
    skipped+=("$dir (no Makefile yet)")
    continue
  fi
  # Match `target:` at the start of a line, allowing `target: deps`.
  if ! grep -qE "^${target}[[:space:]]*:" "$dir/Makefile"; then
    skipped+=("$dir (no '${target}' target)")
    continue
  fi
  printf '%s==> %s: make %s%s\n' "$bold" "$dir" "$target" "$off"
  if ! $MAKE -C "$dir" "$target"; then
    failed+=("$dir")
  fi
done

if [ ${#skipped[@]} -gt 0 ]; then
  printf '\n%sskipped:%s\n' "$dim" "$off"
  for s in "${skipped[@]}"; do printf '  %s\n' "$s"; done
fi

if [ ${#failed[@]} -gt 0 ]; then
  printf '\n%sfailed: %s%s\n' "$red" "${failed[*]}" "$off"
  exit 1
fi

printf '\n%s==> %s: ok (%d of %d directories)%s\n' \
  "$bold" "$target" "$(( ${#dirs[@]} - ${#skipped[@]} ))" "${#dirs[@]}" "$off"
