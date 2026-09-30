#!/usr/bin/env bash
# Read scripts/services.txt and print it in whatever shape the caller needs.
#
#   services.sh names                 -> one service name per line
#   services.sh dirs                  -> one service directory per line
#   services.sh pairs                 -> "name<space>dir" per line
#   services.sh names <lang>          -> names for one language (go|node|python|rust)
#   services.sh dirs <lang>           -> directories for one language
#   services.sh dir <name>            -> the directory of one service
#   services.sh lang <name>           -> the language of one service
#   services.sh matrix                 -> GitHub Actions JSON matrix include list
#   services.sh matrix <lang>          -> same, filtered by language
#
# Everything else in the repository derives its service list from here.
set -euo pipefail

registry="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/services.txt"

# rows: strip comments and blank lines, emit "name dir lang plane"
rows() {
  awk -F'\t' '!/^[[:space:]]*#/ && NF >= 4 { print $1, $2, $3, $4 }' "$registry"
}

filter_lang() {
  local want="${1:-}"
  if [[ -z "$want" ]]; then cat; else awk -v l="$want" '$3 == l'; fi
}

json_matrix() {
  local first=1
  printf '['
  while read -r name dir lang plane; do
    [[ $first -eq 1 ]] || printf ','
    first=0
    printf '{"name":"%s","dir":"%s","lang":"%s","plane":"%s"}' "$name" "$dir" "$lang" "$plane"
  done
  printf ']\n'
}

cmd="${1:-names}"
arg="${2:-}"

case "$cmd" in
  names)  rows | filter_lang "$arg" | awk '{print $1}' ;;
  dirs)   rows | filter_lang "$arg" | awk '{print $2}' ;;
  langs)  rows | awk '{print $3}' | sort -u ;;
  planes) rows | awk '{print $4}' | sort -u ;;
  pairs)  rows | filter_lang "$arg" | awk '{print $1, $2}' ;;
  dir)    rows | awk -v n="$arg" '$1 == n {print $2}' ;;
  lang)   rows | awk -v n="$arg" '$1 == n {print $3}' ;;
  plane)  rows | awk -v n="$arg" '$1 == n {print $4}' ;;
  matrix) rows | filter_lang "$arg" | json_matrix ;;
  count)  rows | wc -l | tr -d ' ' ;;
  *)
    echo "usage: $(basename "$0") {names|dirs|langs|planes|pairs|dir|lang|plane|matrix|count} [arg]" >&2
    exit 2
    ;;
esac
