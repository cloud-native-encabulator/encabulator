#!/usr/bin/env python3
"""Fail on any AWS resource of the OPN315 stacks that lacks the tag auto-delete=no.

Read-only. Lists each stack's resources through AWS describe/list calls (the AWS CLI, with
whatever credentials it finds) and prints one line per resource. Exits 1 naming every resource
without auto-delete=no.

A stack is a region and the name its resources carry: the hub is us-west-2:encabulator-usw2,
the us-east-2 cell us-east-2:encabulator-use2. Resources are found by structure, never by the
tag being checked:

  VPC            the VPC tagged Name=<name>, then in it: subnets, route tables, network ACLs,
                 security groups, internet gateways, NAT gateways, network interfaces (requester-
                 managed ones too), instances, and the Elastic IPs of its NAT gateways and
                 network interfaces
  EBS            volumes attached to those instances, plus volumes tagged eks:eks-cluster-name=<name>
  EC2            launch templates tagged eks:eks-cluster-name=<name> (EKS Auto Mode)
  EKS            cluster <name>, its capabilities, access entries and Pod Identity associations
  RDS            DB instances and DB subnet groups in the VPC, the instances' snapshots
  ElastiCache    serverless caches in the VPC's subnets and their snapshots
  IAM            roles named <name>-*, and their instance profiles
  ECR            repositories named encabulator/* in the stack's region (once per region)

Usage:
  scripts/tag-check.py [--stack REGION:NAME ...] [--aws "aws --profile x"]
  scripts/tag-check.py --fixtures FILE      # recorded responses instead of AWS (tests)
  scripts/tag-check.py --record FILE ...    # also save every response (account ids inside:
                                            # keep the file out of Git)

Run it at least 10 minutes after the last change: what AWS creates without tags (default
route table, cluster security group, requester-managed interfaces, ...) is tagged by the hub's
tagger CronJob, which runs every 10 minutes.

Exit status: 0 every resource carries auto-delete=no, 1 at least one does not (or a stack is
missing), 2 an AWS CLI call failed.
"""
from __future__ import annotations

import argparse
import json
import shlex
import subprocess
import sys
from pathlib import Path

DEFAULT_STACKS = ["us-west-2:encabulator-usw2", "us-east-2:encabulator-use2"]
# ECR repositories are per project, not per stack: every repository under this prefix in a
# stack's region (images mirrored for a live run, e.g. encabulator/dingle-arm-gateway).
ECR_PREFIX = "encabulator/"
REQUIRED = ("auto-delete", "no")
CONTRACT = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
READ_ONLY = ("describe-", "list-", "get-")
NOT_FOUND = {"ResourceNotFoundException", "NoSuchEntity", "DBInstanceNotFound", "DBInstanceNotFoundFault",
             "ServerlessCacheNotFoundFault"}


class CallError(Exception):
    def __init__(self, code, message):
        super().__init__(f"{code}: {message}")
        self.code = code


def key(service, op, region, args):
    return " ".join([service, op, "--region", region, *args])


class LiveSource:
    def __init__(self, aws, record=None):
        self.aws = shlex.split(aws)
        self.record = record
        self.calls = {}

    def call(self, service, op, region, *args):
        if not op.startswith(READ_ONLY):
            raise SystemExit(f"tag-check: refusing {service} {op}: only describe/list/get calls")
        cmd = self.aws + [service, op, "--region", region, "--output", "json", *args]
        proc = subprocess.run(cmd, capture_output=True, text=True)
        if proc.returncode != 0:
            code = "Unknown"
            for name in NOT_FOUND:
                if name in proc.stderr:
                    code = name
            if code in NOT_FOUND:
                self.calls[key(service, op, region, args)] = {"__error__": code}
                raise CallError(code, proc.stderr.strip())
            print(f"tag-check: {' '.join(cmd)} failed:\n{proc.stderr.strip()}", file=sys.stderr)
            sys.exit(2)
        out = json.loads(proc.stdout or "{}")
        self.calls[key(service, op, region, args)] = out
        return out

    def save(self):
        if self.record:
            Path(self.record).write_text(json.dumps({"calls": self.calls}, indent=1, sort_keys=True) + "\n")


class FixtureSource:
    """Recorded responses: {"calls": {key: response}}, optionally {"base": FILE, "calls": {...}}
    to override a few responses of another fixture."""

    def __init__(self, path):
        self.calls = self.load(Path(path))

    def load(self, path):
        doc = json.loads(path.read_text())
        calls = self.load(path.parent / doc["base"]) if "base" in doc else {}
        calls.update(doc.get("calls", {}))
        return calls

    def call(self, service, op, region, *args):
        if not op.startswith(READ_ONLY):
            raise SystemExit(f"tag-check: refusing {service} {op}: only describe/list/get calls")
        k = key(service, op, region, args)
        if k not in self.calls:
            raise SystemExit(f"tag-check: the fixture has no response for: aws {k}")
        out = self.calls[k]
        if isinstance(out, dict) and "__error__" in out:
            raise CallError(out["__error__"], "recorded error")
        return out

    def save(self):
        pass


def tags_of(items):
    """{Key, Value} lists (EC2, RDS, ElastiCache, IAM) or a map (EKS)."""
    if isinstance(items, dict):
        return {str(k): str(v) for k, v in items.items()}
    return {t.get("Key"): t.get("Value", "") for t in items or []}


class Report:
    def __init__(self):
        self.lines = []
        self.bad = 0
        self.count = 0
        self.seen = set()

    def resource(self, region, kind, rid, tags, detail=""):
        tags = tags_of(tags)
        self.seen.add(rid)
        self.count += 1
        ok = tags.get(REQUIRED[0]) == REQUIRED[1]
        extra = [k for k, v in CONTRACT.items() if k != REQUIRED[0] and tags.get(k) != v]
        note = detail
        if ok and extra:
            note = (note + "; " if note else "") + f"note: lacks {', '.join(extra)}"
        if not ok:
            self.bad += 1
        self.lines.append(f"{'ok     ' if ok else 'MISSING'}  {region:<10} {kind:<32} {rid}" + (f"  ({note})" if note else ""))

    def problem(self, region, what):
        self.bad += 1
        self.lines.append(f"MISSING  {region:<10} {'stack':<32} {what}")


def check_vpc(call, report, region, vpc):
    """Everything inside one VPC. Returns its subnet ids."""
    subnet_ids = []
    in_vpc = f"Name=vpc-id,Values={vpc}"
    for s in call("ec2", "describe-subnets", "--filters", in_vpc)["Subnets"]:
        subnet_ids.append(s["SubnetId"])
        report.resource(region, "ec2 subnet", s["SubnetId"], s.get("Tags"))
    for rt in call("ec2", "describe-route-tables", "--filters", in_vpc)["RouteTables"]:
        main = any(a.get("Main") for a in rt.get("Associations", []))
        report.resource(region, "ec2 route-table", rt["RouteTableId"], rt.get("Tags"), "main" if main else "")
    for acl in call("ec2", "describe-network-acls", "--filters", in_vpc)["NetworkAcls"]:
        report.resource(region, "ec2 network-acl", acl["NetworkAclId"], acl.get("Tags"), "default" if acl.get("IsDefault") else "")
    for sg in call("ec2", "describe-security-groups", "--filters", in_vpc)["SecurityGroups"]:
        report.resource(region, "ec2 security-group", sg["GroupId"], sg.get("Tags"), sg.get("GroupName", ""))
    for igw in call("ec2", "describe-internet-gateways", "--filters", f"Name=attachment.vpc-id,Values={vpc}")["InternetGateways"]:
        report.resource(region, "ec2 internet-gateway", igw["InternetGatewayId"], igw.get("Tags"))
    allocations = set()
    for nat in call("ec2", "describe-nat-gateways", "--filter", in_vpc)["NatGateways"]:
        if nat.get("State") in ("deleted", "deleting"):
            continue
        allocations |= {a.get("AllocationId") for a in nat.get("NatGatewayAddresses", [])}
        report.resource(region, "ec2 natgateway", nat["NatGatewayId"], nat.get("Tags"))
    enis = set()
    for eni in call("ec2", "describe-network-interfaces", "--filters", in_vpc)["NetworkInterfaces"]:
        enis.add(eni["NetworkInterfaceId"])
        detail = f"requester-managed: {eni.get('Description', '')}" if eni.get("RequesterManaged") else eni.get("Description", "")
        report.resource(region, "ec2 network-interface", eni["NetworkInterfaceId"], eni.get("TagSet"), detail)
    for addr in call("ec2", "describe-addresses")["Addresses"]:
        if addr.get("AllocationId") in allocations or addr.get("NetworkInterfaceId") in enis:
            report.resource(region, "ec2 elastic-ip", addr["AllocationId"], addr.get("Tags"), addr.get("PublicIp", ""))
    instances = []
    for res in call("ec2", "describe-instances", "--filters", in_vpc,
                    "Name=instance-state-name,Values=pending,running,stopping,stopped")["Reservations"]:
        for inst in res["Instances"]:
            instances.append(inst["InstanceId"])
            report.resource(region, "ec2 instance", inst["InstanceId"], inst.get("Tags"), inst.get("InstanceType", ""))
    volumes = {}
    if instances:
        for vol in call("ec2", "describe-volumes", "--filters", f"Name=attachment.instance-id,Values={','.join(instances)}")["Volumes"]:
            volumes[vol["VolumeId"]] = vol
    for vid, vol in sorted(volumes.items()):
        report.resource(region, "ec2 volume", vid, vol.get("Tags"), f"{vol.get('Size', '?')} GiB, {vol.get('State', '')}")
    return subnet_ids


def check_stack(src, report, region, name):
    def call(service, op, *args):
        return src.call(service, op, region, *args)

    vpcs = call("ec2", "describe-vpcs", "--filters", f"Name=tag:Name,Values={name}")["Vpcs"]
    if not vpcs:
        report.problem(region, f"no VPC tagged Name={name}")
    vpc_ids = [v["VpcId"] for v in vpcs]
    subnet_ids = []
    for v in vpcs:
        report.resource(region, "ec2 vpc", v["VpcId"], v.get("Tags"), name)
        subnet_ids += check_vpc(call, report, region, v["VpcId"])
    vols = {}
    for vol in call("ec2", "describe-volumes", "--filters", f"Name=tag:eks:eks-cluster-name,Values={name}")["Volumes"]:
        vols[vol["VolumeId"]] = vol
    for vid, vol in sorted(vols.items()):
        if vid not in report.seen:
            report.resource(region, "ec2 volume", vid, vol.get("Tags"), f"{vol.get('Size', '?')} GiB, {vol.get('State', '')}")
    for lt in call("ec2", "describe-launch-templates", "--filters", f"Name=tag:eks:eks-cluster-name,Values={name}")["LaunchTemplates"]:
        report.resource(region, "ec2 launch-template", lt["LaunchTemplateId"], lt.get("Tags"), lt.get("LaunchTemplateName", ""))

    # EKS
    try:
        cluster = call("eks", "describe-cluster", "--name", name)["cluster"]
    except CallError:
        report.problem(region, f"no EKS cluster {name}")
        cluster = None
    if cluster:
        report.resource(region, "eks cluster", name, cluster.get("tags"))
        for cap in call("eks", "list-capabilities", "--cluster-name", name).get("capabilities", []):
            cname = cap["capabilityName"]
            detail = call("eks", "describe-capability", "--cluster-name", name, "--capability-name", cname)["capability"]
            report.resource(region, "eks capability", f"{name}/{cname}", detail.get("tags"), detail.get("type", ""))
        for principal in call("eks", "list-access-entries", "--cluster-name", name).get("accessEntries", []):
            entry = call("eks", "describe-access-entry", "--cluster-name", name, "--principal-arn", principal)["accessEntry"]
            report.resource(region, "eks access-entry", f"{name}/{principal.split('/')[-1]}", entry.get("tags"), entry.get("type", ""))
        for assoc in call("eks", "list-pod-identity-associations", "--cluster-name", name).get("associations", []):
            detail = call("eks", "describe-pod-identity-association", "--cluster-name", name,
                          "--association-id", assoc["associationId"])["association"]
            report.resource(region, "eks pod-identity-association", assoc["associationId"], detail.get("tags"),
                            f"{assoc.get('namespace')}/{assoc.get('serviceAccount')}")

    # RDS
    if vpc_ids:
        for db in call("rds", "describe-db-instances")["DBInstances"]:
            if (db.get("DBSubnetGroup") or {}).get("VpcId") not in vpc_ids:
                continue
            ident = db["DBInstanceIdentifier"]
            report.resource(region, "rds db-instance", ident, db.get("TagList"))
            for snap in call("rds", "describe-db-snapshots", "--db-instance-identifier", ident)["DBSnapshots"]:
                report.resource(region, "rds snapshot", snap["DBSnapshotIdentifier"], snap.get("TagList"), snap.get("SnapshotType", ""))
        for group in call("rds", "describe-db-subnet-groups")["DBSubnetGroups"]:
            if group.get("VpcId") not in vpc_ids:
                continue
            tags = call("rds", "list-tags-for-resource", "--resource-name", group["DBSubnetGroupArn"])["TagList"]
            report.resource(region, "rds subnet-group", group["DBSubnetGroupName"], tags)

    # ElastiCache
    if vpc_ids:
        for cache in call("elasticache", "describe-serverless-caches")["ServerlessCaches"]:
            if not set(cache.get("SubnetIds", [])) & set(subnet_ids):
                continue
            cname = cache["ServerlessCacheName"]
            tags = call("elasticache", "list-tags-for-resource", "--resource-name", cache["ARN"])["TagList"]
            report.resource(region, "elasticache serverless-cache", cname, tags)
            for snap in call("elasticache", "describe-serverless-cache-snapshots", "--serverless-cache-name", cname)["ServerlessCacheSnapshots"]:
                tags = call("elasticache", "list-tags-for-resource", "--resource-name", snap["ARN"])["TagList"]
                report.resource(region, "elasticache snapshot", snap["ServerlessCacheSnapshotName"], tags)

    # IAM (global; called with the stack's region)
    for role in call("iam", "list-roles")["Roles"]:
        rname = role["RoleName"]
        if not rname.startswith(f"{name}-"):
            continue
        tags = call("iam", "list-role-tags", "--role-name", rname)["Tags"]
        report.resource(region, "iam role", rname, tags)
        for profile in call("iam", "list-instance-profiles-for-role", "--role-name", rname)["InstanceProfiles"]:
            pname = profile["InstanceProfileName"]
            tags = call("iam", "list-instance-profile-tags", "--instance-profile-name", pname)["Tags"]
            report.resource(region, "iam instance-profile", pname, tags, f"role {rname}")

    # ECR (per region, shared by every stack in it: counted once)
    for repo in call("ecr", "describe-repositories")["repositories"]:
        rname, arn = repo["repositoryName"], repo["repositoryArn"]
        if not rname.startswith(ECR_PREFIX) or arn in report.seen:
            continue
        tags = call("ecr", "list-tags-for-resource", "--resource-arn", arn)["tags"]
        report.resource(region, "ecr repository", rname, tags)
        report.seen.add(arn)


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--stack", action="append", metavar="REGION:NAME",
                        help=f"repeatable; default {' '.join(DEFAULT_STACKS)}")
    parser.add_argument("--aws", default="aws", help="AWS CLI command (default: aws)")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--fixtures", help="read recorded responses from this file instead of calling AWS")
    group.add_argument("--record", help="also write every response to this file")
    args = parser.parse_args()

    stacks = []
    for item in args.stack or DEFAULT_STACKS:
        region, _, name = item.partition(":")
        if not region or not name:
            parser.error(f"--stack wants REGION:NAME, got {item!r}")
        stacks.append((region, name))

    src = FixtureSource(args.fixtures) if args.fixtures else LiveSource(args.aws, args.record)
    report = Report()
    try:
        for region, name in stacks:
            check_stack(src, report, region, name)
    finally:
        src.save()
    print("\n".join(report.lines))
    if report.bad:
        print(f"{report.bad} of {report.count} resources (or stacks) MISSING auto-delete=no", file=sys.stderr)
        return 1
    print(f"all {report.count} resources carry auto-delete=no", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
