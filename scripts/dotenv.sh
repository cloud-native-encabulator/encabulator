#!/usr/bin/env bash
# Read and write the gitignored .env file the AWS path uses (copy .env.example to start).
#
#   scripts/dotenv.sh get FILE KEY          print KEY's value; nothing when unset or empty
#   scripts/dotenv.sh set FILE KEY VALUE    set KEY in place, keeping FILE.bak (idempotent)
#
# The format is the usual one: KEY=value lines, optional `export `, blank lines and lines
# starting with # ignored, the last assignment wins. A value may be wrapped in "double" or
# 'single' quotes and may contain `=`; quotes are removed, nothing inside them is expanded.
# An unquoted value ends at ` #` (an inline comment). Values with `$` or a backtick are
# refused: nothing in this repository needs one, and make and the shell would expand them.
#
# Never prints a whole file, only the one value asked for.
set -euo pipefail

die() { echo "dotenv: $*" >&2; exit 2; }

# value_of FILE KEY: the parsed value of KEY's last assignment, or nothing.
value_of() {
  local file="$1" want="$2" line value found="" lineno=0 at=0
  local assign='^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$'
  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))
    line="${line%$'\r'}"
    [[ "$line" =~ $assign ]] || continue
    [[ "${BASH_REMATCH[2]}" == "$want" ]] || continue
    found="${BASH_REMATCH[3]}"
    at=$lineno
  done <"$file"
  [[ $at -gt 0 ]] || return 0

  # Trim leading whitespace, then unquote or strip an inline comment.
  value="${found#"${found%%[![:space:]]*}"}"
  case "$value" in
    \"*|\'*)
      local q="${value:0:1}" rest="${value:1}"
      [[ "$rest" == *"$q"* ]] || die "$file line $at: $want has an opening $q but no closing one"
      value="${rest%%"$q"*}"
      ;;
    *)
      value="${value%%[[:space:]]#*}"
      value="${value%"${value##*[![:space:]]}"}"
      ;;
  esac
  case "$value" in
    *'$'*|*'`'*) die "$file line $at: $want contains \$ or a backtick, which is not supported" ;;
  esac
  printf '%s' "$value"
}

cmd="${1:-}"
case "$cmd" in
  get)
    [[ $# -eq 3 ]] || die "usage: dotenv.sh get FILE KEY"
    [[ -f "$2" ]] || exit 0
    value_of "$2" "$3"
    ;;
  set)
    [[ $# -eq 4 ]] || die "usage: dotenv.sh set FILE KEY VALUE"
    file="$2" key="$3" value="$4"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || die "not a variable name: $key"
    [[ "$value" =~ ^[A-Za-z0-9._:/@+-]*$ ]] || die "set only writes plain values (letters, digits and ._:/@+-)"
    [[ -f "$file" ]] || die "$file does not exist (copy .env.example to $file first)"
    current="$(value_of "$file" "$key")"
    [[ "$current" == "$value" ]] && exit 0

    # Replace every active assignment of KEY (so the last one, the one that counts, is the new
    # value), or append one. Comments, other keys and the file's mode stay as they are.
    tmp="$(mktemp "${file}.XXXXXX")"
    trap 'rm -f "$tmp"' EXIT
    awk -v key="$key" -v value="$value" '
      BEGIN { pattern = "^[[:space:]]*(export[[:space:]]+)?" key "[[:space:]]*=" }
      $0 ~ pattern { print key "=" value; done = 1; next }
      { print }
      END { if (!done) print key "=" value }
    ' "$file" >"$tmp"
    cp -p "$file" "$file.bak"
    cat "$tmp" >"$file"
    ;;
  *)
    die "usage: dotenv.sh {get FILE KEY|set FILE KEY VALUE}"
    ;;
esac
