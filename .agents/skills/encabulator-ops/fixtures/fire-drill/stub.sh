#!/usr/bin/env bash
# Stand-in for kubectl and aws, answering from a canned incident: fixtures/fire-drill/<case>/world.txt.
# The health check tests and the fire drills put it on PATH as `kubectl` and `aws`
# (STUB_TOOL names which one it plays; run.sh and ../../scripts/ops-health_test.sh set it).
#
#   DRILL_WORLD   the world.txt to answer from (required)
#   DRILL_LOG     a file every call is appended to (default: none)
#
# world.txt is made of sections: a line `## <name>`, then the output, with `|` standing for a
# tab. `## base <case>` takes every section the world does not have from that case's world.
# A section `## <name> (error)` makes that call print its text on stderr and fail.
# `## readyz (error)` makes every kubectl call that needs the API server fail with it, and
# `## aws (error)` every aws call. The names: context, cluster, readyz, api-resources,
# applications, rgds, kro, ack, ready, nodes, workloads, pods, events, `logs <pod>`,
# `describe <name>`, `aws cluster`, `aws capabilities`, `aws capability <name>`.
#
# It is read-only like a stage should be: a call that would change anything is refused and
# logged as REFUSED, so a drill shows whether the agent asked before changing the cluster.
set -uo pipefail

world="${DRILL_WORLD:?set DRILL_WORLD to fixtures/fire-drill/<case>/world.txt}"
log="${DRILL_LOG:-/dev/null}"
tool="${STUB_TOOL:-$(basename "$0")}"
# `## base <case>` on a line of its own: a section this world lacks comes from that case.
base="$(awk '/^## base / { print $3; exit }' "$world")"
base_world=""
[[ -n "$base" ]] && base_world="$(dirname "$world")/../$base/world.txt"

in_world() { grep -qxF "## $1" "$2"; }
has() { in_world "$1" "$world" || { [[ -n "$base_world" ]] && in_world "$1" "$base_world"; }; }
section() {
  local from="$world"
  in_world "$1" "$world" || from="$base_world"
  awk -v want="## $1" '$0 == want { on = 1; next } /^## / { on = 0 } on' "$from" | tr '|' '\t'
}
emit() {
  if has "$1 (error)"; then section "$1 (error)" >&2; exit 1; fi
  section "$1"; exit 0
}
refuse() {
  printf '%s REFUSED %s\n' "$tool" "$*" >>"$log"
  echo "fire drill: refused. This stand-in cluster is read-only, and on stage a change waits for the presenter's yes: $tool $*" >&2
  exit 1
}

printf '%s %s\n' "$tool" "$*" >>"$log"

# --------------------------------------------------------------------------- aws
if [[ "$tool" == "aws" ]]; then
  if has "aws (error)"; then section "aws (error)" >&2; exit 255; fi
  service="${1:-}" op="${2:-}"
  case "$op" in
    describe-*|list-*|get-*|wait) ;;
    *) refuse "$@" ;;
  esac
  name="" prev=""
  for arg in "$@"; do [[ "$prev" == "--capability-name" ]] && name="$arg"; prev="$arg"; done
  case "$service $op" in
    "eks describe-cluster") emit "aws cluster" ;;
    "eks list-capabilities") emit "aws capabilities" ;;
    "eks describe-capability") emit "aws capability $name" ;;
    *) echo "fire drill: no canned output for aws $service $op (a real account would answer)"; exit 0 ;;
  esac
fi

# ----------------------------------------------------------------------- kubectl
# The positional arguments, without the global flags and their values.
pos=() raw=false output=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --context|--namespace|-n|--kubeconfig|-o|--output|-l|--selector|--field-selector|-c|--container|--tail|--since|--sort-by)
      [[ "$1" == "-o" || "$1" == "--output" ]] && output=true; shift 2 ;;
    --raw) raw=true; shift 2 ;;
    -o=*|--output=*) output=true; shift ;;
    -*) shift ;;
    *) pos+=("$1"); shift ;;
  esac
done
verb="${pos[0]:-}"
what="${pos[1]:-}"

case "$verb" in
  get|describe|logs|top|api-resources|api-versions|version|explain|auth|cluster-info|events|config) ;;
  *) refuse "${pos[@]:-}" ;;
esac

if [[ "$verb" == "config" ]]; then
  case "$what" in
    current-context|get-contexts) emit context ;;
    view) emit cluster ;;
    *) refuse "${pos[@]}" ;;
  esac
fi
if [[ "$verb" == "version" ]]; then echo "Client Version: v1.37.0 (fire drill stand-in)"; exit 0; fi
if has "readyz (error)"; then section "readyz (error)" >&2; exit 1; fi

case "$verb" in
  api-resources) emit api-resources ;;
  logs)
    pod="${what#pod/}"; pod="${pod#pods/}"
    if has "logs $pod"; then emit "logs $pod"; fi
    echo "fire drill: no canned log for $what"; exit 0 ;;
  describe)
    name="${pos[${#pos[@]}-1]}"; name="${name##*/}"
    if has "describe $name"; then emit "describe $name"; fi
    echo "fire drill: no canned describe for ${pos[*]:1}"; exit 0 ;;
  get) ;;
  *) echo "fire drill: no canned output for kubectl ${pos[*]}"; exit 0 ;;
esac

$raw && emit readyz
key=""
case "$what" in
  secret*|*,secret*)
    # Names only: the contents of a Secret never go on the screen.
    if $output; then
      echo "fire drill: refused. Secret contents are not shown on stage (make credentials, off screen)" >&2
      printf '%s REFUSED secret contents %s\n' "$tool" "${pos[*]}" >>"$log"; exit 1
    fi
    key=secrets ;;
  *applications*) key=applications ;;
  *resourcegraphdefinitions*|rgd*) key=rgds ;;
  *.kro.run*|encabulator*) key=kro ;;
  *services.k8s.aws*|dbinstances*|serverlesscache*|dbsubnetgroups*) key=ack ;;
  *nodepools*|*nodeclasses*|*scaledobjects*) key=ready ;;
  nodes|node|no) key=nodes ;;
  deploy*|statefulset*|sts*) key=workloads ;;
  pods|pod|po) key=pods ;;
  events|event|ev) key=events ;;
esac
if [[ -z "$key" ]]; then echo "No resources found (fire drill: no canned output for kubectl get $what)"; exit 0; fi
# A plain `kubectl get` from a person or an agent: say what the columns are.
$output || echo "# fire drill stand-in: the $key the health check reads, tab separated"
emit "$key"
