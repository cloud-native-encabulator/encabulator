#!/usr/bin/env bash
# Set up a fire drill: a stand-in cluster in one canned incident state, for an agent session.
#
#   fixtures/fire-drill/run.sh <case>            prepare a drill directory and print the prompt
#   fixtures/fire-drill/run.sh <case> --blind    the same, plus a copy of the repository without
#                                                the answer keys, to start the agent in
#   fixtures/fire-drill/run.sh --grade <dir>     after the drill: the calls it made, what the
#                                                stand-in refused, and the answer key
#   fixtures/fire-drill/run.sh --list            the cases
#
# The drill directory (under $TMPDIR) holds bin/kubectl and bin/aws (the stand-in, stub.sh),
# an empty kubeconfig and env.sh. In a new shell: `source <dir>/env.sh`, start the agent there
# (in <dir>/repo with --blind), give it the printed prompt, then grade. Nothing reaches a real
# cluster or AWS account: kubectl and aws are the stand-in, and every AWS_ variable is unset.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../../../.." && pwd)"

cases() { for d in "$here"/*/; do [[ -f "$d/world.txt" ]] && basename "$d"; done; }
section() { awk -v want="## $1" '$0 == want { on = 1; next } /^## / { on = 0 } on' "$2"; }

case "${1:-}" in
  ""|-h|--help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  --list) cases; exit 0 ;;
  --grade)
    dir="${2:?usage: run.sh --grade <drill dir>}"
    name="$(cat "$dir/case")"
    echo "== calls the agent made (kubectl and aws)"
    sed -E 's/(jsonpath=)[^ ]+/\1.../' "$dir/calls.log" | cut -c1-140
    echo
    echo "== refused (a change, or Secret contents, attempted without the presenter's yes)"
    grep 'REFUSED' "$dir/calls.log" || echo "none"
    echo
    echo "== answer key: $name"
    for part in "Expected diagnosis" "Safe fix" "Must not"; do
      echo "-- $part"
      section "$part" "$here/$name/drill.md"
    done
    exit 0 ;;
esac

name="$1"
[[ -f "$here/$name/world.txt" ]] || { echo "no such case: $name (run.sh --list)" >&2; exit 64; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/fire-drill-$name.XXXXXX")"
src="$here"
if [[ "${2:-}" == "--blind" ]]; then
  mkdir -p "$dir/repo"
  git -C "$repo" archive HEAD | tar -x -C "$dir/repo"
  # The answer keys stay out of the agent's reach. The stand-in answers from the copy's worlds.
  find "$dir/repo" -path '*/fixtures/fire-drill/*/drill.md' -exec rm -f {} +
  src="$dir/repo/.agents/skills/encabulator-ops/fixtures/fire-drill"
fi
mkdir -p "$dir/bin"
for tool in kubectl aws; do
  printf '#!/usr/bin/env bash\nSTUB_TOOL=%s exec "%s" "$@"\n' "$tool" "$src/stub.sh" >"$dir/bin/$tool"
  chmod 755 "$dir/bin/$tool"
done
: >"$dir/kubeconfig"; : >"$dir/calls.log"; echo "$name" >"$dir/case"
cat >"$dir/env.sh" <<EOF
# source this in the shell the agent runs in
for v in \$(env | sed -n 's/^\(AWS_[A-Z_]*\)=.*/\1/p'); do unset "\$v"; done
export PATH="$dir/bin:\$PATH" KUBECONFIG="$dir/kubeconfig"
export DRILL_WORLD="$src/$name/world.txt" DRILL_LOG="$dir/calls.log"
EOF

echo "Fire drill \"$name\" is ready in $dir"
echo
echo "1. In a new shell:   source $dir/env.sh"
[[ -d "$dir/repo" ]] && echo "                     cd $dir/repo"
echo "2. Start the agent there (Claude Code, Codex or Kiro CLI) and give it this prompt:"
echo
section "Presenter says" "$here/$name/drill.md" | sed 's/^/   /'
echo
echo "3. Grade:            $0 --grade $dir"
