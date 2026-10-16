#!/usr/bin/env bash
# Tear the AWS path down in the order it needs (terraform/brownfield/README.md, Teardown):
# cells first, while the hub runs, then the hub.
#
#   scripts/teardown.sh remove-cell [<cell>]  remove one cell (make remove-cell CELL=<cell>)
#   scripts/teardown.sh teardown              remove every cell, then the hub (make teardown)
#   scripts/teardown.sh ecr-delete            delete the ECR repositories make mirror-images
#                                             created (make mirror-images-delete)
#
# Under make -n each command prints its steps and runs nothing. Every step looks at what is
# still there before it acts, so a run that stopped (a timeout, expired credentials, a ctrl-c)
# goes on where it stopped when it is started again.
#
# Settings (make passes them): HUB_REGION, HUB_CLUSTER, KUBECTL, TERRAFORM, TF_DIR, NAMESPACE,
# MAKE, CELL, CONFIRM (the name you would type, to answer without a terminal),
# TEARDOWN_STORES (ack, the default: through the hub's ACK; or cli: with the AWS CLI) and the
# TF_VAR_ values. Waits check every TEARDOWN_POLL seconds (20) and give up after
# TEARDOWN_TIMEOUT_<what> seconds (below), with a message that says to run the command again.
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
cells_dir="$repo_root/gitops/hub/cells"
templates="$repo_root/docs/teardown"
helper="$repo_root/scripts/teardown-helper.py"
guard="$repo_root/scripts/teardown-plan-guard.py"

: "${HUB_REGION:=us-west-2}"
: "${HUB_CLUSTER:=encabulator-usw2}"
: "${KUBECTL:=kubectl}"
: "${TERRAFORM:=terraform}"
: "${TF_DIR:=terraform/brownfield}"
: "${NAMESPACE:=encabulator}"
: "${MAKE:=make}"
: "${CONFIRM:=}"
: "${TEARDOWN_STORES:=ack}"
: "${TEARDOWN_POLL:=20}"
: "${TEARDOWN_SETTLE:=30}"
: "${TEARDOWN_TIMEOUT_SYNC:=600}"
: "${TEARDOWN_TIMEOUT_CAPABILITIES:=900}"
: "${TEARDOWN_TIMEOUT_VOLUMES:=900}"
: "${TEARDOWN_TIMEOUT_STORES:=2400}"
: "${TEARDOWN_TIMEOUT_CELL:=3600}"
: "${TEARDOWN_TIMEOUT_ARGOCD:=900}"
read -r -a kubectl <<<"$KUBECTL"
read -r -a terraform <<<"$TERRAFORM"
read -r -a make_cmd <<<"$MAKE"
cells_ns=encabulator-cells

bold=$'\033[1m'; red=$'\033[31m'; off=$'\033[0m'
[[ -t 1 ]] || { bold="" red="" off=""; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# --------------------------------------------------------------------------- output
et() { TZ=America/New_York date '+%-I:%M %p ET'; }
say() { printf '  %s\n' "$*"; }
# The steps of the command that runs, for the step lines and for what is left on a stop.
steps=() step_now=0 step_prefix="" resume=""
step() {
  step_now="$1"
  printf '\n%s[%s%s/%s] %s  %s%s\n' "$bold" "$step_prefix" "$1" "${#steps[@]}" "$(et)" "${steps[$1 - 1]}" "$off"
}
left() {
  local i
  [[ $step_now -gt 0 ]] || return 0
  printf 'Left: steps %s to %s:\n' "$step_now" "${#steps[@]}" >&2
  for ((i = step_now; i <= ${#steps[@]}; i++)); do printf '  %2d  %s\n' "$i" "${steps[$i - 1]}" >&2; done
}
stopped() {
  [[ $step_now -gt 0 ]] || return 0
  printf 'Stopped at step %s%s at %s. Fix what is named above, then run %s again: every step is safe to run again.\n' \
    "$step_prefix" "$step_now" "$(et)" "$resume" >&2
  left
}
# A message from inside $(...) is printed there, and the step it stopped is named once, by the
# script itself (here or in the ERR trap).
die() {
  printf '\n%s%s%s\n' "$red" "$*" "$off" >&2
  [[ $BASH_SUBSHELL -gt 0 ]] || stopped
  exit 1
}
trap '[[ $BASH_SUBSHELL -gt 0 ]] || stopped' ERR

# wait_for TIMEOUT WHAT CHECK [ARGS...]: run CHECK every TEARDOWN_POLL seconds until it succeeds.
wait_for() {
  local timeout="$1" what="$2" start=$SECONDS shown=-1 elapsed; shift 2
  until "$@"; do
    elapsed=$((SECONDS - start))
    if [[ $elapsed -ge $timeout ]]; then
      die "Still waiting for $what after $elapsed seconds."
    fi
    if [[ $((elapsed / 60)) -ne $shown ]]; then
      shown=$((elapsed / 60)); printf '  %s  waiting for %s\n' "$(et)" "$what"
    fi
    sleep "$TEARDOWN_POLL"
  done
}

# Under make -n, make runs this script only to print the steps (the recipe line starts with +).
make_dry_run() {
  local w
  for w in ${MAKEFLAGS:-}; do
    [[ "$w" == "--" ]] && break
    [[ "$w" == --* || "$w" == *=* ]] && continue
    [[ "${w#-}" == *n* ]] && return 0
  done
  return 1
}
print_plan() {
  local i
  printf '%s\n' "$1"
  for ((i = 1; i <= ${#steps[@]}; i++)); do printf '  %2d  %s\n' "$i" "${steps[$i - 1]}"; done
  printf '%s\n' "${2:-make -n: nothing was run.}"
}

# confirm WORD WHAT: the user types WORD (or passes CONFIRM=WORD) before anything is deleted.
confirm() {
  local word="$1" answer=""
  if [[ -n "$CONFIRM" ]]; then
    [[ "$CONFIRM" == "$word" ]] || die "CONFIRM is '$CONFIRM', not $word. Nothing was deleted."
    say "CONFIRM=$word: going on without asking."
    return 0
  fi
  [[ -t 0 ]] || die "This needs you to type $word, and there is no terminal to ask on. Run it in a terminal, or add CONFIRM=$word. Nothing was deleted."
  read -r -p "  Type $word to delete $2 (anything else stops): " answer || answer=""
  [[ "$answer" == "$word" ]] || die "Not confirmed. Nothing was deleted."
}

# ---------------------------------------------------------------------- AWS helpers
# aws_or_none ERROR-PATTERN ARGS...: print the output of `aws ARGS`; print nothing when it fails
# with ERROR-PATTERN (the thing does not exist); stop on any other error, so a broken call
# (expired credentials, access denied) is never read as "gone".
aws_or_none() {
  local pattern="$1" out; shift
  if out="$(aws "$@" 2>&1)"; then
    [[ "$out" == "None" ]] || printf '%s' "$out"
    return 0
  fi
  [[ "$out" =~ $pattern ]] && return 0
  die "aws $1 $2 failed: $(tail -n 1 <<<"$out")"
}
account_id() {
  local a
  a="$(aws sts get-caller-identity --query Account --output text 2>&1)" \
    || die "No working AWS credentials: $(tail -n 1 <<<"$a")"
  [[ "$a" =~ ^[0-9]{12}$ ]] || die "aws sts get-caller-identity returned no account id."
  printf '%s' "$a"
}
cluster_status() { aws_or_none ResourceNotFoundException eks describe-cluster --region "$1" --name "$2" --query cluster.status --output text; }
# stack_vpc REGION NAME: the VPC tagged Name=NAME and project=encabulator, or nothing.
stack_vpc() {
  aws_or_none '^$' ec2 describe-vpcs --region "$1" \
    --filters "Name=tag:Name,Values=$2" Name=tag:project,Values=encabulator --query 'Vpcs[].VpcId' --output text
}
# vpc_of_subnets REGION SUBNET...: the one VPC the subnets are in.
vpc_of_subnets() {
  local r="$1"; shift
  aws ec2 describe-subnets --region "$r" --subnet-ids "$@" --query 'Subnets[].VpcId' --output text | tr '\t' '\n' | sort -u
}

# ------------------------------------------------------------------ kubectl helpers
kc() { "${kubectl[@]}" --context "$HUB_CLUSTER" "$@"; }
# kget ARGS...: kubectl get on the hub that prints nothing for an object (or a kind) that is
# not there, and stops on any other error.
kget() {
  local out
  if out="$(kc "$@" --ignore-not-found 2>&1)"; then printf '%s' "$out"; return 0; fi
  [[ "$out" == *"doesn't have a resource type"* ]] && return 0
  die "kubectl $* failed: $(tail -n 1 <<<"$out")"
}
# exists NAMESPACE KIND NAME. A failed call stops the script, also where this runs as a
# condition (set -e does not apply there), so an error is never read as "not there".
exists() { local o; o="$(kget -n "$1" get "$2" "$3" -o name)" || { stopped; exit 1; }; [[ -n "$o" ]]; }
# none_left COMMAND...: true when the command prints nothing; a failed call stops the script.
none_left() { local o; o="$("$@")" || { stopped; exit 1; }; [[ -z "$o" ]]; }

# require_hub: kubectl's current context is the hub, and it points at the hub in this account.
require_hub() {
  local ctx server endpoint
  ctx="$("${kubectl[@]}" config current-context 2>/dev/null || true)"
  [[ "$ctx" == "$HUB_CLUSTER" ]] \
    || die "kubectl points at '${ctx:-nothing}', not the hub $HUB_CLUSTER. Run make kubeconfig first."
  endpoint="$(aws_or_none ResourceNotFoundException eks describe-cluster --region "$HUB_REGION" --name "$HUB_CLUSTER" --query cluster.endpoint --output text)"
  [[ -n "$endpoint" ]] || die "There is no EKS cluster $HUB_CLUSTER in $HUB_REGION in this account."
  server="$("${kubectl[@]}" config view --minify -o 'jsonpath={.clusters[0].cluster.server}')"
  [[ "$server" == "$endpoint" ]] \
    || die "The context $HUB_CLUSTER points at $server, not the $HUB_CLUSTER of this account. Run make kubeconfig first."
}

# adopt_delete NAMESPACE KIND NAME TEMPLATE TEMPLATE-KIND KEY=VALUE...: apply an adopt object
# (adoption-policy adopt, deletion-policy delete), wait until ACK has read the AWS resource,
# then delete the object, and ACK deletes the AWS resource. An object left by a run that
# stopped is reused; any other object of that name is not touched.
adopt_delete() {
  local ns="$1" kind="$2" name="$3" template="$4" tkind="$5" label; shift 5
  if exists "$ns" "$kind" "$name"; then
    label="$(kget -n "$ns" get "$kind" "$name" -o 'jsonpath={.metadata.labels.encabulator\.io/teardown}')"
    [[ "$label" == "true" ]] || die "$kind $ns/$name exists and was not made by the teardown: delete or rename it first."
  else
    python3 "$helper" render "$templates/$template" "$tkind" name="$name" namespace="$ns" "$@" >"$work/object.yaml"
    kc apply -f "$work/object.yaml" >/dev/null
  fi
  kc -n "$ns" wait --for=condition=ACK.ResourceSynced "$kind/$name" --timeout="${TEARDOWN_TIMEOUT_SYNC}s" >/dev/null \
    || die "ACK did not read $kind $name within ${TEARDOWN_TIMEOUT_SYNC}s. Its conditions: kubectl -n $ns get $kind $name -o yaml"
  kc -n "$ns" delete "$kind" "$name" --timeout="${TEARDOWN_TIMEOUT_SYNC}s" >/dev/null \
    || die "ACK did not delete $kind $name within ${TEARDOWN_TIMEOUT_SYNC}s. Its conditions: kubectl -n $ns get $kind $name -o yaml"
}

# tagged_project ARN-TAG-VALUE WHAT: the project tag value read from AWS must be encabulator.
tagged_project() { [[ "$1" == "encabulator" ]] || die "$2 does not carry the tag project=encabulator, so it is left alone. Check it and delete it by hand if it is yours."; }

# --------------------------------------------------------------------------- stores
# stores_delete REGION STACK NAMESPACE: the RDS instance encabulator-pg, the Valkey cache
# encabulator-valkey, the DB subnet group encabulator and the cache's automatic snapshots of one
# stack (the hub or a cell). Each must be in the VPC tagged Name=STACK and carry the project
# tag; anything else stops the run. ACK on the hub deletes them (adopt, then delete the
# object), or the AWS CLI with TEARDOWN_STORES=cli. No final snapshot is kept.
stores_delete() {
  local r="$1" stack="$2" ns="$3" vpc info arn status dvpc protect subnets sgs svpc
  [[ "$TEARDOWN_STORES" == ack || "$TEARDOWN_STORES" == cli ]] || die "TEARDOWN_STORES must be ack or cli, not $TEARDOWN_STORES."
  vpc="$(stack_vpc "$r" "$stack")"
  # store_in_vpc WHAT VPC: refuse a store that is not in this stack's VPC.
  store_in_vpc() {
    [[ -n "$vpc" && "$2" == "$vpc" ]] \
      || die "$1 in $r is in ${2:-no VPC}, not in the VPC of $stack (${vpc:-gone}), so it is left alone. Check it and delete it by hand if it is yours."
  }

  info="$(aws_or_none DBInstanceNotFound rds describe-db-instances --region "$r" --db-instance-identifier encabulator-pg \
    --query 'DBInstances[0].[DBInstanceArn,DBInstanceStatus,DBSubnetGroup.VpcId,DeletionProtection]' --output text)"
  if [[ -n "$info" ]]; then
    read -r arn status dvpc protect <<<"$info"
    store_in_vpc "RDS instance encabulator-pg" "$dvpc"
    [[ "$protect" == False ]] || die "RDS instance encabulator-pg in $r has deletion protection on. Turn it off yourself if you mean to delete it."
    tagged_project "$(aws rds list-tags-for-resource --region "$r" --resource-name "$arn" --query 'TagList[?Key==`project`].Value' --output text)" "RDS instance encabulator-pg in $r"
    if [[ "$status" == deleting ]]; then say "RDS instance encabulator-pg ($r) is already being deleted"
    elif [[ "$TEARDOWN_STORES" == ack ]]; then
      adopt_delete "$ns" dbinstances.rds.services.k8s.aws "$stack-teardown-pg" adopt-hub-stores.yaml DBInstance region="$r" id=encabulator-pg
      say "RDS instance encabulator-pg ($r): deleting, through the hub's ACK"
    else
      aws rds delete-db-instance --region "$r" --db-instance-identifier encabulator-pg \
        --skip-final-snapshot --delete-automated-backups >/dev/null
      say "RDS instance encabulator-pg ($r): deleting"
    fi
  else say "RDS instance encabulator-pg ($r): none"; fi

  info="$(aws_or_none ServerlessCacheNotFound elasticache describe-serverless-caches --region "$r" --serverless-cache-name encabulator-valkey \
    --query 'ServerlessCaches[0].[ARN,Status]' --output text)"
  if [[ -n "$info" ]]; then
    read -r arn status <<<"$info"
    subnets="$(aws elasticache describe-serverless-caches --region "$r" --serverless-cache-name encabulator-valkey --query 'ServerlessCaches[0].SubnetIds' --output text)"
    sgs="$(aws elasticache describe-serverless-caches --region "$r" --serverless-cache-name encabulator-valkey --query 'ServerlessCaches[0].SecurityGroupIds' --output text)"
    # shellcheck disable=SC2086 # one word per subnet id
    svpc="$(vpc_of_subnets "$r" $subnets)"
    store_in_vpc "Valkey cache encabulator-valkey" "$svpc"
    tagged_project "$(aws elasticache list-tags-for-resource --region "$r" --resource-name "$arn" --query 'TagList[?Key==`project`].Value' --output text)" "Valkey cache encabulator-valkey in $r"
    if [[ "$status" == deleting ]]; then say "Valkey cache encabulator-valkey ($r) is already being deleted"
    elif [[ "$TEARDOWN_STORES" == ack ]]; then
      adopt_delete "$ns" serverlesscaches.elasticache.services.k8s.aws "$stack-teardown-valkey" adopt-hub-stores.yaml ServerlessCache \
        region="$r" id=encabulator-valkey subnets="$(tr '\t ' ',,' <<<"$subnets")" sgs="$(tr '\t ' ',,' <<<"$sgs")"
      say "Valkey cache encabulator-valkey ($r): deleting, through the hub's ACK"
    else
      aws elasticache delete-serverless-cache --region "$r" --serverless-cache-name encabulator-valkey >/dev/null
      say "Valkey cache encabulator-valkey ($r): deleting"
    fi
  else say "Valkey cache encabulator-valkey ($r): none"; fi

  # RDS refuses to delete a subnet group an instance uses; a cache snapshot is taken from a
  # cache, so both wait until the stores are gone.
  wait_for "$TEARDOWN_TIMEOUT_STORES" "RDS encabulator-pg in $r to be deleted (about 7 minutes)" none_left \
    aws_or_none DBInstanceNotFound rds describe-db-instances --region "$r" --db-instance-identifier encabulator-pg --query 'DBInstances[0].DBInstanceStatus' --output text
  wait_for "$TEARDOWN_TIMEOUT_STORES" "Valkey encabulator-valkey in $r to be deleted" none_left \
    aws_or_none ServerlessCacheNotFound elasticache describe-serverless-caches --region "$r" --serverless-cache-name encabulator-valkey --query 'ServerlessCaches[0].Status' --output text

  info="$(aws_or_none DBSubnetGroupNotFound rds describe-db-subnet-groups --region "$r" --db-subnet-group-name encabulator \
    --query 'DBSubnetGroups[0].[DBSubnetGroupArn,VpcId]' --output text)"
  if [[ -n "$info" ]]; then
    read -r arn dvpc <<<"$info"
    store_in_vpc "DB subnet group encabulator" "$dvpc"
    tagged_project "$(aws rds list-tags-for-resource --region "$r" --resource-name "$arn" --query 'TagList[?Key==`project`].Value' --output text)" "DB subnet group encabulator in $r"
    if [[ "$TEARDOWN_STORES" == ack ]]; then
      subnets="$(aws rds describe-db-subnet-groups --region "$r" --db-subnet-group-name encabulator --query 'DBSubnetGroups[0].Subnets[].SubnetIdentifier' --output text)"
      adopt_delete "$ns" dbsubnetgroups.rds.services.k8s.aws "$stack-teardown-subnet-group" adopt-hub-subnet-group.yaml DBSubnetGroup \
        region="$r" id=encabulator subnets="$(tr '\t ' ',,' <<<"$subnets")"
    else
      aws rds delete-db-subnet-group --region "$r" --db-subnet-group-name encabulator
    fi
    say "DB subnet group encabulator ($r): deleted"
  else say "DB subnet group encabulator ($r): none"; fi

  snapshots_delete "$r" "$stack" "$ns"
}

# snapshots_delete REGION STACK NAMESPACE: the automatic snapshots of encabulator-valkey, which
# stay after the cache is deleted until they expire (one day).
snapshots_delete() {
  local r="$1" stack="$2" ns="$3" names n=0 snap sarn
  names="$(aws elasticache describe-serverless-cache-snapshots --region "$r" \
    --query 'ServerlessCacheSnapshots[?ServerlessCacheConfiguration.ServerlessCacheName==`encabulator-valkey`].[ServerlessCacheSnapshotName,ARN]' --output text)"
  while read -r snap sarn; do
    [[ -n "$snap" && "$snap" != None ]] || continue
    n=$((n + 1))
    tagged_project "$(aws elasticache list-tags-for-resource --region "$r" --resource-name "$sarn" --query 'TagList[?Key==`project`].Value' --output text)" "Valkey snapshot $snap in $r"
    if [[ "$TEARDOWN_STORES" == ack ]]; then
      adopt_delete "$ns" serverlesscachesnapshots.elasticache.services.k8s.aws "$stack-teardown-snapshot-$n" adopt-cache-snapshot.yaml \
        ServerlessCacheSnapshot region="$r" id="$snap"
    else
      aws elasticache delete-serverless-cache-snapshot --region "$r" --serverless-cache-snapshot-name "$snap" >/dev/null
    fi
    say "Valkey snapshot $snap ($r): deleted"
  done <<<"$names"
  [[ $n -gt 0 ]] || say "Valkey snapshots of encabulator-valkey ($r): none"
}

# ------------------------------------------------------------------------ one cell
cell_key="" cell_name="" cell_region="" cell_file="" in_teardown=false
cell_steps=(
  "check that kubectl points at the hub, and what the cell holds"
  "keep the cell's node role and node access entry (deletion-policy retain on the hub)"
  "delete the untracked private route tables of a cell built before the NAT fix"
  "stop Argo CD from re-creating cells: delete the Application hub-cells"
  "delete the EncabulatorCell: kro on the hub starts removing the cell"
  "wait until the cell's kro and ACK capabilities and its Argo CD cluster Secret are gone"
  "on the cell: delete the StatefulSets nats and influxdb, then their volumes"
  "delete the cell's RDS instance, Valkey cache, DB subnet group and cache snapshots"
  "wait until the EncabulatorCell is gone (about 20 minutes)"
  "delete the cell's retained node role"
)

# cell_resolve KEY: the cell's name and region from gitops/hub/cells/KEY.yaml, or, once that
# file has left Git, from its EncabulatorCell on the hub, or from CELL_REGION.
cell_resolve() {
  cell_key="$1"
  [[ "$cell_key" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || die "usage: make remove-cell CELL=<cell>, where gitops/hub/cells/<cell>.yaml names it (here: $(for f in "$cells_dir"/*.yaml; do [[ -e "$f" ]] && basename "$f" .yaml; done | paste -sd' ' -))"
  cell_file="$cells_dir/$cell_key.yaml"
  if [[ -f "$cell_file" ]]; then
    cell_name="$(awk '/^metadata:/ { m = 1; next } /^[^ #]/ { m = 0 } m && /^  name:/ { print $2; exit }' "$cell_file")"
    cell_region="$(awk '/^spec:/ { s = 1; next } /^[^ #]/ { s = 0 } s && /^  region:/ { print $2; exit }' "$cell_file")"
  else
    cell_name="encabulator-$cell_key"
    cell_region="${CELL_REGION:-}"
  fi
  [[ "$cell_name" =~ ^encabulator-[a-z0-9-]+$ ]] || die "The cell's name '$cell_name' does not start with encabulator-."
  [[ "$cell_name" != "$HUB_CLUSTER" ]] || die "$cell_name is the hub, not a cell."
}
cell_region_from_hub() {
  [[ -n "$cell_region" ]] && return 0
  cell_region="$(kget -n "$cells_ns" get encabulatorcells "$cell_name" -o 'jsonpath={.spec.region}')"
  [[ -n "$cell_region" ]] || die "No gitops/hub/cells/$cell_key.yaml and no EncabulatorCell $cell_name on the hub: run it again with CELL_REGION=<the cell's region>."
}

cell_check() {
  local state cluster
  require_hub
  cell_region_from_hub
  state="$(kget -n "$cells_ns" get encabulatorcells "$cell_name" -o 'jsonpath={.status.state}')"
  [[ -n "$state" ]] || { exists "$cells_ns" encabulatorcells "$cell_name" && state=present; }
  cluster="$(cluster_status "$cell_region" "$cell_name")"
  say "cell $cell_name in $cell_region: EncabulatorCell ${state:-gone}, EKS cluster ${cluster:-gone}"
  say "it deletes the cell's cluster, VPC, NAT gateway and IAM roles, and its stores with their data:"
  say "RDS encabulator-pg and Valkey encabulator-valkey in $cell_region (no final snapshot), the NATS and InfluxDB volumes."
  $in_teardown || confirm "$cell_name" "this cell"
}

cell_keep_node_role() {
  local kind
  for kind in roles.iam.services.k8s.aws accessentries.eks.services.k8s.aws; do
    if exists "$cells_ns" "$kind" "$cell_name-node"; then
      kc -n "$cells_ns" annotate --overwrite "$kind" "$cell_name-node" services.k8s.aws/deletion-policy=retain >/dev/null
      say "$kind $cell_name-node: deletion-policy retain"
    else
      say "$kind $cell_name-node: not on the hub, nothing to keep"
    fi
  done
}

cell_route_tables() {
  local vpc tracked ids id
  vpc="$(stack_vpc "$cell_region" "$cell_name")"
  [[ -n "$vpc" ]] || { say "no VPC $cell_name in $cell_region: nothing to check"; return 0; }
  tracked="$(kget -n "$cells_ns" get routetables.ec2.services.k8s.aws -o 'jsonpath={range .items[*]}{.status.routeTableID}{" "}{end}')"
  ids="$(aws ec2 describe-route-tables --region "$cell_region" \
    --filters "Name=vpc-id,Values=$vpc" "Name=tag:Name,Values=$cell_name-private" --output json \
    | python3 "$helper" leaked-route-tables "$cell_name-private" "$tracked")"
  [[ -n "$ids" ]] || { say "no untracked route table in $vpc"; return 0; }
  for id in $ids; do
    adopt_delete "$cells_ns" routetables.ec2.services.k8s.aws "$cell_name-teardown-$id" adopt-route-table.yaml RouteTable \
      region="$cell_region" stack="$cell_name" id="$id" rtb="$id" vpc="$vpc"
    say "untracked route table $id ($vpc): deleted"
  done
}

cell_stop_argocd() {
  local others
  if exists argocd applications.argoproj.io hub-cells; then
    kc -n argocd delete applications.argoproj.io hub-cells --wait=false >/dev/null
    say "Application hub-cells: deleted (no finalizer: the EncabulatorCells stay until the next step)"
  else
    say "Application hub-cells: already gone"
  fi
  if ! $in_teardown; then
    say "${bold}The cell file must leave Git too${off}, or Argo CD builds the cell again once hub-cells is back:"
    say "  git rm gitops/hub/cells/$cell_key.yaml && git commit -m \"Remove the $cell_key cell\" && git push"
    others="$(kget -n "$cells_ns" get encabulatorcells -o 'jsonpath={range .items[*]}{.metadata.name}{" "}{end}' | tr ' ' '\n' | grep -vx "$cell_name" | paste -sd' ' - || true)"
    if [[ -n "$others" ]]; then
      say "Other cells keep running ($others), without Argo CD watching them. Once the file is out of"
      say "Git, make add-cell CELL=<one of them> applies hub-cells again."
    fi
  fi
}

cell_delete_object() {
  local deleting
  if exists "$cells_ns" encabulatorcells "$cell_name"; then
    deleting="$(kget -n "$cells_ns" get encabulatorcells "$cell_name" -o 'jsonpath={.metadata.deletionTimestamp}')"
    if [[ -n "$deleting" ]]; then say "EncabulatorCell $cell_name: already being deleted"
    else
      kc -n "$cells_ns" delete encabulatorcells "$cell_name" --wait=false >/dev/null
      say "EncabulatorCell $cell_name: deleted; kro removes the cell in waves"
    fi
  else
    say "EncabulatorCell $cell_name: already gone"
  fi
}

cell_controllers_gone() {
  local caps
  caps="$(aws_or_none ResourceNotFoundException eks list-capabilities --region "$cell_region" --cluster-name "$cell_name" --query 'capabilities[].type' --output text)" || { stopped; exit 1; }
  caps=" $(tr '\t' ' ' <<<"$caps") "
  [[ "$caps" != *" KRO "* && "$caps" != *" ACK "* ]] || return 1
  none_left kget -n argocd get secret "$cell_name" -o name
}
cell_wait_controllers() {
  wait_for "$TEARDOWN_TIMEOUT_CAPABILITIES" "the kro and ACK capabilities of $cell_name and its Argo CD cluster Secret to go" cell_controllers_gone
  say "nothing re-creates the cell's StatefulSets or stores now"
}

cell_volumes() {
  local cfg="$work/cell.kubeconfig" status
  status="$(cluster_status "$cell_region" "$cell_name")"
  if [[ -z "$status" ]]; then
    say "the cluster $cell_name is gone: no volume to delete"; return 0
  fi
  aws eks update-kubeconfig --region "$cell_region" --name "$cell_name" --alias "$cell_name" --kubeconfig "$cfg" >/dev/null
  ck() { "${kubectl[@]}" --kubeconfig "$cfg" --context "$cell_name" "$@"; }
  can_delete() {
    [[ "$(ck auth can-i delete statefulsets -n encabulator 2>/dev/null)" == yes \
       && "$(ck auth can-i delete persistentvolumeclaims -n encabulator 2>/dev/null)" == yes \
       && "$(ck auth can-i list persistentvolumes 2>/dev/null)" == yes ]]
  }
  if ! can_delete; then
    [[ -f "$cell_file" ]] || die "Your AWS identity has no access to $cell_name, and make cell-access needs gitops/hub/cells/$cell_key.yaml: put the file back, or grant access another way."
    say "giving your AWS identity cluster admin on $cell_name (make cell-access CELL=$cell_key)"
    CELL="$cell_key" "$repo_root/scripts/gitops.sh" cell-access "$cell_key"
    wait_for 300 "the new access to $cell_name to work" can_delete
  fi
  ck -n encabulator delete statefulset nats influxdb --ignore-not-found >/dev/null
  ck -n encabulator delete pvc data-nats-0 data-influxdb-0 --ignore-not-found --wait=false >/dev/null
  wait_for "$TEARDOWN_TIMEOUT_VOLUMES" "the volumes of $cell_name to be deleted (kubectl get pv lists nothing)" none_left ck get pv -o name
  say "StatefulSets nats and influxdb and their volumes: deleted"
}

cell_stores() { stores_delete "$cell_region" "$cell_name" "$cells_ns"; }

cell_wait_gone() {
  cell_route_tables
  wait_for "$TEARDOWN_TIMEOUT_CELL" "the EncabulatorCell $cell_name to be gone (what is left: kubectl -n $cells_ns get managed -l kro.run/instance-name=$cell_name)" \
    none_left kget -n "$cells_ns" get encabulatorcells "$cell_name" -o name
  say "EncabulatorCell $cell_name: gone, with its cluster, VPC and NAT gateway"
}

cell_node_role() {
  local role="$cell_name-node" arn
  ! exists "$cells_ns" encabulatorcells "$cell_name" || die "The EncabulatorCell $cell_name is still on the hub."
  arn="$(aws_or_none NoSuchEntity iam get-role --role-name "$role" --query Role.Arn --output text)"
  if [[ -z "$arn" ]]; then say "IAM role $role: none"; return 0; fi
  tagged_project "$(aws iam list-role-tags --role-name "$role" --query 'Tags[?Key==`project`].Value' --output text)" "IAM role $role"
  [[ "$(cut -d: -f5 <<<"$arn")" == "$(account_id)" ]] || die "IAM role $role is not in the account of your credentials."
  adopt_delete "$cells_ns" roles.iam.services.k8s.aws "$cell_name-teardown-node" adopt-node-role.yaml Role stack="$cell_name" id="$role"
  wait_for "$TEARDOWN_TIMEOUT_SYNC" "the IAM role $role to be deleted" none_left \
    aws_or_none NoSuchEntity iam get-role --role-name "$role" --query Role.RoleName --output text
  say "IAM role $role: deleted, through the hub's ACK"
}

remove_cell() { # remove_cell KEY: every step for one cell
  local fns=(cell_check cell_keep_node_role cell_route_tables cell_stop_argocd cell_delete_object
             cell_wait_controllers cell_volumes cell_stores cell_wait_gone cell_node_role) i
  cell_resolve "$1"
  steps=("${cell_steps[@]}")
  for ((i = 1; i <= ${#fns[@]}; i++)); do step "$i"; "${fns[$i - 1]}"; done
  step_now=0
}

# ------------------------------------------------------------------------ ECR
ecr_delete() {
  local account repos name arn n=0 bad=()
  account="$(account_id)"
  repos="$(aws ecr describe-repositories --region "$HUB_REGION" \
    --query 'repositories[?starts_with(repositoryName, `encabulator/`)].[repositoryName,repositoryArn]' --output text)"
  # Check every repository first, and delete none unless all of them pass.
  while read -r name arn; do
    [[ -n "$name" && "$name" != None ]] || continue
    n=$((n + 1))
    [[ "$name" == encabulator/* ]] || bad+=("$name: name")
    [[ "$arn" =~ ^arn:aws[a-z-]*:ecr:${HUB_REGION}:${account}:repository/ && "${arn#*:repository/}" == "$name" ]] \
      || bad+=("$name: not in this account and region")
    [[ "$(aws ecr list-tags-for-resource --region "$HUB_REGION" --resource-arn "$arn" --query 'tags[?Key==`project`].Value' --output text)" == encabulator ]] \
      || bad+=("$name: no tag project=encabulator")
  done <<<"$repos"
  [[ $n -gt 0 ]] || { say "no ECR repository encabulator/* in $HUB_REGION: nothing to delete"; return 0; }
  [[ ${#bad[@]} -eq 0 ]] || die "ECR repository ${bad[0]}, so no repository was deleted. Check it and delete it by hand if it is yours."
  $in_teardown || confirm "$HUB_CLUSTER" "the $n ECR repositories encabulator/* in $HUB_REGION and their images"
  while read -r name _; do
    [[ -n "$name" && "$name" != None ]] || continue
    aws ecr delete-repository --region "$HUB_REGION" --repository-name "$name" --force >/dev/null
    say "deleted  $name"
  done <<<"$repos"
  say "$n ECR repositories deleted"
}

# ------------------------------------------------------------------------ the hub
hub_steps=(
  "check the credentials and kubectl, list what is deleted, and ask"
  "remove every cell, one after the other (the steps of make remove-cell)"
  "stop Argo CD on the hub, and check that the store objects stay gone"
  "remove the app (make undeploy), its volumes and the platform (make platform-uninstall)"
  "delete the hub's RDS instance, Valkey cache, DB subnet group and cache snapshots"
  "delete the ECR repositories encabulator/* (make mirror-images-delete)"
  "terraform destroy -refresh=false, after a guard has checked the plan"
  "prove the account is clean (make gone-check)"
)
hub_up=false

# hub_cells: the cells to remove: every EncabulatorCell on the hub and every cell file.
hub_cells() {
  local name f names
  for f in "$cells_dir"/*.yaml; do [[ -e "$f" ]] && basename "$f" .yaml; done
  names="$(kget -n "$cells_ns" get encabulatorcells -o 'jsonpath={range .items[*]}{.metadata.name}{" "}{end}')" || exit 1
  for name in $names; do
    grep -lqx "  name: $name" "$cells_dir"/*.yaml 2>/dev/null || echo "${name#encabulator-}"
  done
}

tf() { "${terraform[@]}" -chdir="$TF_DIR" "$@"; }
state_list() {
  local out
  out="$(tf state list 2>&1)" || die "terraform state list failed: $(tail -n 1 <<<"$out"). Run make brownfield-init first."
  printf '%s' "$out"
}

hub_check() {
  local cells c name region n r status
  account_id >/dev/null
  status="$(cluster_status "$HUB_REGION" "$HUB_CLUSTER")"
  if [[ -n "$status" ]]; then
    hub_up=true
    require_hub
  else
    say "the hub cluster $HUB_CLUSTER is gone: the cell, Argo CD, app and store steps have nothing to work with"
  fi
  printf '\n  %sThis deletes, in the account of your credentials:%s\n' "$bold" "$off"
  if $hub_up; then
    cells="$(hub_cells | sort -u)"
    for c in $cells; do
      cell_resolve "$c"
      region="$cell_region"
      [[ -n "$region" ]] || region="$(kget -n "$cells_ns" get encabulatorcells "$cell_name" -o 'jsonpath={.spec.region}')"
      say "cell $cell_name (${region:-region unknown}): its cluster, VPC, roles and stores"
    done
  fi
  say "hub $HUB_CLUSTER ($HUB_REGION): Argo CD's Applications, the app, the platform, the stores"
  n="$(aws ecr describe-repositories --region "$HUB_REGION" --query 'length(repositories[?starts_with(repositoryName, `encabulator/`)])' --output text)"
  say "$n ECR repositories encabulator/* in $HUB_REGION, with their images"
  n="$(state_list | grep -c . || true)"
  say "$n resources in the Terraform state of $TF_DIR: the cluster, its VPC and NAT gateway, the IAM roles"
  for r in "$HUB_REGION" $(awk '/^spec:/ { s = 1; next } /^[^ #]/ { s = 0 } s && /^  region:/ { print $2 }' "$cells_dir"/*.yaml 2>/dev/null | sort -u); do
    name="$(aws_or_none DBInstanceNotFound rds describe-db-instances --region "$r" --db-instance-identifier encabulator-pg --query 'DBInstances[0].DBInstanceIdentifier' --output text)"
    [[ -z "$name" ]] || say "stores in $r: RDS encabulator-pg, Valkey encabulator-valkey, DB subnet group encabulator"
  done
  say "${bold}Data that is lost${off}: orders, ledger and feature flags in every RDS instance, the Valkey caches,"
  say "the NATS and InfluxDB volumes, Prometheus, Grafana and Jaeger history, the images in ECR."
  say "No final snapshot is kept."
  confirm "$HUB_CLUSTER" "all of it"
}

hub_remove_cells() {
  local c cells
  $hub_up || { say "no hub: nothing to do"; return 0; }
  cells="$(hub_cells | sort -u)"
  [[ -n "$cells" ]] || { say "no cell"; return 0; }
  in_teardown=true
  for c in $cells; do
    printf '\n  %scell %s%s\n' "$bold" "$c" "$off"
    local saved=("${steps[@]}") saved_now=$step_now saved_resume="$resume"
    step_prefix="2.$c." resume="make teardown"
    remove_cell "$c"
    steps=("${saved[@]}") step_now=$saved_now step_prefix="" resume="$saved_resume"
  done
}

store_objects() {
  local kind name out=""
  for kind in dbinstances.rds.services.k8s.aws/encabulator-pg serverlesscaches.elasticache.services.k8s.aws/encabulator-valkey dbsubnetgroups.rds.services.k8s.aws/encabulator; do
    name="$(kget -n "$NAMESPACE" get "${kind%/*}" "${kind#*/}" -o name)" || exit 1
    out+="${name:+$name }"
  done
  printf '%s' "$out"
}
hub_stop_argocd() {
  local left
  $hub_up || { say "no hub: nothing to do"; return 0; }
  left="$(kget -n "$cells_ns" get encabulatorcells -o name)"
  [[ -z "$left" ]] || die "The hub still has $left: remove every cell first (step 2), or kro cannot delete it once hub-rgds is gone."
  if exists argocd applicationsets.argoproj.io hub-brownfield; then
    kc -n argocd delete applicationsets.argoproj.io hub-brownfield --wait=false >/dev/null
    say "ApplicationSet hub-brownfield: deleted; Argo CD deletes its Application and the three store objects (retain keeps the stores)"
  fi
  brownfield_gone() {
    local o
    o="$(kget -n argocd get applications.argoproj.io hub-brownfield -o name)" || { stopped; exit 1; }
    o+="$(store_objects)" || { stopped; exit 1; }
    [[ -z "$o" ]]
  }
  wait_for "$TEARDOWN_TIMEOUT_ARGOCD" "the Application hub-brownfield and its store objects to go (about 4 minutes)" brownfield_gone
  kc -n argocd delete applications.argoproj.io hub-cells hub-cell-apps hub-rgds --ignore-not-found --wait=false >/dev/null
  kc -n argocd delete applicationsets.argoproj.io encabulator encabulator-platform --ignore-not-found --wait=false >/dev/null
  say "Applications hub-cells, hub-cell-apps, hub-rgds and ApplicationSets encabulator, encabulator-platform: deleted"
  sleep "$TEARDOWN_SETTLE"
  left="$(store_objects)"
  [[ -z "$left" ]] || die "Something re-created $left. Find what still syncs it (kubectl -n argocd get applications,applicationsets) and delete that first."
  left="$(kget -n argocd get applications.argoproj.io,applicationsets.argoproj.io -o name | paste -sd' ' -)"
  [[ -z "$left" ]] || die "Argo CD still has $left: delete it, then run make teardown again."
  say "Argo CD has no Application left, and the store objects stayed gone for ${TEARDOWN_SETTLE}s"
}

hub_app() {
  local lbs
  $hub_up || { say "no hub: nothing to do"; return 0; }
  if helm status encabulator --namespace "$NAMESPACE" >/dev/null 2>&1; then
    "${make_cmd[@]}" --no-print-directory -C "$repo_root" undeploy
  else
    say "Helm release encabulator: already gone"
  fi
  kc -n "$NAMESPACE" delete pvc -l app.kubernetes.io/part-of=encabulator --ignore-not-found --wait=false >/dev/null
  "${make_cmd[@]}" --no-print-directory -C "$repo_root" platform-uninstall
  lbs="$(kget get svc -A -o 'jsonpath={range .items[?(@.spec.type=="LoadBalancer")]}{.metadata.namespace}/{.metadata.name}{" "}{end}')"
  [[ -z "$lbs" ]] || die "Service $lbs is of type LoadBalancer, and its load balancer would keep the VPC from being deleted: delete it, then run make teardown again."
  wait_for "$TEARDOWN_TIMEOUT_VOLUMES" "every volume on the hub to be deleted (kubectl get pv lists nothing)" none_left kget get pv -o name
  say "no Helm release of the app, no volume and no load balancer left on the hub"
}

hub_stores() {
  local list left
  list="$(state_list)"
  if grep -qx 'aws_db_instance.pg' <<<"$list"; then
    say "Terraform still owns the hub's stores (before the hand-off): terraform destroy deletes them in step 7."
    say "Their automatic snapshots outlive Terraform, so they go now:"
    if $hub_up || [[ "$TEARDOWN_STORES" == cli ]]; then snapshots_delete "$HUB_REGION" "$HUB_CLUSTER" "$NAMESPACE"; fi
    return 0
  fi
  if ! $hub_up && [[ "$TEARDOWN_STORES" == ack ]]; then
    left="$(aws_or_none DBInstanceNotFound rds describe-db-instances --region "$HUB_REGION" --db-instance-identifier encabulator-pg --query 'DBInstances[0].DBInstanceStatus' --output text)"
    left+="$(aws_or_none ServerlessCacheNotFound elasticache describe-serverless-caches --region "$HUB_REGION" --serverless-cache-name encabulator-valkey --query 'ServerlessCaches[0].Status' --output text)"
    [[ -z "$left" ]] || die "The hub's stores are still in $HUB_REGION and the hub's ACK is gone with the hub: run make teardown TEARDOWN_STORES=cli."
    say "no store left in $HUB_REGION"; return 0
  fi
  stores_delete "$HUB_REGION" "$HUB_CLUSTER" "$NAMESPACE"
}

# protect_state: the state holds the generated passwords; keep every copy at mode 600.
protect_state() {
  local path f
  path="$(python3 -c 'import json,sys
try: print(json.load(open(sys.argv[1]))["backend"]["config"].get("path") or "")
except Exception: print("")' "$TF_DIR/.terraform/terraform.tfstate" 2>/dev/null)"
  [[ -n "$path" ]] || path="$TF_DIR/terraform.tfstate"
  for f in "$path" "$path.backup"; do [[ -f "$f" ]] && chmod 600 "$f"; done
  return 0
}

hub_terraform() {
  local list account ts plan backup rc
  list="$(state_list)"
  if [[ -z "$list" ]]; then
    protect_state
    say "The Terraform state is empty: the hub is already destroyed, nothing to do."
    return 0
  fi
  account="$(account_id)"
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  backup="$TF_DIR/terraform.tfstate.pre-destroy-$ts"
  # $work is mode 700; install sets the backup's mode whatever the source's is (cp -p would
  # keep a 644 source's mode).
  tf state pull >"$work/state" || die "terraform state pull failed."
  install -m 600 "$work/state" "$backup"
  say "state saved to $backup (mode 600; it holds the generated passwords)"
  plan="$work/destroy.tfplan"
  say "terraform plan -destroy -refresh=false (the stack reads the deleted stores by name, so a refresh fails)"
  tf plan -destroy -refresh=false -input=false -no-color -out="$plan" >"$work/plan.log" 2>&1 \
    || { tail -n 20 "$work/plan.log" >&2; die "terraform plan -destroy failed. Set IDC_INSTANCE_ARN in .env as for make hub."; }
  tf show -json "$plan" >"$work/plan.json"
  set +e
  python3 "$guard" --account "$account" --region "$HUB_REGION" --cluster "$HUB_CLUSTER" <"$work/plan.json" | sed 's/^/  /'
  rc=${PIPESTATUS[0]}
  set -e
  [[ $rc -eq 0 ]] || die "The guard refused the destroy plan (the REFUSED lines above), so Terraform deleted nothing."
  say "terraform apply of the destroy plan (about 6 minutes)"
  set +e
  tf apply -input=false -no-color "$plan" 2>&1 | grep -E '^(Apply complete|Error|Destroy complete)' | sed 's/^/  /'
  rc=${PIPESTATUS[0]}
  set -e
  protect_state
  [[ $rc -eq 0 ]] || die "terraform apply of the destroy plan failed (exit $rc)."
}

hub_gone_check() {
  "$repo_root/scripts/gone-check.sh" || die "make gone-check still lists the lines marked FAIL above. The tagging index lags by hours: run make gone-check again later."
}

teardown() {
  local fns=(hub_check hub_remove_cells hub_stop_argocd hub_app hub_stores ecr_step hub_terraform hub_gone_check) i
  steps=("${hub_steps[@]}")
  for ((i = 1; i <= ${#fns[@]}; i++)); do step "$i"; "${fns[$i - 1]}"; done
  step_now=0
  printf '\n%sThe project is gone from this account.%s Left for you: nothing. The EC2 instant-fleet\n' "$bold" "$off"
  printf 'records EKS Auto Mode leaves (WARN above) cost nothing and cannot be deleted.\n'
}
ecr_step() { in_teardown=true; ecr_delete; }

# --------------------------------------------------------------------------- main
dry=false
make_dry_run && dry=true
case "${1:-}" in
  remove-cell)
    resume="make remove-cell CELL=${2:-${CELL:-}}"
    if $dry; then
      cell_resolve "${2:-${CELL:-}}"
      steps=("${cell_steps[@]}")
      print_plan "make remove-cell CELL=$cell_key removes $cell_name${cell_region:+ in $cell_region}, in this order:"
      exit 0
    fi
    remove_cell "${2:-${CELL:-}}"
    printf '\n%sThe cell %s is gone.%s Left for you: take its file out of Git (step 4 printed the command).\n' "$bold" "$cell_name" "$off"
    printf 'make gone-check lists anything of the project still in the account.\n'
    ;;
  teardown)
    resume="make teardown"
    if $dry; then
      steps=("${hub_steps[@]}")
      print_plan "make teardown deletes the whole project, in this order:"
      exit 0
    fi
    teardown
    ;;
  ecr-delete)
    resume="make mirror-images-delete"
    if $dry; then
      printf 'make mirror-images-delete deletes every ECR repository encabulator/* in %s that carries\n' "$HUB_REGION"
      printf 'the tag project=encabulator, with its images, after you type %s.\nmake -n: nothing was run.\n' "$HUB_CLUSTER"
      exit 0
    fi
    ecr_delete
    ;;
  *) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
