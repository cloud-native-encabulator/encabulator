#!/usr/bin/env python3
"""Offline tests for sweep.py: a fake boto3 session over recorded describe shapes.

  python3 terraform/brownfield/tagger/test_sweep.py

No AWS calls and no boto3 needed. The fake EC2 client applies the vpc-id and tag:<key>
filters the way EC2 does, so the tests prove the sweeper selects by VPC and touches nothing
outside the Encabulator VPCs.
"""
from __future__ import annotations

import contextlib
import copy
import io
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import sweep  # noqa: E402

ACCOUNT = "111122223333"
CONTRACT = [{"Key": "auto-delete", "Value": "no"}, {"Key": "project", "Value": "encabulator"},
            {"Key": "talk", "Value": "opn315"}]


def named(name, *extra):
    return [{"Key": "Name", "Value": name}, *extra, *CONTRACT]


# A cell as ACK and EKS leave it: what ACK created carries the tags, what EC2 and EKS created
# on their own does not. Plus a VPC that is not the talk's, which must never be touched.
CELL = {
    "Vpcs": [
        {"VpcId": "vpc-0cell", "Tags": named("encabulator-use2")},
        {"VpcId": "vpc-0other", "Tags": [{"Key": "Name", "Value": "someone-else"}]},
    ],
    "RouteTables": [
        {"RouteTableId": "rtb-0main", "VpcId": "vpc-0cell", "Associations": [{"Main": True}], "Tags": []},
        {"RouteTableId": "rtb-0public", "VpcId": "vpc-0cell", "Associations": [], "Tags": named("encabulator-use2-public")},
        {"RouteTableId": "rtb-0private", "VpcId": "vpc-0cell", "Associations": [], "Tags": named("encabulator-use2-private")},
        {"RouteTableId": "rtb-0othermain", "VpcId": "vpc-0other", "Associations": [{"Main": True}], "Tags": []},
    ],
    "NetworkAcls": [
        {"NetworkAclId": "acl-0default", "VpcId": "vpc-0cell", "IsDefault": True, "Tags": []},
        {"NetworkAclId": "acl-0other", "VpcId": "vpc-0other", "IsDefault": True, "Tags": []},
    ],
    "SecurityGroups": [
        {"GroupId": "sg-0default", "GroupName": "default", "VpcId": "vpc-0cell", "Tags": []},
        {"GroupId": "sg-0cluster", "GroupName": "eks-cluster-sg-encabulator-use2-1234567890", "VpcId": "vpc-0cell",
         "Tags": [{"Key": "aws:eks:cluster-name", "Value": "encabulator-use2"},
                  {"Key": "kubernetes.io/cluster/encabulator-use2", "Value": "owned"},
                  {"Key": "Name", "Value": "eks-cluster-sg-encabulator-use2-1234567890"}]},
        {"GroupId": "sg-0db", "GroupName": "encabulator-db", "VpcId": "vpc-0cell", "Tags": named("encabulator-db")},
        {"GroupId": "sg-0otherdefault", "GroupName": "default", "VpcId": "vpc-0other", "Tags": []},
    ],
    "NetworkInterfaces": [
        {"NetworkInterfaceId": "eni-0node", "VpcId": "vpc-0cell", "RequesterManaged": False,
         "Description": "", "TagSet": [{"Key": "eks:eks-cluster-name", "Value": "encabulator-use2"}, *CONTRACT]},
        {"NetworkInterfaceId": "eni-0eks", "VpcId": "vpc-0cell", "RequesterManaged": True,
         "Description": "Amazon EKS encabulator-use2", "TagSet": []},
        {"NetworkInterfaceId": "eni-0nat", "VpcId": "vpc-0cell", "RequesterManaged": True,
         "Description": "Interface for NAT Gateway nat-0cell", "TagSet": []},
        # auto-delete=no alone is not enough: all three contract tags.
        {"NetworkInterfaceId": "eni-0rds", "VpcId": "vpc-0cell", "RequesterManaged": True,
         "Description": "RDSNetworkInterface", "TagSet": [{"Key": "auto-delete", "Value": "no"}]},
        {"NetworkInterfaceId": "eni-0otherlambda", "VpcId": "vpc-0other", "RequesterManaged": True,
         "Description": "AWS Lambda VPC ENI", "TagSet": []},
    ],
}
EXPECTED_CELL = {"rtb-0main", "acl-0default", "sg-0default", "sg-0cluster", "eni-0eks", "eni-0nat", "eni-0rds"}
OUTSIDE = {"vpc-0other", "rtb-0othermain", "acl-0other", "sg-0otherdefault", "eni-0otherlambda"}

SNAPSHOTS = {
    "encabulator-valkey": [
        {"ServerlessCacheSnapshotName": "encabulator-valkey-auto-1",
         "ARN": f"arn:aws:elasticache:us-east-2:{ACCOUNT}:serverlesscachesnapshot:encabulator-valkey-auto-1"},
        {"ServerlessCacheSnapshotName": "encabulator-valkey-auto-0",
         "ARN": f"arn:aws:elasticache:us-east-2:{ACCOUNT}:serverlesscachesnapshot:encabulator-valkey-auto-0"},
    ],
}
SNAPSHOT_TAGS = {
    f"arn:aws:elasticache:us-east-2:{ACCOUNT}:serverlesscachesnapshot:encabulator-valkey-auto-1": [],
    f"arn:aws:elasticache:us-east-2:{ACCOUNT}:serverlesscachesnapshot:encabulator-valkey-auto-0": list(CONTRACT),
}
PROFILES = {"encabulator-use2-node": [{"InstanceProfileName": "eks-us-east-2-encabulator-use2-abc123"}]}
PROFILE_TAGS = {"eks-us-east-2-encabulator-use2-abc123": [{"Key": "eks:eks-cluster-name", "Value": "encabulator-use2"}]}

# EKS access entries per (region, cluster). The cell's service-linked-role entry is the one EKS
# Auto Mode creates itself, without tags; the capability entries ACK created carry them.
CONTRACT_MAP = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
SLR = f"arn:aws:iam::{ACCOUNT}:role/aws-service-role/eks.amazonaws.com/AWSServiceRoleForAmazonEKS"
KRO = f"arn:aws:iam::{ACCOUNT}:role/encabulator-use2-kro"
ACCESS_ENTRIES = {
    ("us-east-2", "encabulator-use2"): {
        SLR: {"accessEntryArn": f"arn:aws:eks:us-east-2:{ACCOUNT}:access-entry/encabulator-use2/role/{ACCOUNT}/AWSServiceRoleForAmazonEKS/1",
              "tags": {}},
        KRO: {"accessEntryArn": f"arn:aws:eks:us-east-2:{ACCOUNT}:access-entry/encabulator-use2/role/{ACCOUNT}/encabulator-use2-kro/2",
              "tags": dict(CONTRACT_MAP)},
    },
}


class ClientError(Exception):
    def __init__(self, code):
        super().__init__(code)
        self.response = {"Error": {"Code": code}}


class Paginator:
    def __init__(self, fn):
        self.fn = fn

    def paginate(self, **params):
        yield self.fn(**params)


def matches(item, filters):
    for f in filters or []:
        name, values = f["Name"], f["Values"]
        if name == "vpc-id":
            if item.get("VpcId") not in values:
                return False
        elif name.startswith("tag:"):
            tags = sweep.tag_map(item.get("Tags") or item.get("TagSet"))
            if tags.get(name[4:]) not in values:
                return False
        else:
            raise AssertionError(f"filter the fake does not know: {name}")
    return True


class FakeEC2:
    KEYS = {"describe_vpcs": "Vpcs", "describe_route_tables": "RouteTables",
            "describe_network_acls": "NetworkAcls", "describe_security_groups": "SecurityGroups",
            "describe_network_interfaces": "NetworkInterfaces"}
    IDS = {"Vpcs": "VpcId", "RouteTables": "RouteTableId", "NetworkAcls": "NetworkAclId",
           "SecurityGroups": "GroupId", "NetworkInterfaces": "NetworkInterfaceId"}

    def __init__(self, world, calls, deny_create=False):
        self.world, self.calls, self.deny_create = world, calls, deny_create

    def get_paginator(self, op):
        key = self.KEYS[op]
        return Paginator(lambda Filters=None: {key: [copy.deepcopy(i) for i in self.world.get(key, [])
                                                    if matches(i, Filters)]})

    def create_tags(self, Resources, Tags):
        self.calls.append(("ec2.create_tags", tuple(Resources), tuple((t["Key"], t["Value"]) for t in Tags)))
        if self.deny_create:
            raise ClientError("UnauthorizedOperation")
        for key, id_field in self.IDS.items():
            for item in self.world.get(key, []):
                if item[id_field] in Resources:
                    field = "TagSet" if key == "NetworkInterfaces" else "Tags"
                    merged = {**sweep.tag_map(item.get(field)), **{t["Key"]: t["Value"] for t in Tags}}
                    item[field] = [{"Key": k, "Value": v} for k, v in merged.items()]


class FakeElastiCache:
    def __init__(self, region, calls, snapshots, tags):
        self.region, self.calls, self.snapshots, self.tags = region, calls, snapshots, tags

    def get_paginator(self, op):
        assert op == "describe_serverless_cache_snapshots", op

        def page(ServerlessCacheName):
            if self.region != "us-east-2" or ServerlessCacheName not in self.snapshots:
                raise ClientError("ServerlessCacheNotFoundFault")
            return {"ServerlessCacheSnapshots": self.snapshots[ServerlessCacheName]}
        return Paginator(page)

    def list_tags_for_resource(self, ResourceName):
        return {"TagList": self.tags[ResourceName]}

    def add_tags_to_resource(self, ResourceName, Tags):
        self.calls.append(("elasticache.add_tags_to_resource", ResourceName, tuple((t["Key"], t["Value"]) for t in Tags)))
        self.tags[ResourceName] = list(Tags)


class FakeIAM:
    def __init__(self, calls, profiles, tags):
        self.calls, self.profiles, self.tags = calls, profiles, tags

    def get_paginator(self, op):
        assert op == "list_instance_profiles_for_role", op

        def page(RoleName):
            if RoleName not in self.profiles:
                raise ClientError("NoSuchEntity")
            return {"InstanceProfiles": self.profiles[RoleName]}
        return Paginator(page)

    def list_instance_profile_tags(self, InstanceProfileName):
        return {"Tags": self.tags[InstanceProfileName]}

    def tag_instance_profile(self, InstanceProfileName, Tags):
        self.calls.append(("iam.tag_instance_profile", InstanceProfileName, tuple((t["Key"], t["Value"]) for t in Tags)))
        self.tags[InstanceProfileName] = list(Tags)


class FakeEKS:
    def __init__(self, region, calls, entries):
        self.region, self.calls, self.entries = region, calls, entries

    def get_paginator(self, op):
        assert op == "list_access_entries", op

        def page(clusterName):
            if (self.region, clusterName) not in self.entries:
                raise ClientError("ResourceNotFoundException")
            return {"accessEntries": list(self.entries[(self.region, clusterName)])}
        return Paginator(page)

    def describe_access_entry(self, clusterName, principalArn):
        return {"accessEntry": copy.deepcopy(self.entries[(self.region, clusterName)][principalArn])}

    def tag_resource(self, resourceArn, tags):
        self.calls.append(("eks.tag_resource", resourceArn, tuple(tags.items())))
        for entries in self.entries.values():
            for entry in entries.values():
                if entry["accessEntryArn"] == resourceArn:
                    entry["tags"] = {**entry["tags"], **tags}


class FakeSession:
    def __init__(self, worlds, deny_create=False):
        self.worlds = worlds
        self.calls = []
        self.deny_create = deny_create
        self.snapshot_tags = copy.deepcopy(SNAPSHOT_TAGS)
        self.profile_tags = copy.deepcopy(PROFILE_TAGS)
        self.access_entries = copy.deepcopy(ACCESS_ENTRIES)

    def client(self, service, region_name=None):
        if service == "ec2":
            return FakeEC2(self.worlds.setdefault(region_name, {}), self.calls, self.deny_create)
        if service == "elasticache":
            return FakeElastiCache(region_name, self.calls, SNAPSHOTS, self.snapshot_tags)
        if service == "iam":
            return FakeIAM(self.calls, PROFILES, self.profile_tags)
        if service == "eks":
            return FakeEKS(region_name, self.calls, self.access_entries)
        raise AssertionError(f"sweep.py must not use {service}")


CONTRACT_PAIRS = (("auto-delete", "no"), ("project", "encabulator"), ("talk", "opn315"))


def run(session, dry_run=False):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        status = sweep.Sweep(session, ["us-west-2", "us-east-2"], ["encabulator-valkey"],
                             ["encabulator-usw2-node", "encabulator-use2-node"], dry_run=dry_run,
                             cluster_names=["encabulator-usw2", "encabulator-use2"]).run()
    return status, [json.loads(line) for line in out.getvalue().splitlines()]


class SweepTest(unittest.TestCase):
    def setUp(self):
        self.session = FakeSession({"us-east-2": copy.deepcopy(CELL), "us-west-2": {"Vpcs": []}})

    def ec2_tagged(self):
        return {rid for call in self.session.calls if call[0] == "ec2.create_tags" for rid in call[1]}

    def test_tags_exactly_the_implicit_resources_of_the_cell(self):
        status, _ = run(self.session)
        self.assertEqual(status, 0)
        self.assertEqual(self.ec2_tagged(), EXPECTED_CELL)
        for call in self.session.calls:
            self.assertEqual(call[2], CONTRACT_PAIRS, call)

    def test_never_touches_another_vpc(self):
        run(self.session)
        self.assertFalse(self.ec2_tagged() & OUTSIDE)

    def test_second_run_is_a_no_op(self):
        run(self.session)
        first = len(self.session.calls)
        status, lines = run(self.session)
        self.assertEqual(status, 0)
        self.assertEqual(len(self.session.calls), first, self.session.calls[first:])
        self.assertEqual(lines[-1]["tagged"], 0)

    def test_untagged_snapshot_and_instance_profile(self):
        run(self.session)
        snaps = [c[1] for c in self.session.calls if c[0] == "elasticache.add_tags_to_resource"]
        self.assertEqual(snaps, [f"arn:aws:elasticache:us-east-2:{ACCOUNT}:serverlesscachesnapshot:encabulator-valkey-auto-1"])
        profiles = [c[1] for c in self.session.calls if c[0] == "iam.tag_instance_profile"]
        self.assertEqual(profiles, ["eks-us-east-2-encabulator-use2-abc123"])

    def test_untagged_access_entry_eks_created(self):
        # Only the entry EKS created untagged; the tagged one and the hub (absent from us-east-2,
        # and with no entries in this fake's us-west-2) are left alone.
        run(self.session)
        entries = [c[1] for c in self.session.calls if c[0] == "eks.tag_resource"]
        self.assertEqual(entries, [ACCESS_ENTRIES[("us-east-2", "encabulator-use2")][SLR]["accessEntryArn"]])

    def test_missing_cache_or_role_is_not_a_failure(self):
        # us-west-2 has no encabulator-valkey in this fake, encabulator-usw2-node has no profile.
        status, lines = run(self.session)
        self.assertEqual(status, 0)
        self.assertFalse([line for line in lines if line["level"] == "error"])

    def test_dry_run_changes_nothing(self):
        status, lines = run(self.session, dry_run=True)
        self.assertEqual(status, 0)
        self.assertEqual(self.session.calls, [])
        would = {line["resource"] for line in lines if line.get("action") == "would tag"}
        self.assertTrue(EXPECTED_CELL <= would)

    def test_denied_create_tags_fails_the_run(self):
        session = FakeSession({"us-east-2": copy.deepcopy(CELL)}, deny_create=True)
        status, lines = run(session)
        self.assertEqual(status, 1)
        self.assertTrue([line for line in lines if line["level"] == "error" and line["step"] == "create_tags"])

    def test_only_reads_and_the_three_tag_writes(self):
        run(self.session)
        self.assertEqual({c[0] for c in self.session.calls},
                         {"ec2.create_tags", "elasticache.add_tags_to_resource", "iam.tag_instance_profile",
                          "eks.tag_resource"})

    def test_hub_vpc_from_terraform_is_swept_too(self):
        # The hub: Terraform tags the default route table, SG and NACL and the cluster SG;
        # what is left is the requester-managed interfaces.
        hub = {
            "Vpcs": [{"VpcId": "vpc-0hub", "Tags": named("encabulator-usw2")}],
            "RouteTables": [{"RouteTableId": "rtb-0hubmain", "VpcId": "vpc-0hub", "Associations": [{"Main": True}],
                             "Tags": named("encabulator-usw2-default")}],
            "NetworkAcls": [{"NetworkAclId": "acl-0hub", "VpcId": "vpc-0hub", "IsDefault": True,
                             "Tags": named("encabulator-usw2-default")}],
            "SecurityGroups": [{"GroupId": "sg-0hubcluster", "GroupName": "eks-cluster-sg-encabulator-usw2-1",
                                "VpcId": "vpc-0hub", "Tags": list(CONTRACT)}],
            "NetworkInterfaces": [{"NetworkInterfaceId": "eni-0hubcache", "VpcId": "vpc-0hub", "RequesterManaged": True,
                                   "Description": "ElastiCache Serverless encabulator-valkey", "TagSet": []}],
        }
        session = FakeSession({"us-west-2": hub, "us-east-2": {"Vpcs": []}})
        run(session)
        self.assertEqual({rid for c in session.calls if c[0] == "ec2.create_tags" for rid in c[1]}, {"eni-0hubcache"})


if __name__ == "__main__":
    unittest.main(verbosity=2)
