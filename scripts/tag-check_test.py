#!/usr/bin/env python3
"""Offline tests for scripts/tag-check.py and scripts/tag-check-audit.py. No AWS calls.

  TERRAFORM="mise exec terraform@1.14.9 -- terraform" python3 scripts/tag-check_test.py

tag-check.py runs against the recorded responses in scripts/tag-check-fixtures/: the fully
tagged hub + cell must pass, the fixture with one untagged resource must fail and name it, and
removing auto-delete=no from any single resource in the tagged fixture must fail on exactly that
resource (so every kind the check covers is proven to fail, not just one).

tag-check-audit.py runs against this checkout and the hub Terraform's offline plan (needs
terraform; terraform/brownfield initialised), then against copies with one tag path removed.
"""
from __future__ import annotations

import copy
import importlib.util
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
CHECK = HERE / "tag-check.py"
AUDIT = HERE / "tag-check-audit.py"
FIXTURES = HERE / "tag-check-fixtures"
TERRAFORM = os.environ.get("TERRAFORM", "terraform")


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod  # dataclasses look the module up while it loads
    spec.loader.exec_module(mod)
    return mod


tc = load(CHECK, "tag_check")


def run(*args):
    proc = subprocess.run([sys.executable, *map(str, args)], capture_output=True, text=True)
    return proc.returncode, proc.stdout, proc.stderr


class MemorySource(tc.FixtureSource):
    def __init__(self, calls):
        self.calls = calls


def check_in_process(calls, stacks=(("us-west-2", "encabulator-usw2"), ("us-east-2", "encabulator-use2"))):
    report = tc.Report()
    src = MemorySource(calls)
    for region, name in stacks:
        tc.check_stack(src, report, region, name)
    return report


def tag_holders(node, path=()):
    """Every tag list ({Key, Value}) or EKS tag map holding auto-delete=no, with its path."""
    if isinstance(node, dict):
        for k, v in node.items():
            if k in ("Tags", "TagSet", "TagList") and isinstance(v, list) and {"Key": "auto-delete", "Value": "no"} in v:
                yield path + (k,)
            elif k == "tags" and isinstance(v, list) and {"Key": "auto-delete", "Value": "no"} in v:
                yield path + (k,)  # ECR
            elif k == "tags" and isinstance(v, dict) and v.get("auto-delete") == "no":
                yield path + (k,)
            else:
                yield from tag_holders(v, path + (k,))
    elif isinstance(node, list):
        for i, v in enumerate(node):
            yield from tag_holders(v, path + (i,))


# Most specific first: a subnet also names its VPC, an Elastic IP its network interface.
ID_FIELDS = ("AllocationId", "NatGatewayId", "NetworkInterfaceId", "InstanceId", "VolumeId", "LaunchTemplateId",
             "RouteTableId", "NetworkAclId", "GroupId", "InternetGatewayId", "SubnetId", "VpcId")


def owner(calls, path):
    """The object a tag holder belongs to, and its id (an EC2 id, or the call for the others)."""
    node = calls
    for p in path[:-1]:
        node = node[p]
    for field in ID_FIELDS:
        if field in node:
            return node[field]
    return path[:-1]


def drop_auto_delete(calls, paths):
    out = copy.deepcopy(calls)
    for path in paths:
        node = out
        for p in path[:-1]:
            node = node[p]
        holder = node[path[-1]]
        if isinstance(holder, list):
            node[path[-1]] = [t for t in holder if t.get("Key") != "auto-delete"]
        else:
            holder.pop("auto-delete")
    return out


class TagCheckTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tagged = tc.FixtureSource(FIXTURES / "tagged.json").calls

    def test_fully_tagged_stacks_pass(self):
        status, out, err = run(CHECK, "--fixtures", FIXTURES / "tagged.json")
        self.assertEqual(status, 0, out + err)
        self.assertNotIn("MISSING", out)
        self.assertIn("carry auto-delete=no", err)

    def test_one_untagged_resource_fails_and_is_named(self):
        status, out, err = run(CHECK, "--fixtures", FIXTURES / "missing-tag.json")
        self.assertEqual(status, 1, out + err)
        missing = [line for line in out.splitlines() if line.startswith("MISSING")]
        self.assertEqual(len(missing), 1, missing)
        self.assertIn("us-east-2", missing[0])
        self.assertIn("ec2 security-group", missing[0])
        self.assertIn("(default)", missing[0])

    def test_every_resource_in_the_fixture_fails_without_its_tag(self):
        # One resource can appear in two responses (a volume attached to a node also carries
        # eks:eks-cluster-name): its tags are dropped from both, as AWS would show them.
        kinds = set()
        resources = {}
        for path in tag_holders(self.tagged):
            resources.setdefault(owner(self.tagged, path), []).append(path)
        self.assertGreater(len(resources), 90)
        for rid, paths in resources.items():
            with self.subTest(resource=str(rid)[:120]):
                report = check_in_process(drop_auto_delete(self.tagged, paths))
                missing = [line for line in report.lines if line.startswith("MISSING")]
                self.assertEqual(report.bad, 1, f"{rid}: {missing}")
                kinds.add(re.split(r"\s{2,}", missing[0])[2])
        required = {
            "ec2 vpc", "ec2 subnet", "ec2 route-table", "ec2 network-acl", "ec2 security-group",
            "ec2 internet-gateway", "ec2 natgateway", "ec2 elastic-ip", "ec2 network-interface",
            "ec2 instance", "ec2 volume", "rds db-instance", "elasticache serverless-cache",
            "eks cluster", "eks capability", "eks access-entry", "iam role", "ecr repository",
        }
        self.assertTrue(required <= kinds, f"kinds never proven to fail: {sorted(required - kinds)}")

    def test_resources_outside_the_stacks_are_ignored(self):
        # The fixture holds an untagged foreign Elastic IP, RDS instance and subnet group, cache,
        # roles (Admin, encabulator-usw2), ECR repository and a deleted NAT gateway; the tagged
        # run passes anyway.
        report = check_in_process(self.tagged)
        self.assertEqual(report.bad, 0, [line for line in report.lines if not line.startswith("ok")])
        text = "\n".join(report.lines)
        for foreign in ("eipalloc-000000000000ffff", "unrelated-db", "unrelated-cache", "nat-0000000000000dead", " Admin\n",
                        "other-team/unrelated-app"):
            self.assertNotIn(foreign, text + "\n")

    def test_ecr_repositories_are_counted_once_per_region(self):
        report = check_in_process(self.tagged, stacks=(("us-west-2", "encabulator-usw2"), ("us-west-2", "encabulator-usw2")))
        repos = [line for line in report.lines if " ecr repository " in line]
        self.assertEqual(len(repos), 2, repos)

    def test_missing_stack_fails(self):
        calls = copy.deepcopy(self.tagged)
        calls[tc.key("ec2", "describe-vpcs", "us-east-2", ["--filters", "Name=tag:Name,Values=encabulator-use2"])] = {"Vpcs": []}
        calls[tc.key("eks", "describe-cluster", "us-east-2", ["--name", "encabulator-use2"])] = {"__error__": "ResourceNotFoundException"}
        report = check_in_process(calls)
        problems = [line for line in report.lines if "stack" in line and line.startswith("MISSING")]
        self.assertEqual(len(problems), 2, report.lines)

    def test_only_read_only_calls(self):
        ops = re.findall(r'call\("(?:ec2|eks|rds|elasticache|iam|ecr)", "([a-z-]+)"', CHECK.read_text())
        self.assertGreater(len(ops), 20)
        self.assertEqual([op for op in ops if not op.startswith(tc.READ_ONLY)], [])
        with self.assertRaises(SystemExit):
            MemorySource({}).call("ec2", "create-tags", "us-east-2")
        with self.assertRaises(SystemExit):
            tc.LiveSource("false").call("ec2", "delete-vpc", "us-east-2")

    def test_fixtures_hold_no_real_account(self):
        for path in FIXTURES.glob("*.json"):
            # 12 digits not inside a hex resource id (vpc-0..., eipalloc-0...).
            accounts = set(re.findall(r"(?<![0-9a-f])[0-9]{12}(?![0-9a-f])", path.read_text()))
            self.assertLessEqual(accounts, {"111122223333"}, path)


@unittest.skipUnless(shutil.which(TERRAFORM.split()[0]), f"{TERRAFORM.split()[0]} not found (set TERRAFORM)")
class AuditTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = Path(tempfile.mkdtemp(prefix="tag-check-audit-"))
        cls.plan = cls.tmp / "plan.txt"
        status, out, err = run(AUDIT, "--terraform", TERRAFORM)
        cls.repo_status, cls.repo_out, cls.repo_err = status, out, err
        # Save the plan once for the mutation tests (same command the audit runs).
        audit = load(AUDIT, "tag_check_audit")
        cls.plan.write_text(audit.terraform_plan(TERRAFORM))

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tmp)

    def copy_repo(self):
        dest = Path(tempfile.mkdtemp(prefix="repo-", dir=self.tmp))
        for rel in ("gitops", "charts", "platform"):
            shutil.copytree(REPO / rel, dest / rel)
        tf = dest / "terraform" / "brownfield"
        tf.mkdir(parents=True)
        shutil.copy(REPO / "terraform" / "brownfield" / "nodes.tf", tf / "nodes.tf")
        shutil.copytree(REPO / "terraform" / "brownfield" / "charts", tf / "charts")
        return dest

    def audit(self, repo=None, plan=None):
        return run(AUDIT, "--repo", repo or REPO, "--tf-plan", plan or self.plan)

    def edit(self, path, old, new):
        text = path.read_text()
        self.assertIn(old, text, path)
        path.write_text(text.replace(old, new, 1))

    def assert_fails_on(self, status, out, needle):
        self.assertEqual(status, 1, out)
        fails = [line for line in out.splitlines() if line.startswith("FAIL")]
        self.assertTrue(any(needle in line for line in fails), f"no FAIL mentioning {needle!r}: {fails}")

    def test_this_checkout_has_a_tag_path_for_everything(self):
        self.assertEqual(self.repo_status, 0, self.repo_out + self.repo_err)
        self.assertNotIn("FAIL", self.repo_out)
        for must in ("module.vpc.aws_default_route_table.default", "module.vpc.aws_default_security_group.this",
                     "module.vpc.aws_default_network_acl.this", "EKS cluster security group",
                     "main route table, default security group, default network ACL",
                     "EBS volumes for PVCs of StorageClass `encabulator-gp3`"):
            self.assertIn(must, self.repo_out)

    def test_doc_is_up_to_date(self):
        status, out, err = run(AUDIT, "--repo", REPO, "--tf-plan", self.plan, "--check-doc", REPO / "docs" / "TAGGING.md")
        self.assertEqual(status, 0, err)

    def test_cell_storageclass_without_tags_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/app/rgds/encabulator.yaml", '          tagSpecification_1: "auto-delete=no"\n', "")
        self.assert_fails_on(*self.audit(repo)[:2], "StorageClass `encabulator-gp3`")

    def test_cell_ack_resource_without_tags_fails(self):
        repo = self.copy_repo()
        rgd = repo / "gitops/hub/rgds/encabulator-cell.yaml"
        text = rgd.read_text()
        start = text.index("kind: InternetGateway")
        cut = text.index('            - { key: auto-delete, value: "no" }\n', start)
        rgd.write_text(text[:cut] + text[cut:].replace('            - { key: auto-delete, value: "no" }\n', "", 1))
        self.assert_fails_on(*self.audit(repo)[:2], "InternetGateway")

    def test_hub_ack_manifest_without_tags_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/hub/brownfield/valkey.yaml", '    - key: auto-delete\n      value: "no"\n', "")
        self.assert_fails_on(*self.audit(repo)[:2], "ServerlessCache `encabulator-valkey`")

    def test_cell_db_without_copy_tags_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/app/rgds/encabulator.yaml", "copyTagsToSnapshot: true", "copyTagsToSnapshot: false")
        self.assert_fails_on(*self.audit(repo)[:2], "RDS automated backups")

    def test_cell_creator_access_entry_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/hub/rgds/encabulator-cell.yaml", "bootstrapClusterCreatorAdminPermissions: false",
                  "bootstrapClusterCreatorAdminPermissions: true")
        self.assert_fails_on(*self.audit(repo)[:2], "cluster creator")

    def test_nodeclass_without_tags_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/app/nodes/auto-mode.yaml", '    auto-delete: "no"\n', "")
        self.assert_fails_on(*self.audit(repo)[:2], "NodeClass `encabulator`")

    def test_load_balancer_service_fails(self):
        repo = self.copy_repo()
        self.edit(repo / "gitops/app/rgds/encabulator.yaml", "          type: ClusterIP\n          clusterIP: None\n",
                  "          type: LoadBalancer\n")
        self.assert_fails_on(*self.audit(repo)[:2], "load balancers")

    def test_tagger_not_sweeping_the_cell_region_fails(self):
        plan = self.tmp / "plan-no-use2.txt"
        text = self.plan.read_text()
        self.assertIn('value = "us-west-2 us-east-2"', re.sub(r"\s+=", " =", text))
        plan.write_text(re.sub(r'(\+ value\s+= )"us-west-2 us-east-2"', r'\1"us-west-2"', text))
        status, out, _ = self.audit(plan=plan)
        self.assert_fails_on(status, out, "main route table, default security group, default network ACL")

    def test_hub_defaults_neither_adopted_nor_swept_fails(self):
        plan = self.tmp / "plan-no-defaults.txt"
        text = self.plan.read_text()
        for address in ("module.vpc.aws_default_route_table.default[0]", "module.vpc.aws_default_security_group.this[0]",
                        "module.vpc.aws_default_network_acl.this[0]"):
            start = text.index(f"  # {address} will be created")
            end = text.index("\n    }\n", start) + len("\n    }\n")
            text = text[:start] + text[end:]
        text = re.sub(r'(\+ value\s+= )"us-west-2 us-east-2"', r'\1"us-east-2"', text)
        plan.write_text(text)
        status, out, _ = self.audit(plan=plan)
        self.assertEqual(status, 1, out)
        hub = [line for line in out.splitlines() if line.startswith("FAIL") and "hub (Terraform" in line]
        self.assertTrue(any("main route table, default security group, default network ACL" in line for line in hub), hub)

    def test_planned_resource_without_auto_delete_fails(self):
        plan = self.tmp / "plan-untagged-sg.txt"
        text = self.plan.read_text()
        block = text.index("  # aws_security_group.db will be created")
        end = text.index("\n    }\n", block)
        plan.write_text(text[:block] + text[block:end].replace('"auto-delete" = "no"', '"auto-delete" = "yes"') + text[end:])
        self.assert_fails_on(*self.audit(plan=plan)[:2], "aws_security_group.db")

    def test_unclassified_untaggable_type_fails(self):
        plan = self.tmp / "plan-new-type.txt"
        extra = ('  # aws_route53_record.new will be created\n  + resource "aws_route53_record" "new" {\n'
                 '      + name = "x"\n    }\n\n')
        text = self.plan.read_text()
        i = text.index("  # aws_security_group.db will be created")
        plan.write_text(text[:i] + extra + text[i:])
        self.assert_fails_on(*self.audit(plan=plan)[:2], "aws_route53_record")


if __name__ == "__main__":
    unittest.main(verbosity=2)
