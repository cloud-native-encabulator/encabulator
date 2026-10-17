#!/usr/bin/env bash
# One screen, read-only health check of an Encabulator cluster: the hub or a cell.
# Part of the encabulator-ops agent skill (../SKILL.md).
#
#   ops-health.sh                    the current kubectl context
#   ops-health.sh --context use2     another kubectl context (a cell, for example)
#   ops-health.sh --explain          one line per finding saying what it means
#   ops-health.sh --no-aws           kubectl only, even when the AWS CLI is installed
#   ops-health.sh --namespace NS     where the app runs (default encabulator)
#
# Read-only: it runs `kubectl config`, `kubectl api-resources` and `kubectl get` (never on
# Secrets), and with the AWS CLI only `aws eks describe-*` and `aws eks list-*`. Nothing it
# prints carries an account id: every 12-digit number is shown as 111122223333.
#
# KUBECTL and AWS name the commands it runs (a wrapper, or stand-ins in the tests), for example
# KUBECTL="kubectl --kubeconfig ./hub.kubeconfig". Needs bash 3.2 or newer, awk and sed.
#
# Exit status: 0 nothing failed (warnings may be listed), 1 at least one FAIL, 2 the cluster is
# unreachable, 64 bad usage.
set -uo pipefail

KUBECTL="${KUBECTL:-kubectl}"
AWS="${AWS:-aws}"
read -r -a kubectl_cmd <<<"$KUBECTL"
read -r -a aws_cmd <<<"$AWS"

context="" explain=false use_aws=true namespace="encabulator"
usage() { sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; }
while [[ $# -gt 0 ]]; do
  case "$1" in
    --context) [[ $# -ge 2 ]] || { usage >&2; exit 64; }; context="$2"; shift 2 ;;
    --namespace) [[ $# -ge 2 ]] || { usage >&2; exit 64; }; namespace="$2"; shift 2 ;;
    --explain) explain=true; shift ;;
    --no-aws) use_aws=false; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ops-health: unknown argument: $1" >&2; usage >&2; exit 64 ;;
  esac
done

work="$(mktemp -d "${TMPDIR:-/tmp}/ops-health.XXXXXX")" || exit 1
trap 'rm -rf "$work"' EXIT
findings="$work/findings"
: >"$findings"

# Every call gets a deadline, so a stalled API server cannot hold the screen.
deadline=()
command -v timeout >/dev/null 2>&1 && deadline=(timeout 20)
ctx_args=()
[[ -n "$context" ]] && ctx_args=(--context "$context")

# ${a[@]+"${a[@]}"}: an empty array under set -u, which bash before 4.4 calls unbound.
k() { ${deadline[@]+"${deadline[@]}"} "${kubectl_cmd[@]}" ${ctx_args[@]+"${ctx_args[@]}"} "$@"; }
kget() { k get --request-timeout=10s "$@"; }
a() { ${deadline[@]+"${deadline[@]}"} "${aws_cmd[@]}" "$@"; }

# add LEVEL AREA RULE TEXT: one finding. LEVEL is OK, INFO, WARN or FAIL.
add() { printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >>"$findings"; }
first_line() { tr -d '\r' | sed -n '/./{p;q;}' | cut -c1-160; }

# ------------------------------------------------------------------ jsonpath templates
# One line per object, tab separated. The tests render each template over real-shaped
# objects with kubectl itself (tests/jsonpath_test.sh), so keep the field order in step with
# the awk below and with fixtures/fire-drill/*/world.txt.
APPS_ITEM='{.metadata.name}{"\t"}{.status.sync.status}{"\t"}{.status.health.status}{"\t"}{.status.operationState.phase}{"\t"}{.status.operationState.message}{"\n"}'
RGDS_ITEM='{.metadata.name}{"\t"}{.status.state}{"\t"}{.status.conditions[?(@.status=="False")].message}{"\n"}'
KRO_ITEM='{.kind}{"\t"}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.status.state}{"\t"}{.status.clusterStatus}{"\t"}{.status.conditions[?(@.status=="False")].message}{"\n"}'
ACK_ITEM='{.kind}{"\t"}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.status.conditions[?(@.type=="ACK.ResourceSynced")].status}{"\t"}{.status.conditions[?(@.type=="ACK.Terminal")].message}{"\t"}{.status.conditions[?(@.type=="ACK.Recoverable")].message}{"\t"}{.status.conditions[?(@.type=="ACK.ResourceSynced")].message}{"\n"}'
READY_ITEM='{.kind}{"\t"}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\t"}{.status.conditions[?(@.type=="Ready")].message}{"\n"}'
NODES_ITEM='{.metadata.name}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}'
WORK_ITEM='{.kind}{"\t"}{.metadata.name}{"\t"}{.spec.replicas}{"\t"}{.status.readyReplicas}{"\n"}'
CONTAINER_FIELDS='{.name}{"\t"}{.ready}{"\t"}{.state.waiting.reason}{"\t"}{.restartCount}{"\t"}{.image}{"\t"}{.lastState.terminated.reason}{"\t"}{.state.waiting.message}{"\n"}'
PODS_ITEM='P{"\t"}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.status.phase}{"\t"}{.status.conditions[?(@.type=="PodScheduled")].message}{"\n"}{range .status.initContainerStatuses[*]}I{"\t"}'"$CONTAINER_FIELDS"'{end}{range .status.containerStatuses[*]}C{"\t"}'"$CONTAINER_FIELDS"'{end}'
EVENTS_ITEM='{.lastTimestamp}{"\t"}{.eventTime}{"\t"}{.metadata.namespace}{"\t"}{.involvedObject.kind}/{.involvedObject.name}{"\t"}{.reason}{"\t"}{.message}{"\n"}'
list() { printf '{range .items[*]}%s{end}' "$1"; }

# The kinds of this project, by full name: a bare `dbinstances` resolves to DocumentDB.
ACK_KINDS="dbinstances.rds.services.k8s.aws dbsubnetgroups.rds.services.k8s.aws
serverlesscaches.elasticache.services.k8s.aws serverlesscachesnapshots.elasticache.services.k8s.aws
roles.iam.services.k8s.aws vpcs.ec2.services.k8s.aws subnets.ec2.services.k8s.aws
routetables.ec2.services.k8s.aws natgateways.ec2.services.k8s.aws
internetgateways.ec2.services.k8s.aws elasticipaddresses.ec2.services.k8s.aws
securitygroups.ec2.services.k8s.aws clusters.eks.services.k8s.aws
capabilities.eks.services.k8s.aws accessentries.eks.services.k8s.aws"
KRO_KINDS="encabulatorcells.kro.run encabulators.kro.run encabulatorservices.kro.run"
READY_KINDS="nodepools.karpenter.sh nodeclasses.eks.amazonaws.com scaledobjects.keda.sh"
DEFAULT_IMAGE_PREFIX="ghcr.io/cloud-native-encabulator/encabulator/"

# present KINDS: the ones the API server serves, comma separated.
present() {
  local kind out=""
  for kind in $1; do
    grep -qxF "$kind" "$work/api-resources" && out="${out:+$out,}$kind"
  done
  printf '%s' "$out"
}

# ------------------------------------------------------------------------- explain
explain_rule() {
  case "$1" in
    cluster.unreachable) echo "kubectl cannot reach the API server: expired credentials, the wrong context or the network. The talk goes on: show the captured runs (references/stage-rules.md, If the cluster is unreachable)." ;;
    aws.unavailable) echo "The AWS CLI check did not run (often expired credentials). Everything else came from kubectl, so the rest of the screen stands." ;;
    aws.cluster) echo "EKS reports the cluster itself is not ACTIVE. CREATING is normal for about 12 minutes after a cell starts; anything else, read aws eks describe-cluster." ;;
    aws.capability) echo "An EKS capability (managed Argo CD, ACK or kro) is not ACTIVE, so its controller is not reconciling. Read its health with aws eks describe-capability." ;;
    argocd.retrying) echo "Argo CD is retrying a sync on its own. On a new cell the NodePool reads Degraded for about 10 seconds and the first sync retries; it heals in 3 to 5 minutes. Wait." ;;
    argocd.outofsync) echo "The cluster differs from Git. With automated sync and selfHeal, Argo CD puts it back within seconds; if it stays, its last sync failed: read the message." ;;
    argocd.degraded) echo "A resource of this Application is unhealthy (often a Deployment whose pods are not ready). Look at the pods and kro lines on this screen for the cause." ;;
    argocd.progressing) echo "Argo CD is waiting for resources to become healthy, for example pods starting. Normal right after a sync." ;;
    argocd.op-failed) echo "The last sync failed. The message names the object Git could not apply; fix it in Git with a pull request, not with kubectl edit." ;;
    kro.rgd-inactive) echo "kro rejected this ResourceGraphDefinition (a schema or CEL error), so no instance of its kind is reconciled. The message says why; fix the RGD in Git." ;;
    kro.waiting-hub) echo "The cell's graph waits for the hub's Argo CD cluster Secret, which make gitops-register writes. AWS keeps building; Argo CD deploys nothing to the cell until the hub is registered." ;;
    kro.in-progress) echo "kro is still creating or waiting for resources of this instance, in graph order. A new cell is IN_PROGRESS for about 20 minutes; the message names what it waits for." ;;
    kro.failed) echo "kro could not reconcile this instance. The message names the resource; read its ACK conditions (the ack lines) for the AWS error." ;;
    ack.pending) echo "ACK is waiting for AWS (a NAT gateway or a cluster still being created). Normal; it clears by itself." ;;
    ack.snapshot-cache-gone) echo "An adopted cache snapshot is read by its cache name, and the cache is gone, so ACK can never read or delete it. Delete the snapshot with aws elasticache delete-serverless-cache-snapshot, as make teardown does, or let it expire within a day. Teardown work, never on stage." ;;
    ack.terminal) echo "ACK hit an error it will not retry (ACK.Terminal), for example an invalid value. The spec in Git has to change." ;;
    ack.not-synced) echo "ACK has not brought this AWS resource in line with its spec yet. ACK.Recoverable carries the AWS error and ACK retries on its own." ;;
    ready.not-ready) echo "Not Ready yet. A NodePool reads Degraded for about 10 seconds while its NodeClass validates; a ScaledObject that is not Ready means KEDA cannot read its trigger." ;;
    nodes.none) echo "No nodes yet. EKS Auto Mode starts nodes when pods need them, about a minute after they are scheduled." ;;
    nodes.not-ready) echo "A node is not Ready: starting up, or being replaced by Auto Mode. Pods on it move to other nodes." ;;
    app.scaled-zero) echo "Scaled to zero replicas: a Malaise fault does this on purpose and reverts by itself when it expires (the console's Malaise page shows it)." ;;
    app.not-ready) echo "Fewer replicas are ready than wanted. The pods lines say why (image, crash, scheduling)." ;;
    pods.image-private) echo "The registry refused an anonymous pull: the GHCR package is private. Make it public, or mirror the images to ECR (make mirror-images). Check with scripts/mirror-images.sh --check." ;;
    pods.image-missing) echo "The tag is not in the registry: the version was not built or the tag moved. Pin a tag that exists (VERSION, Encabulator spec.imageTag)." ;;
    pods.image-prefix) echo "The image is not from ghcr.io/cloud-native-encabulator/encabulator or an ECR mirror: check REGISTRY in .env and the cell Secret's encabulator.io/registry." ;;
    pods.image-pull) echo "The node cannot pull this image: a private package, a missing tag or a wrong prefix. kubectl describe pod shows the registry's answer." ;;
    pods.crashloop) echo "The container starts and exits again and again. Its log names the missing dependency (NATS, PostgreSQL, Valkey, InfluxDB). Fix the dependency; the service recovers by itself." ;;
    pods.oomkilled) echo "The container was killed for using more memory than its limit. Raise the limit in Git, or find what grew." ;;
    pods.pending) echo "The pod has no node yet. Auto Mode starts one within a minute or two; if the message says insufficient capacity, read the full event." ;;
    pods.creating) echo "The pod is starting: pulling images or attaching a volume. Normal for a minute after a deploy." ;;
    pods.failed) echo "Pods that ended in phase Failed (evicted, for example). They stay listed and do no harm." ;;
    images.mixed) echo "Service pods run more than one image tag, so a rollout is under way or a tag moved (images.yml pushes new tags on every merge to main)." ;;
    events.capacity) echo "EC2 had no capacity for one instance type in one zone. The full message lists the zones that have it; Auto Mode tries other types (C, M, R) by itself." ;;
    events.warning) echo "Recent warning events, newest first. One-off warnings during startup are normal; a repeating one is the clue." ;;
    *) echo "See references/playbooks.md." ;;
  esac
}

# ------------------------------------------------------------------------- checks
check_cluster() {
  local ctx_name err
  if [[ -n "$context" ]]; then ctx_name="$context"; else ctx_name="$(k config current-context 2>/dev/null)"; fi
  [[ -n "$ctx_name" ]] || ctx_name="(none)"
  echo "$ctx_name" >"$work/context"
  if ! k get --raw /readyz --request-timeout=5s >"$work/readyz" 2>"$work/readyz.err"; then
    err="$(first_line <"$work/readyz.err")"
    add FAIL cluster cluster.unreachable "unreachable: ${err:-no answer within 5 seconds}"
    return 1
  fi
  add OK cluster ok "reachable (context $ctx_name)"
  k api-resources -o name --verbs=list --request-timeout=10s >"$work/api-resources" 2>/dev/null || : >"$work/api-resources"
  return 0
}

check_aws() {
  $use_aws || return 0
  command -v "${aws_cmd[0]}" >/dev/null 2>&1 || { add INFO aws ok "skipped (no AWS CLI)"; return 0; }
  local arn region name status caps cap cstatus out bad=""
  arn="$(k config view --minify -o 'jsonpath={.contexts[0].context.cluster}' 2>/dev/null)"
  if [[ ! "$arn" =~ ^arn:aws[a-z-]*:eks:([a-z0-9-]+):[0-9]+:cluster/(.+)$ ]]; then
    add INFO aws ok "skipped (the context is not an EKS cluster ARN)"
    return 0
  fi
  region="${BASH_REMATCH[1]}" name="${BASH_REMATCH[2]}"
  if ! status="$(a eks describe-cluster --region "$region" --name "$name" --query cluster.status --output text 2>"$work/aws.err")"; then
    add WARN aws aws.unavailable "aws eks describe-cluster failed: $(first_line <"$work/aws.err")"
    return 0
  fi
  [[ "$status" == "ACTIVE" ]] || add WARN aws aws.cluster "cluster $name is $status"
  caps="$(a eks list-capabilities --region "$region" --cluster-name "$name" --query 'capabilities[].capabilityName' --output text 2>/dev/null)" || caps=""
  out=""
  for cap in $caps; do
    [[ "$cap" == "None" ]] && continue
    cstatus="$(a eks describe-capability --region "$region" --cluster-name "$name" --capability-name "$cap" --query capability.status --output text 2>/dev/null)" || cstatus="unknown"
    out="${out:+$out, }$cap $cstatus"
    [[ "$cstatus" == "ACTIVE" ]] || bad="${bad:+$bad, }$cap $cstatus"
  done
  [[ -n "$bad" ]] && add FAIL aws aws.capability "capability not ACTIVE: $bad"
  [[ "$status" == "ACTIVE" && -z "$bad" ]] && add OK aws ok "cluster $name ACTIVE in $region; capabilities: ${out:-none}"
  return 0
}

check_argocd() {
  if ! grep -qxF applications.argoproj.io "$work/api-resources"; then
    add INFO argocd ok "no Argo CD on this cluster (a cell is synced from the hub's Argo CD)"
    return 0
  fi
  kget applications.argoproj.io -n argocd -o "jsonpath=$(list "$APPS_ITEM")" >"$work/apps" 2>/dev/null || : >"$work/apps"
  awk -F'\t' -v out="$findings" '
    NF < 3 { next }
    { n++; name=$1; sync=$2; health=$3; phase=$4; msg=substr($5, 1, 120) }
    phase == "Running" && msg ~ /[Rr]etrying/ { printf "WARN\targocd\targocd.retrying\t%s %s/%s, retrying: %s\n", name, sync, health, msg >> out; bad++; next }
    phase == "Failed" || phase == "Error"  { printf "FAIL\targocd\targocd.op-failed\t%s sync %s: %s\n", name, phase, msg >> out; bad++; next }
    health == "Degraded"                    { printf "FAIL\targocd\targocd.degraded\t%s is Degraded (%s)\n", name, sync >> out; bad++; next }
    sync != "Synced"                        { printf "WARN\targocd\targocd.outofsync\t%s is %s (%s)\n", name, (sync == "" ? "Unknown" : sync), health >> out; bad++; next }
    health == "Progressing" || health == "Missing" || health == "Unknown" || health == "" {
                                              printf "WARN\targocd\targocd.progressing\t%s is %s\n", name, (health == "" ? "Unknown" : health) >> out; bad++; next }
    END {
      if (n == 0) printf "INFO\targocd\tok\tno Applications in argocd yet (make gitops-bootstrap creates them)\n" >> out
      else if (bad == 0) printf "OK\targocd\tok\t%d Applications Synced and Healthy\n", n >> out
    }' "$work/apps"
}

check_kro() {
  local kinds
  if grep -qxF resourcegraphdefinitions.kro.run "$work/api-resources"; then
    kget resourcegraphdefinitions.kro.run -o "jsonpath=$(list "$RGDS_ITEM")" >"$work/rgds" 2>/dev/null || : >"$work/rgds"
  else
    : >"$work/rgds"
  fi
  kinds="$(present "$KRO_KINDS")"
  if [[ -n "$kinds" ]]; then
    kget "$kinds" -A -o "jsonpath=$(list "$KRO_ITEM")" >"$work/kro" 2>/dev/null || : >"$work/kro"
  else
    : >"$work/kro"
  fi
  awk -F'\t' -v out="$findings" '
    FILENAME ~ /rgds$/ && NF >= 2 {
      r++
      if ($2 != "Active") { printf "FAIL\tkro\tkro.rgd-inactive\tRGD %s is %s: %s\n", $1, ($2 == "" ? "not Active" : $2), substr($3, 1, 110) >> out; bad++ }
      next
    }
    FILENAME ~ /kro$/ && NF >= 4 {
      i++; kind=$1; ns=$2; name=$3; state=$4; cl=$5; msg=substr($6, 1, 110)
      what = kind " " ns "/" name (cl != "" ? " (cluster " cl ")" : "")
      if (state == "ACTIVE") next
      bad++
      if (msg ~ /external reference hub/) printf "WARN\tkro\tkro.waiting-hub\t%s %s: %s\n", what, state, msg >> out
      else if (state == "IN_PROGRESS" || state == "") printf "WARN\tkro\tkro.in-progress\t%s %s%s\n", what, (state == "" ? "no state yet" : state), (msg != "" ? ": " msg : "") >> out
      else printf "FAIL\tkro\tkro.failed\t%s %s%s\n", what, state, (msg != "" ? ": " msg : "") >> out
    }
    END {
      if (r + i == 0) printf "INFO\tkro\tok\tno kro ResourceGraphDefinitions or instances here\n" >> out
      else if (bad == 0) printf "OK\tkro\tok\tResourceGraphDefinitions Active: %d, instances ACTIVE: %d\n", r, i >> out
    }' "$work/rgds" "$work/kro"
}

check_ack() {
  local kinds
  kinds="$(present "$ACK_KINDS")"
  if [[ -z "$kinds" ]]; then
    add INFO ack ok "no ACK resources of this project here"
    return 0
  fi
  kget "$kinds" -A -o "jsonpath=$(list "$ACK_ITEM")" >"$work/ack" 2>/dev/null || : >"$work/ack"
  awk -F'\t' -v out="$findings" '
    NF < 4 { next }
    { n++; what=$1 " " $2 "/" $3; synced=$4; term=$5; rec=$6; smsg=$7 }
    synced == "True" { next }
    { bad++; m = (term != "" ? term : (rec != "" ? rec : smsg)); m = substr(m, 1, 120) }
    m ~ /ServerlessCacheNotFoundFault/ { printf "FAIL\tack\tack.snapshot-cache-gone\t%s: %s\n", what, m >> out; next }
    term != ""                         { printf "FAIL\tack\tack.terminal\t%s Terminal: %s\n", what, m >> out; next }
    m ~ /pending|is not available|being created|CREATING/ { printf "WARN\tack\tack.pending\t%s waiting for AWS: %s\n", what, m >> out; next }
                                       { printf "WARN\tack\tack.not-synced\t%s synced=%s%s\n", what, (synced == "" ? "Unknown" : synced), (m != "" ? ": " m : "") >> out }
    END {
      if (n == 0) printf "INFO\tack\tok\tno ACK resources of this project here\n" >> out
      else if (bad == 0) printf "OK\tack\tok\t%d ACK resources synced\n", n >> out
    }' "$work/ack"
}

check_ready() {
  local kinds
  kinds="$(present "$READY_KINDS")"
  [[ -n "$kinds" ]] || return 0
  kget "$kinds" -A -o "jsonpath=$(list "$READY_ITEM")" >"$work/ready" 2>/dev/null || : >"$work/ready"
  awk -F'\t' -v out="$findings" '
    NF >= 4 && $4 != "True" { printf "WARN\tnodes\tready.not-ready\t%s %s%s is not Ready%s\n", $1, ($2 != "" ? $2 "/" : ""), $3, ($5 != "" ? ": " substr($5, 1, 100) : "") >> out }
  ' "$work/ready"
}

check_nodes() {
  kget nodes -o "jsonpath=$(list "$NODES_ITEM")" >"$work/nodes" 2>/dev/null || : >"$work/nodes"
  awk -F'\t' -v out="$findings" '
    NF >= 2 { n++; if ($2 == "True") r++; else nr = nr (nr == "" ? "" : ", ") $1 }
    END {
      if (n == 0) printf "WARN\tnodes\tnodes.none\tno nodes yet\n" >> out
      else if (nr != "") printf "WARN\tnodes\tnodes.not-ready\t%d of %d nodes Ready; not Ready: %s\n", r, n, nr >> out
      else printf "OK\tnodes\tok\t%d of %d nodes Ready\n", r, n >> out
    }' "$work/nodes"
}

check_app() {
  kget deployments.apps,statefulsets.apps -n "$namespace" -o "jsonpath=$(list "$WORK_ITEM")" >"$work/workloads" 2>/dev/null || : >"$work/workloads"
  awk -F'\t' -v out="$findings" -v ns="$namespace" '
    NF < 3 { next }
    { n++; want = $3 + 0; ready = $4 + 0 }
    want == 0 { printf "WARN\tapp\tapp.scaled-zero\t%s %s scaled to 0 replicas\n", $1, $2 >> out; bad++; next }
    ready < want { printf "FAIL\tapp\tapp.not-ready\t%s %s %d of %d ready\n", $1, $2, ready, want >> out; bad++; next }
    { ok++ }
    END {
      if (n == 0) printf "INFO\tapp\tok\tno workloads in namespace %s\n", ns >> out
      else if (bad == 0) printf "OK\tapp\tok\t%d of %d workloads ready in %s\n", ok, n, ns >> out
    }' "$work/workloads"
}

check_pods() {
  kget pods -A -o "jsonpath=$(list "$PODS_ITEM")" >"$work/pods" 2>/dev/null || : >"$work/pods"
  awk -F'\t' -v out="$findings" -v prefix="$DEFAULT_IMAGE_PREFIX" -v images="$work/images" '
    function report(level, rule, text) { if (++shown <= 8) printf "%s\tpods\t%s\t%s\n", level, rule, text >> out; else hidden++ }
    $1 == "P" { pod = $2 "/" $3; phase = $4; sched = $5; seen++
                if (phase == "Succeeded") { pod = ""; next }
                if (phase == "Failed") { failed++; pod = ""; next }
                if (phase == "Pending" && sched != "") { report("WARN", "pods.pending", pod " Pending: " substr(sched, 1, 110)); bad++; pod = "" }
                next }
    ($1 == "C" || $1 == "I") && pod != "" {
      kind = $1; ready = $3; reason = $4; restarts = $5; image = $6; last = $7; msg = $8
      if (kind == "C" && image ~ /\/encabulator\//) print image >> images
      if (reason ~ /ImagePullBackOff|ErrImagePull|InvalidImageName/) {
        rule = "pods.image-pull"
        if (msg ~ /401|403|[Uu]nauthorized|denied|forbidden/) rule = "pods.image-private"
        else if (msg ~ /not found|manifest unknown|NotFound/) rule = "pods.image-missing"
        else if (image !~ ("^" prefix) && image !~ /\.dkr\.ecr\./) rule = "pods.image-prefix"
        report("FAIL", rule, pod " " reason " " image); bad++; pod = ""; next
      }
      if (reason == "CrashLoopBackOff") {
        report("FAIL", (last == "OOMKilled" ? "pods.oomkilled" : "pods.crashloop"), pod " container " $2 " CrashLoopBackOff, " restarts " restarts" (last != "" ? ", last exit " last : "")); bad++; pod = ""; next
      }
      if (kind == "C" && ready != "true" && phase == "Pending") { report("WARN", "pods.creating", pod " " (reason != "" ? reason : "starting")); bad++; pod = ""; next }
      if (kind == "C" && ready != "true") { report("WARN", "pods.creating", pod " container " $2 " not ready" (reason != "" ? " (" reason ")" : "")); bad++; pod = ""; next }
    }
    END {
      if (hidden > 0) printf "WARN\tpods\tpods.more\t%d more pods with problems (kubectl get pods -A)\n", hidden >> out
      if (failed > 0) printf "INFO\tpods\tpods.failed\t%d pods in phase Failed (evicted or ended)\n", failed >> out
      if (bad == 0) printf "OK\tpods\tok\tevery running pod is ready (%d pods)\n", seen >> out
    }' "$work/pods"
  : >>"$work/images"
}

check_images() {
  [[ -s "$work/images" ]] || return 0
  awk -v out="$findings" '
    { i = $0; tag = i; sub(/^.*:/, "", tag); if (tag == i || tag ~ /\//) tag = "(no tag)"
      p = i; sub(/:[^:\/]*$/, "", p); sub(/\/[^\/]*$/, "", p)
      key = p "/<service>:" tag; if (!(key in c)) order[++k] = key; c[key]++; tags[tag] = 1 }
    END {
      for (t in tags) nt++
      line = ""
      for (j = 1; j <= k; j++) line = line (line == "" ? "" : ", ") order[j] " (" c[order[j]] ")"
      printf "%s\timages\t%s\t%s\n", (nt > 1 ? "WARN" : "INFO"), (nt > 1 ? "images.mixed" : "ok"), line >> out
    }' "$work/images"
}

check_events() {
  kget events -A --field-selector type=Warning -o "jsonpath=$(list "$EVENTS_ITEM")" >"$work/events" 2>/dev/null || : >"$work/events"
  awk -F'\t' 'NF >= 5 { t = ($1 != "" ? $1 : $2); print t "\t" $3 "\t" $4 "\t" $5 "\t" $6 }' "$work/events" \
    | sort -r | awk -F'\t' -v out="$findings" '
      $5 ~ /InsufficientInstanceCapacity|insufficient capacity|InsufficientCapacity/ {
        if (cap++ == 0) { cobj = $3; creason = $4; cmsg = $5 }; next }
      { key = $3 "|" $4; if (key in seen) next; seen[key] = 1
        if (++n <= 5) printf "INFO\tevents\tevents.warning\t%s %s: %s\n", $3, $4, substr($5, 1, 120) >> out }
      END {
        # One line for every capacity event: the newest, with its full list of zones.
        if (cap > 0) printf "WARN\tevents\tevents.capacity\t%s %s (%d events, newest): %s\n", cobj, creason, cap, substr(cmsg, 1, 400) >> out
        if (NR == 0) printf "OK\tevents\tok\tno warning events\n" >> out
      }'
}

# ------------------------------------------------------------------------- report
mask() { sed -E 's/(^|[^0-9])[0-9]{12}([^0-9]|$)/\1111122223333\2/g; s/(^|[^0-9])[0-9]{12}([^0-9]|$)/\1111122223333\2/g'; }

# start_with: the finding to fix first. A store that is not ready comes before the services that
# crash because of it; then the first FAIL by area, causes before symptoms.
start_with() {
  local area line
  if grep -q $'\tpods.crashloop\t' "$findings"; then
    line="$( { grep -E $'^(FAIL|WARN)\tpods\t[a-z.-]+\t.*(nats|influxdb|postgres|valkey)' "$findings"
               grep -E $'^(FAIL|WARN)\tapp\t[a-z.-]+\t.*(nats|influxdb|postgres|valkey)' "$findings"; } \
             | grep -v $'\tpods.crashloop\t' | head -n 1)"
    if [[ -n "$line" ]]; then
      printf '%s (the crashing services depend on it)' "$(printf '%s' "$line" | cut -f4)"
      return
    fi
  fi
  for area in cluster aws ack kro pods argocd app nodes images events; do
    line="$(grep -E $'^FAIL\t'"$area"$'\t' "$findings" | head -n 1)"
    [[ -n "$line" ]] && { printf '%s: %s' "$area" "$(printf '%s' "$line" | cut -f4)"; return; }
  done
}

report() {
  local role="$1" level area rule text fails warns first=""
  printf 'encabulator-ops health  %s  context %s  %s  read-only\n' "$role" "$(cat "$work/context" 2>/dev/null)" "$(date '+%H:%M:%S')"
  while IFS=$'\t' read -r level area rule text; do
    printf '%-5s %-8s %s\n' "$level" "$area" "$text"
    if $explain && [[ "$level" == "WARN" || "$level" == "FAIL" ]]; then
      printf '      means: %s\n' "$(explain_rule "$rule")"
    fi
  done <"$findings"
  fails="$(grep -c '^FAIL' "$findings")"; warns="$(grep -c '^WARN' "$findings")"
  first="$(start_with)"
  if [[ "$fails" -eq 0 && "$warns" -eq 0 ]]; then
    echo "VERDICT healthy: nothing needs fixing"
  elif [[ "$fails" -eq 0 ]]; then
    echo "VERDICT no FAIL, $warns WARN: nothing is broken; the WARN lines say what is still moving or waiting"
  else
    echo "VERDICT $fails FAIL, $warns WARN: start with $(printf '%s' "$first" | cut -c1-100)"
  fi
  $explain || echo "(--explain says what each line means; references/playbooks.md has the fixes)"
}

main() {
  local role="cluster"
  if ! check_cluster; then
    report "cluster"
    return 2
  fi
  grep -qxF encabulatorcells.kro.run "$work/api-resources" && role="hub"
  [[ "$role" == "cluster" ]] && grep -qxF encabulators.kro.run "$work/api-resources" && role="cell"
  check_aws
  check_argocd
  check_kro
  check_ack
  check_nodes
  check_ready
  check_app
  check_pods
  check_images
  check_events
  report "$role"
  grep -q '^FAIL' "$findings" && return 1
  return 0
}

main | mask
exit "${PIPESTATUS[0]}"
