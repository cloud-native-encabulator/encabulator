#!/usr/bin/env python3
"""Check a Terraform destroy plan of terraform/brownfield before it is applied. Read-only.

  terraform show -json PLAN | teardown-plan-guard.py --account ACCOUNT --region REGION --cluster NAME

Exit 0 only when every planned change:
  - is a delete (no create, update or replace) of a managed resource, and there is at least one;
  - is a resource this stack declares (its configuration, module.vpc included), from the aws,
    kubernetes, helm or random provider: a state that holds anything else is refused;
  - for AWS resources: carries auto-delete=no, project=encabulator and talk=opn315 and a Name
    tag (when it has one) that starts with encabulator; has its ARN in ACCOUNT and in REGION
    (IAM is global); is named for the hub: the EKS cluster NAME, IAM roles NAME-*, the VPC
    tagged Name=NAME, the security group encabulator-db, stores named encabulator*; and sits in
    that VPC, on that cluster, or under a role, route table, subnet or security group that is
    deleted in the same plan.
Prints a summary by type, then GUARD OK or one REFUSED line per problem. Exit 1 on a refusal.
"""
import argparse
import json
import re
import sys
from collections import Counter

CONTRACT = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
PROVIDERS = {f"registry.terraform.io/hashicorp/{p}" for p in ("aws", "kubernetes", "helm", "random")}


def mask(s):
    return re.sub(r"\b\d{12}\b", "<account-id>", str(s))


def configured(module, prefix=""):
    """Every managed resource address the configuration declares, module calls included."""
    out = set()
    for r in module.get("resources", []):
        if r.get("mode", "managed") == "managed":
            out.add(prefix + r["address"])
    for name, call in (module.get("module_calls") or {}).items():
        out |= configured(call.get("module", {}), f"{prefix}module.{name}.")
    return out


def base(address):
    """module.vpc.aws_subnet.private[0] -> module.vpc.aws_subnet.private"""
    return re.sub(r"\[[^\]]*\]", "", address)


def rtype(address):
    return base(address).split(".")[-2]


def check(plan, account, region, cluster):
    bad = []
    changes = plan.get("resource_changes") or []
    known = configured((plan.get("configuration") or {}).get("root_module") or {})
    by = {rc["address"]: (rc.get("change") or {}).get("before") or {} for rc in changes}

    if not changes:
        bad.append("the plan deletes nothing, but the state is not empty")
    for rc in changes:
        a = rc["address"]
        if (rc.get("change") or {}).get("actions") != ["delete"]:
            bad.append(f"{a}: action {(rc.get('change') or {}).get('actions')}, only delete is allowed")
        if rc.get("mode") != "managed":
            bad.append(f"{a}: not a managed resource")
        if base(a) not in known:
            bad.append(f"{a}: not a resource of terraform/brownfield")
        if rc.get("provider_name") not in PROVIDERS:
            bad.append(f"{a}: provider {rc.get('provider_name')}")

    def planned(t):
        return [b for addr, b in by.items() if rtype(addr) == t]

    vpcs = planned("aws_vpc")
    vpc_ids = {b.get("id") for b in vpcs}
    for b in vpcs:
        if (b.get("tags_all") or {}).get("Name") != cluster:
            bad.append(f"VPC {b.get('id')}: Name tag {(b.get('tags_all') or {}).get('Name')}, not {cluster}")
    clusters = planned("aws_eks_cluster")
    for b in clusters:
        if b.get("name") != cluster:
            bad.append(f"EKS cluster {b.get('name')}, not {cluster}")
    cluster_sgs = {((b.get("vpc_config") or [{}])[0] or {}).get("cluster_security_group_id") for b in clusters}
    roles = {b.get("name") for b in planned("aws_iam_role")}
    rtbs = {b.get("id") for b in planned("aws_route_table")} | {b.get("id") for b in planned("aws_default_route_table")}
    subnets = {b.get("id") for b in planned("aws_subnet")}
    sgs = {b.get("id") for b in planned("aws_security_group")}
    accounts = set()

    for addr, b in by.items():
        t = rtype(addr)
        if not t.startswith("aws_"):
            continue
        if "tags_all" in b:
            tags = b.get("tags_all") or {}
            for k, v in CONTRACT.items():
                if tags.get(k) != v:
                    bad.append(f"{addr}: tag {k}={tags.get(k)}, not {v}")
            if "Name" in tags and not str(tags["Name"]).startswith("encabulator"):
                bad.append(f"{addr}: Name tag {tags['Name']} does not start with encabulator")
        arn = b.get("arn") or ""
        parts = arn.split(":")
        if len(parts) > 4:
            if parts[2] != "iam" and parts[3] not in (region, ""):
                bad.append(f"{addr}: in region {parts[3]}, not {region}")
            if parts[4]:
                accounts.add(parts[4])
        if b.get("vpc_id") and b.get("vpc_id") not in vpc_ids:
            bad.append(f"{addr}: in VPC {b.get('vpc_id')}, which this plan does not delete")
        if t == "aws_iam_role" and not str(b.get("name", "")).startswith(cluster + "-"):
            bad.append(f"{addr}: role {b.get('name')} is not named {cluster}-*")
        if t in ("aws_iam_role_policy", "aws_iam_role_policy_attachment") and b.get("role") not in roles:
            bad.append(f"{addr}: role {b.get('role')} is not deleted in this plan")
        if t.startswith("aws_eks_") and t != "aws_eks_cluster" and b.get("cluster_name") != cluster:
            bad.append(f"{addr}: on cluster {b.get('cluster_name')}, not {cluster}")
        if t in ("aws_route", "aws_route_table_association") and b.get("route_table_id") not in rtbs:
            bad.append(f"{addr}: route table {b.get('route_table_id')} is not deleted in this plan")
        if t == "aws_route_table_association" and b.get("subnet_id") not in subnets:
            bad.append(f"{addr}: subnet {b.get('subnet_id')} is not deleted in this plan")
        if t == "aws_vpc_security_group_ingress_rule" and b.get("security_group_id") not in sgs:
            bad.append(f"{addr}: security group {b.get('security_group_id')} is not deleted in this plan")
        if t == "aws_ec2_tag" and b.get("resource_id") not in cluster_sgs:
            bad.append(f"{addr}: tags {b.get('resource_id')}, not the cluster security group")
        if t == "aws_security_group" and b.get("name") != "encabulator-db":
            bad.append(f"{addr}: security group {b.get('name')}, not encabulator-db")
        if t == "aws_db_instance" and not str(b.get("identifier", "")).startswith("encabulator"):
            bad.append(f"{addr}: RDS instance {b.get('identifier')}")
        if t in ("aws_db_subnet_group", "aws_elasticache_serverless_cache") and not str(b.get("name", "")).startswith("encabulator"):
            bad.append(f"{addr}: {t} {b.get('name')}")

    for acc in sorted(accounts):
        if acc != account:
            bad.append(f"an ARN is in account {acc}, not the account of your credentials")
    if changes and not accounts:
        bad.append("no ARN in the plan to check the account against")
    return changes, bad


def main():
    p = argparse.ArgumentParser(description="Check a destroy plan of terraform/brownfield.")
    p.add_argument("--account", required=True)
    p.add_argument("--region", required=True)
    p.add_argument("--cluster", required=True)
    args = p.parse_args()
    if not re.fullmatch(r"\d{12}", args.account):
        print("REFUSED: --account is not a 12-digit account id")
        return 1
    try:
        plan = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(f"REFUSED: the plan is not JSON ({e})")
        return 1
    changes, bad = check(plan, args.account, args.region, args.cluster)
    print("The destroy plan deletes:")
    for typ, n in sorted(Counter(rc.get("type") for rc in changes).items()):
        print(f"  {n:3d}  {typ}")
    print(f"  {len(changes):3d}  in total")
    if bad:
        for line in bad:
            print("REFUSED: " + mask(line))
        return 1
    print(f"GUARD OK: only deletes of tagged {args.cluster} resources in {args.region} and IAM")
    return 0


if __name__ == "__main__":
    sys.exit(main())
