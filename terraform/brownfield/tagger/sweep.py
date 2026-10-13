#!/usr/bin/env python3
"""Put the contract tags on what AWS creates implicitly for the Encabulator stacks.

Runs on the hub as the CronJob encabulator-tagger (tagger.tf), every 10 minutes. Everything
the talk creates on purpose is tagged when it is created (Terraform default_tags, ACK
spec.tags, the NodeClass, the StorageClass). This covers what AWS creates on its own,
without a tag request, in the hub and in every cell:

  in every VPC tagged project=encabulator and talk=opn315, in each region of REGIONS:
    route tables, network ACLs, security groups and network interfaces that lack a contract
    tag: the main route table, default network ACL and default security group EC2 creates
    with the VPC, the cluster security group EKS creates, and the requester-managed network
    interfaces of the EKS control plane, the NAT gateway, RDS and ElastiCache
  ElastiCache serverless snapshots of the caches in CACHE_NAMES (the daily snapshot)
  IAM instance profiles of the roles in NODE_ROLES (EKS Auto Mode creates one per NodeClass role)
  EKS access entries of the clusters in CLUSTER_NAMES that EKS created itself (Auto Mode adds one
    for the AWSServiceRoleForAmazonEKS service-linked role, untagged)

It only ever adds the three contract tags. It never removes a tag and never changes anything
else, and the IAM role it runs with can do nothing but that (tagger.tf). Re-running is a no-op
once everything is tagged.

Environment:
  REGIONS       space-separated regions, e.g. "us-west-2 us-east-2"
  CACHE_NAMES   space-separated serverless cache names, e.g. "encabulator-valkey"
  NODE_ROLES    space-separated IAM role names, e.g. "encabulator-usw2-node encabulator-use2-node"
  CLUSTER_NAMES space-separated EKS cluster names, e.g. "encabulator-usw2 encabulator-use2"
  DRY_RUN       "1" to print what would be tagged without tagging

Exit status: 0 when every call succeeded, 1 when any failed (the CronJob shows the failure;
the next run tries again).
"""
from __future__ import annotations

import json
import os
import sys

TAGS = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
# How the hub and the cells mark their VPCs (auto-delete is what this adds, so not part of it).
VPC_SELECTOR = {"project": "encabulator", "talk": "opn315"}
EC2_BATCH = 100


def log(**fields):
    print(json.dumps(fields, sort_keys=True), flush=True)


def tag_map(tags):
    """Tags as a dict, from an EC2, IAM or ElastiCache [{Key, Value}] list or an EKS map."""
    if isinstance(tags, dict):
        return dict(tags)
    return {t["Key"]: t.get("Value", "") for t in tags or []}


def missing(tags):
    """True when a contract tag is absent or has another value."""
    have = tag_map(tags)
    return any(have.get(k) != v for k, v in TAGS.items())


def aws_tags():
    return [{"Key": k, "Value": v} for k, v in TAGS.items()]


def pages(client, operation, key, **params):
    for page in client.get_paginator(operation).paginate(**params):
        yield from page.get(key, [])


class Sweep:
    def __init__(self, session, regions, cache_names, node_roles, dry_run=False, cluster_names=()):
        self.session = session
        self.regions = regions
        self.cache_names = cache_names
        self.node_roles = node_roles
        self.cluster_names = cluster_names
        self.dry_run = dry_run
        self.failures = 0
        self.tagged = []

    def fail(self, step, err, **where):
        self.failures += 1
        log(level="error", step=step, error=f"{type(err).__name__}: {err}", **where)

    # ---------------------------------------------------------------------------- EC2
    def vpcs(self, ec2):
        filters = [{"Name": f"tag:{k}", "Values": [v]} for k, v in VPC_SELECTOR.items()]
        return [v["VpcId"] for v in pages(ec2, "describe_vpcs", "Vpcs", Filters=filters)]

    def untagged_in_vpc(self, ec2, vpc):
        """(resource id, what it is) for every route table, NACL, SG and ENI missing a tag."""
        in_vpc = [{"Name": "vpc-id", "Values": [vpc]}]
        found = []
        for rt in pages(ec2, "describe_route_tables", "RouteTables", Filters=in_vpc):
            main = any(a.get("Main") for a in rt.get("Associations", []))
            if missing(rt.get("Tags")):
                found.append((rt["RouteTableId"], "main route table" if main else "route table"))
        for acl in pages(ec2, "describe_network_acls", "NetworkAcls", Filters=in_vpc):
            if missing(acl.get("Tags")):
                found.append((acl["NetworkAclId"], "default network ACL" if acl.get("IsDefault") else "network ACL"))
        for sg in pages(ec2, "describe_security_groups", "SecurityGroups", Filters=in_vpc):
            if missing(sg.get("Tags")):
                found.append((sg["GroupId"], f"security group {sg.get('GroupName', '')}".strip()))
        for eni in pages(ec2, "describe_network_interfaces", "NetworkInterfaces", Filters=in_vpc):
            if missing(eni.get("TagSet")):
                what = "network interface"
                if eni.get("RequesterManaged"):
                    what += f" (requester-managed: {eni.get('Description', '')})"
                found.append((eni["NetworkInterfaceId"], what))
        return found

    def sweep_ec2(self, region):
        ec2 = self.session.client("ec2", region_name=region)
        try:
            vpcs = self.vpcs(ec2)
        except Exception as err:  # noqa: BLE001 - report every failure, keep going
            self.fail("describe_vpcs", err, region=region)
            return
        for vpc in vpcs:
            try:
                found = self.untagged_in_vpc(ec2, vpc)
            except Exception as err:  # noqa: BLE001
                self.fail("describe", err, region=region, vpc=vpc)
                continue
            for i in range(0, len(found), EC2_BATCH):
                batch = found[i:i + EC2_BATCH]
                ids = [rid for rid, _ in batch]
                try:
                    if not self.dry_run:
                        ec2.create_tags(Resources=ids, Tags=aws_tags())
                except Exception as err:  # noqa: BLE001
                    self.fail("create_tags", err, region=region, vpc=vpc, resources=ids)
                    continue
                for rid, what in batch:
                    self.tagged.append(rid)
                    log(level="info", action="would tag" if self.dry_run else "tagged",
                        region=region, vpc=vpc, resource=rid, what=what)

    # -------------------------------------------------------------------- ElastiCache
    def sweep_cache_snapshots(self, region):
        if not self.cache_names:
            return
        ec = self.session.client("elasticache", region_name=region)
        for name in self.cache_names:
            try:
                snapshots = list(pages(ec, "describe_serverless_cache_snapshots", "ServerlessCacheSnapshots",
                                       ServerlessCacheName=name))
            except Exception as err:  # noqa: BLE001
                # No cache of that name in this region (yet) is normal for a cell being built.
                if getattr(err, "response", {}).get("Error", {}).get("Code") == "ServerlessCacheNotFoundFault":
                    continue
                self.fail("describe_serverless_cache_snapshots", err, region=region, cache=name)
                continue
            for snap in snapshots:
                arn = snap["ARN"]
                try:
                    tags = ec.list_tags_for_resource(ResourceName=arn).get("TagList", [])
                    if not missing(tags):
                        continue
                    if not self.dry_run:
                        ec.add_tags_to_resource(ResourceName=arn, Tags=aws_tags())
                except Exception as err:  # noqa: BLE001
                    self.fail("add_tags_to_resource", err, region=region, resource=arn)
                    continue
                self.tagged.append(arn)
                log(level="info", action="would tag" if self.dry_run else "tagged", region=region,
                    resource=arn, what="serverless cache snapshot")

    # ---------------------------------------------------------------------------- EKS
    def sweep_access_entries(self, region):
        if not self.cluster_names:
            return
        eks = self.session.client("eks", region_name=region)
        for name in self.cluster_names:
            try:
                principals = list(pages(eks, "list_access_entries", "accessEntries", clusterName=name))
            except Exception as err:  # noqa: BLE001
                # Each cluster lives in one region, and a cell's does not exist until it is built.
                if getattr(err, "response", {}).get("Error", {}).get("Code") == "ResourceNotFoundException":
                    continue
                self.fail("list_access_entries", err, region=region, cluster=name)
                continue
            for principal in principals:
                try:
                    entry = eks.describe_access_entry(clusterName=name, principalArn=principal)["accessEntry"]
                    if not missing(entry.get("tags")):
                        continue
                    if not self.dry_run:
                        eks.tag_resource(resourceArn=entry["accessEntryArn"], tags=dict(TAGS))
                except Exception as err:  # noqa: BLE001
                    self.fail("tag_resource", err, region=region, cluster=name, principal=principal)
                    continue
                self.tagged.append(entry["accessEntryArn"])
                log(level="info", action="would tag" if self.dry_run else "tagged", region=region,
                    resource=entry["accessEntryArn"], what=f"EKS access entry of {name}")

    # ---------------------------------------------------------------------------- IAM
    def sweep_instance_profiles(self):
        if not self.node_roles:
            return
        iam = self.session.client("iam")
        for role in self.node_roles:
            try:
                profiles = list(pages(iam, "list_instance_profiles_for_role", "InstanceProfiles", RoleName=role))
            except Exception as err:  # noqa: BLE001
                # A cell's node role does not exist until the cell is built.
                if getattr(err, "response", {}).get("Error", {}).get("Code") == "NoSuchEntity":
                    continue
                self.fail("list_instance_profiles_for_role", err, role=role)
                continue
            for profile in profiles:
                name = profile["InstanceProfileName"]
                try:
                    tags = iam.list_instance_profile_tags(InstanceProfileName=name).get("Tags", [])
                    if not missing(tags):
                        continue
                    if not self.dry_run:
                        iam.tag_instance_profile(InstanceProfileName=name, Tags=aws_tags())
                except Exception as err:  # noqa: BLE001
                    self.fail("tag_instance_profile", err, resource=name)
                    continue
                self.tagged.append(name)
                log(level="info", action="would tag" if self.dry_run else "tagged", resource=name,
                    what=f"instance profile of {role}")

    def run(self):
        for region in self.regions:
            self.sweep_ec2(region)
            self.sweep_cache_snapshots(region)
            self.sweep_access_entries(region)
        self.sweep_instance_profiles()
        log(level="info", action="done", tagged=len(self.tagged), failures=self.failures, dry_run=self.dry_run)
        return 1 if self.failures else 0


def words(name):
    return os.environ.get(name, "").split()


def main():
    regions = words("REGIONS")
    if not regions:
        log(level="error", error="REGIONS is empty")
        return 2
    import boto3  # in the image; imported here so the tests run without it

    return Sweep(boto3.session.Session(), regions, words("CACHE_NAMES"), words("NODE_ROLES"),
                 dry_run=os.environ.get("DRY_RUN") == "1", cluster_names=words("CLUSTER_NAMES")).run()


if __name__ == "__main__":
    sys.exit(main())
