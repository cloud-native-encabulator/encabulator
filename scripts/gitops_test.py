#!/usr/bin/env python3
"""Offline tests for scripts/gitops.sh, scripts/gitops-settings.sh,
gitops/hub/bootstrap/register-hub.sh and their Makefile targets. No AWS, no cluster, no network.

  python3 scripts/gitops_test.py

aws, kubectl and git are stand-ins on PATH (one small program, below). Each records every call
(argv and stdin) and answers from a JSON "world" the test writes, so a test can say what the
account and the hub look like and then read exactly what the script asked for.
"""
from __future__ import annotations

import base64
import json
import os
import re
import shutil
import stat
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
GITOPS = REPO / "scripts" / "gitops.sh"
REGISTER = REPO / "gitops" / "hub" / "bootstrap" / "register-hub.sh"
BOOTSTRAP = REPO / "gitops" / "hub" / "bootstrap"
DEFAULT_URL = "https://github.com/cloud-native-encabulator/encabulator"
CUSTOM_URL = "https://git.example.com/example-org/encabulator-fork.git"
REGISTRY = "111122223333.dkr.ecr.us-west-2.amazonaws.com/encabulator"
HUB_ENDPOINT = "https://EXAMPLE.gr7.us-west-2.eks.amazonaws.com"

# One stand-in for aws, kubectl and git; it looks at its own name.
STUB = r'''#!/usr/bin/env python3
import json, os, sys
tool = os.path.basename(sys.argv[0])
args = sys.argv[1:]
stdin = "" if sys.stdin.isatty() else sys.stdin.read()
world = json.load(open(os.environ["STUB_WORLD"]))
with open(os.environ["STUB_LOG"], "a") as f:
    f.write(json.dumps({"tool": tool, "args": args, "stdin": stdin}) + "\n")

def opt(name):
    return args[args.index(name) + 1] if name in args else None

def out(s):
    sys.stdout.write(s + "\n"); sys.exit(0)

def fail(msg, code=254):
    sys.stderr.write(msg + "\n"); sys.exit(code)

if tool == "aws":
    service, op = args[0], args[1]
    q = opt("--query")
    if (service, op) == ("eks", "describe-cluster"):
        c = world["clusters"].get(opt("--name"))
        if not c:
            fail("ResourceNotFoundException: cluster not found")
        out({"cluster.arn": c["arn"], "cluster.endpoint": c["endpoint"], "cluster.status": c["status"],
             "cluster.resourcesVpcConfig.vpcId": c["vpc"]}[q])
    if (service, op) == ("rds", "describe-db-subnet-groups"):
        out(" ".join(world["db_subnets"]))
    if (service, op) == ("ec2", "describe-security-groups"):
        out(world["db_sg"])
    if (service, op) == ("elasticache", "describe-serverless-caches"):
        out(" ".join(world["cache_subnets"]) if "SubnetIds" in q else world["cache_sg"])
    if (service, op) == ("sts", "get-caller-identity"):
        out(world["caller"])
    if (service, op) == ("iam", "get-role"):
        arn = world.get("roles", {}).get(opt("--role-name"))
        out(arn) if arn else fail("AccessDenied")
    if (service, op) == ("eks", "describe-access-entry"):
        e = world.get("access_entries", {}).get(opt("--principal-arn"))
        out(e) if e else fail("ResourceNotFoundException: no access entry")
    if (service, op) in (("eks", "create-access-entry"), ("eks", "associate-access-policy"),
                         ("eks", "tag-resource")):
        out("{}")
    fail(f"stub aws: unexpected call {args}")

if tool == "kubectl":
    if args[:2] == ["config", "view"]:
        out(world["kubectl_server"])
    if "get" in args and "secret" in args:
        name = args[args.index("secret") + 1]
        s = world.get("secrets", {}).get(name)
        if not s:
            fail(f'Error from server (NotFound): secrets "{name}" not found', 1)
        jp = opt("-o") or ""
        if "jsonpath" in jp:
            out(s.get("labels", {}).get("encabulator.io/hub", ""))
        out("secret/" + name)
    if "apply" in args:
        if "--dry-run=server" in args:
            out(stdin.rstrip("\n"))
        out("applied")
    fail(f"stub kubectl: unexpected call {args}")

if tool == "git":
    if "fetch" in args:
        url, rev = args[-2], args[-1]
        rev_files = world.get("remote", {}).get(url, {}).get(rev)
        if rev_files is None:
            fail(f"fatal: couldn't find remote ref {rev}", 128)
        json.dump(rev_files, open(os.environ["STUB_LOG"] + ".fetch_head", "w"))
        sys.exit(0)
    if "cat-file" in args:
        path = args[-1].split(":", 1)[1]
        files = json.load(open(os.environ["STUB_LOG"] + ".fetch_head"))
        sys.exit(0 if path in files else 1)
    fail(f"stub git: unexpected call {args}")
fail("stub: unknown tool " + tool)
'''


def hub_world(**extra):
    w = {
        "clusters": {
            "encabulator-usw2": {"arn": "arn:aws:eks:us-west-2:111122223333:cluster/encabulator-usw2",
                                 "endpoint": HUB_ENDPOINT, "status": "ACTIVE", "vpc": "vpc-0123456789abcdef0"},
            "encabulator-use2": {"arn": "arn:aws:eks:us-east-2:111122223333:cluster/encabulator-use2",
                                 "endpoint": "https://EXAMPLE2.gr7.us-east-2.eks.amazonaws.com",
                                 "status": "ACTIVE", "vpc": "vpc-0123456789abcdef1"},
        },
        "kubectl_server": HUB_ENDPOINT,
        "db_subnets": ["subnet-0bbbbbbbbbbbbbbb2", "subnet-0aaaaaaaaaaaaaaa1"],
        "cache_subnets": ["subnet-0aaaaaaaaaaaaaaa1", "subnet-0bbbbbbbbbbbbbbb2"],
        "db_sg": "sg-0123456789abcdef0",
        "cache_sg": "sg-0123456789abcdef0",
        "secrets": {"encabulator-usw2": {"labels": {"encabulator.io/hub": "true"}}},
        "caller": "arn:aws:sts::111122223333:assumed-role/ExampleAdmin/someone",
        "roles": {"ExampleAdmin": "arn:aws:iam::111122223333:role/team/ExampleAdmin"},
        "remote": {},
    }
    w.update(extra)
    return w


class Sandbox:
    """A temp dir with the stand-ins on PATH, a world file and a call log."""

    def __init__(self, world):
        self.dir = Path(tempfile.mkdtemp(prefix="gitops-test-"))
        self.bin = self.dir / "bin"
        self.bin.mkdir()
        for tool in ("aws", "kubectl", "git"):
            p = self.bin / tool
            p.write_text(STUB)
            p.chmod(0o755)
        self.world = self.dir / "world.json"
        self.log = self.dir / "calls.jsonl"
        self.set_world(world)

    def set_world(self, world):
        self.world.write_text(json.dumps(world))

    def env(self, **settings):
        env = {k: v for k, v in os.environ.items()
               if k not in ("GIT_REPO_URL", "GIT_REVISION", "REGISTRY", "GIT_TOKEN_FILE", "CELL",
                            "PRINCIPAL_ARN", "KUBECONFIG", "AWS_PROFILE", "MAKEFLAGS", "MFLAGS",
                            "AWS", "KUBECTL", "GIT")}
        env.update(PATH=f"{self.bin}:{os.environ['PATH']}", STUB_WORLD=str(self.world),
                   STUB_LOG=str(self.log), NO_COLOR="1")
        env.update(settings)
        return env

    def run(self, *cmd, **settings):
        self.log.write_text("")
        p = subprocess.run([str(c) for c in cmd], capture_output=True, text=True, cwd=REPO,
                           env=self.env(**settings), stdin=subprocess.DEVNULL)
        return p.returncode, p.stdout, p.stderr

    def calls(self, tool=None):
        lines = [json.loads(x) for x in self.log.read_text().splitlines() if x]
        return [c for c in lines if tool is None or c["tool"] == tool]

    def close(self):
        shutil.rmtree(self.dir)


def docs(text):
    """Split a rendered stream into (kind, name) pairs, in order."""
    found = []
    for doc in re.split(r"^---$", text, flags=re.M):
        kind = re.search(r"^kind: (\S+)", doc, re.M)
        name = re.search(r"^  name: (\S+)", doc, re.M)
        if kind:
            found.append((kind.group(1), name.group(1)))
    return found


class RenderTest(unittest.TestCase):
    def setUp(self):
        self.sb = Sandbox(hub_world())

    def tearDown(self):
        self.sb.close()

    def test_default_render_is_the_committed_files(self):
        status, out, err = self.sb.run(GITOPS, "render", "all")
        self.assertEqual(status, 0, err)
        files = ["projects.yaml", "hub-rgds.yaml", "hub-cell-apps.yaml", "hub-brownfield.yaml", "hub-cells.yaml"]
        want = "---\n".join((BOOTSTRAP / f).read_text() for f in files)
        self.assertEqual(out, want)

    def test_custom_render_replaces_every_default(self):
        status, out, err = self.sb.run(GITOPS, "render", "all", GIT_REPO_URL=CUSTOM_URL,
                                       GIT_REVISION="1.10", REGISTRY=REGISTRY,
                                       GIT_TOKEN_FILE="/nonexistent/token")
        self.assertEqual(status, 0, err)
        self.assertNotIn(DEFAULT_URL, out)
        self.assertNotIn("targetRevision: main", out)
        self.assertEqual(out.count(CUSTOM_URL), 7)
        self.assertEqual(out.count('targetRevision: "1.10"'), 5)
        # The registry and the token file are not part of the bootstrap files.
        self.assertNotIn(REGISTRY, out)
        self.assertNotIn("token", out)

    def test_bootstrap_render_leaves_out_hub_cells(self):
        status, out, _ = self.sb.run(GITOPS, "render", "bootstrap")
        self.assertEqual(status, 0)
        self.assertEqual(docs(out), [("AppProject", "encabulator-hub"), ("AppProject", "encabulator-cells"),
                                     ("Application", "hub-rgds"), ("Application", "hub-cell-apps"),
                                     ("ApplicationSet", "hub-brownfield")])
        status, out, _ = self.sb.run(GITOPS, "render", "cells")
        self.assertEqual(docs(out), [("Application", "hub-cells")])

    def test_invalid_settings_are_refused(self):
        for settings, word in [({"GIT_REPO_URL": "https://example.com/a b"}, "GIT_REPO_URL"),
                               ({"GIT_REPO_URL": 'https://example.com/"x'}, "GIT_REPO_URL"),
                               ({"GIT_REPO_URL": "http://example.com/x"}, "GIT_REPO_URL"),
                               ({"GIT_REVISION": "release..x"}, "GIT_REVISION"),
                               ({"GIT_REVISION": "-x"}, "GIT_REVISION"),
                               ({"GIT_REVISION": "a: b"}, "GIT_REVISION"),
                               ({"REGISTRY": "Example.com/X"}, "REGISTRY")]:
            with self.subTest(settings=settings):
                status, out, err = self.sb.run(GITOPS, "render", **settings)
                self.assertEqual(status, 2, out + err)
                self.assertIn(word, err)
                self.assertEqual(out, "")

    def test_accepted_url_shapes(self):
        for url in ("https://github.com/example/repo", "https://github.com/example/repo.git",
                    "ssh://git@example.com/x/y.git", "git@github.com:example/repo.git"):
            with self.subTest(url=url):
                status, out, err = self.sb.run(GITOPS, "render", GIT_REPO_URL=url)
                self.assertEqual(status, 0, err)
                self.assertEqual(out.count(url), 7)


class RenderCheckTest(unittest.TestCase):
    """The check passes on this checkout and fails on a copy with one deliberate drift."""

    def copy(self):
        d = Path(tempfile.mkdtemp(prefix="gitops-check-"))
        self.addCleanup(shutil.rmtree, d)
        for sub in ("scripts", "gitops/hub"):
            shutil.copytree(REPO / sub, d / sub)
        return d

    def check(self, root):
        p = subprocess.run([str(root / "scripts" / "gitops.sh"), "check"], capture_output=True, text=True,
                           stdin=subprocess.DEVNULL,
                           env={k: v for k, v in os.environ.items() if not k.startswith("GIT_")})
        return p.returncode, p.stderr

    def test_passes_here(self):
        status, err = self.check(REPO)
        self.assertEqual(status, 0, err)
        self.assertIn("total: repository URL 7, targetRevision 5", err)

    def test_fails_on_a_hard_coded_revision_in_a_bootstrap_file(self):
        root = self.copy()
        f = root / "gitops/hub/bootstrap/hub-rgds.yaml"
        f.write_text(f.read_text().replace("targetRevision: main", "targetRevision: release", 1))
        status, err = self.check(root)
        self.assertEqual(status, 1, err)
        self.assertIn("hub-rgds.yaml: repository URL 2 (want 2), targetRevision 1 (want 2)", err)

    def test_fails_on_a_new_unrendered_repository_url(self):
        root = self.copy()
        f = root / "gitops/hub/bootstrap/hub-cells.yaml"
        f.write_text(f.read_text().replace(DEFAULT_URL, "https://github.com/Cloud-Native-Encabulator/Encabulator"))
        status, err = self.check(root)
        self.assertEqual(status, 1, err)
        self.assertIn("hub-cells.yaml: repository URL 0 (want 1)", err)

    def test_fails_on_a_bootstrap_file_the_render_does_not_know(self):
        root = self.copy()
        shutil.copy(root / "gitops/hub/bootstrap/hub-cells.yaml", root / "gitops/hub/bootstrap/hub-extra.yaml")
        status, err = self.check(root)
        self.assertEqual(status, 1, err)
        self.assertIn("hub-extra.yaml is not rendered by scripts/gitops.sh", err)

    def test_fails_when_appset_names_the_repository_directly(self):
        root = self.copy()
        f = root / "gitops/hub/appset.yaml"
        text = f.read_text()
        f.write_text(text.replace(
            "targetRevision: '{{ index .metadata.annotations \"encabulator.io/revision\" | default \"main\" }}'",
            "targetRevision: main", 1))
        status, err = self.check(root)
        self.assertEqual(status, 1, err)
        self.assertIn("revision 1 (want 2)", err)
        self.assertIn("sets this repository or revision directly", err)


class RegisterHubTest(unittest.TestCase):
    def setUp(self):
        self.sb = Sandbox(hub_world())

    def tearDown(self):
        self.sb.close()

    def manifest(self):
        applies = [c for c in self.sb.calls("kubectl") if "apply" in c["args"]]
        self.assertEqual(len(applies), 1)
        return applies[0]["stdin"]

    def annotations(self, text):
        block = text.split("\n  annotations:\n", 1)[1]
        lines = []
        for line in block.splitlines():
            if not line.startswith("    "):
                break
            lines.append(line)
        return dict(re.findall(r"^    (encabulator\.io/[a-z-]+): (.*)$", "\n".join(lines), re.M))

    def test_without_settings_the_secret_is_as_before(self):
        status, out, err = self.sb.run(REGISTER, "--dry-run")
        self.assertEqual(status, 0, err)
        self.assertEqual(self.annotations(self.manifest()), {
            "encabulator.io/region": "us-west-2",
            "encabulator.io/subnet-ids": "subnet-0aaaaaaaaaaaaaaa1,subnet-0bbbbbbbbbbbbbbb2",
            "encabulator.io/security-group-id": "sg-0123456789abcdef0"})
        self.assertIn("--dry-run=server", self.sb.calls("kubectl")[-1]["args"])
        # --dry-run prints the Secret the API server accepted.
        self.assertIn("encabulator.io/subnet-ids", out)

    def test_settings_are_recorded_next_to_the_subnet_ids(self):
        status, out, err = self.sb.run(REGISTER, "--dry-run", GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release",
                                       REGISTRY=REGISTRY)
        self.assertEqual(status, 0, err)
        ann = self.annotations(self.manifest())
        self.assertEqual(ann["encabulator.io/repo-url"], CUSTOM_URL)
        self.assertEqual(ann["encabulator.io/revision"], '"release"')
        self.assertEqual(ann["encabulator.io/registry"], REGISTRY)
        self.assertEqual(len(ann), 6)
        for v in (CUSTOM_URL, "release", REGISTRY):
            self.assertIn(v, out)

    def test_only_settings_that_differ_from_the_default_are_recorded(self):
        status, _, err = self.sb.run(REGISTER, "--dry-run", GIT_REPO_URL=DEFAULT_URL, GIT_REVISION="main",
                                     REGISTRY="ghcr.io/cloud-native-encabulator/encabulator")
        self.assertEqual(status, 0, err)
        self.assertEqual(len(self.annotations(self.manifest())), 3)
        status, _, err = self.sb.run(REGISTER, "--dry-run", GIT_REVISION="release")
        self.assertEqual(set(self.annotations(self.manifest())) - {"encabulator.io/region",
                         "encabulator.io/subnet-ids", "encabulator.io/security-group-id"},
                         {"encabulator.io/revision"})

    def test_a_real_run_applies_and_says_where_cells_sync_from(self):
        status, out, err = self.sb.run(REGISTER, GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release")
        self.assertEqual(status, 0, err)
        self.assertNotIn("--dry-run=server", self.sb.calls("kubectl")[-1]["args"])
        self.assertIn(f"Cells sync from {CUSTOM_URL} at release.", out)

    def test_existing_checks_still_refuse(self):
        w = hub_world()
        w["kubectl_server"] = "https://some-other-cluster.example.com"
        self.sb.set_world(w)
        status, _, err = self.sb.run(REGISTER, "--dry-run")
        self.assertEqual(status, 1)
        self.assertIn("kubectl points at", err)
        w = hub_world(cache_subnets=["subnet-0ccccccccccccccc3", "subnet-0aaaaaaaaaaaaaaa1"])
        self.sb.set_world(w)
        status, _, err = self.sb.run(REGISTER, "--dry-run")
        self.assertEqual(status, 1)
        self.assertIn("subnets", err)
        self.assertEqual([c for c in self.sb.calls("kubectl") if "apply" in c["args"]], [])

    def test_invalid_setting_stops_before_any_call(self):
        status, _, err = self.sb.run(REGISTER, "--dry-run", REGISTRY="not a registry")
        self.assertEqual(status, 2)
        self.assertIn("REGISTRY", err)
        self.assertEqual(self.sb.calls(), [])


class BootstrapAndCellsTest(unittest.TestCase):
    def setUp(self):
        self.sb = Sandbox(hub_world())

    def tearDown(self):
        self.sb.close()

    def applied(self):
        return [c["stdin"] for c in self.sb.calls("kubectl") if "apply" in c["args"]]

    def test_bootstrap_applies_everything_but_hub_cells(self):
        status, _, err = self.sb.run(GITOPS, "bootstrap", GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release")
        self.assertEqual(status, 0, err)
        (stream,) = self.applied()
        self.assertNotIn(("Application", "hub-cells"), docs(stream))
        self.assertEqual(len(docs(stream)), 5)
        self.assertEqual(stream.count(CUSTOM_URL), 6)
        self.assertIn("cells are off", err)
        self.assertIn("make add-cell CELL=<cell>", err)

    def test_bootstrap_needs_a_registered_hub(self):
        self.sb.set_world(hub_world(secrets={}))
        status, _, err = self.sb.run(GITOPS, "bootstrap")
        self.assertEqual(status, 1)
        self.assertIn("make gitops-register", err)
        self.assertEqual(self.applied(), [])

    def remote(self, files):
        return {CUSTOM_URL: {"release": files}}

    def test_add_cell_applies_hub_cells_when_the_file_is_in_the_revision(self):
        self.sb.set_world(hub_world(remote=self.remote(["gitops/hub/cells/use2.yaml"])))
        for attempt in (1, 2):  # applying it twice is the same apply
            status, _, err = self.sb.run(GITOPS, "add-cell", CELL="use2", GIT_REPO_URL=CUSTOM_URL,
                                         GIT_REVISION="release")
            self.assertEqual(status, 0, err)
            (stream,) = self.applied()
            self.assertEqual(docs(stream), [("Application", "hub-cells")])
            self.assertIn(f"repoURL: {CUSTOM_URL}", stream)
            self.assertIn('targetRevision: "release"', stream)
            self.assertIn("kubectl get encabulatorcells -n encabulator-cells -w", err)
            self.assertIn("about 20 minutes", err)
            self.assertIn("encabulator-use2 in us-east-2", err)
            fetch = [c for c in self.sb.calls("git") if "fetch" in c["args"]][0]
            self.assertEqual(fetch["args"][-2:], [CUSTOM_URL, "release"])

    def test_add_cell_refuses_a_cell_not_in_the_revision(self):
        self.sb.set_world(hub_world(remote=self.remote(["gitops/hub/cells/other.yaml"])))
        status, _, err = self.sb.run(GITOPS, "add-cell", CELL="use2", GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release")
        self.assertEqual(status, 1)
        self.assertIn("gitops/hub/cells/use2.yaml is not in", err)
        self.assertIn("commit and push it there first", err)
        self.assertEqual(self.applied(), [])

    def test_add_cell_falls_back_to_the_checkout_when_the_remote_is_unreadable(self):
        status, _, err = self.sb.run(GITOPS, "add-cell", CELL="use2", GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release")
        self.assertEqual(status, 0, err)
        self.assertIn("could not read release", err)
        self.assertIn("make sure it is pushed", err)
        status, _, err = self.sb.run(GITOPS, "add-cell", CELL="nosuchcell", GIT_REPO_URL=CUSTOM_URL)
        self.assertEqual(status, 1)
        self.assertIn("does not exist", err)

    def test_add_cell_needs_a_cell_name(self):
        status, _, err = self.sb.run(GITOPS, "add-cell")
        self.assertEqual(status, 1)
        self.assertIn("usage: make add-cell CELL=<cell>", err)
        self.assertIn("use2", err)


class RepoAccessTest(unittest.TestCase):
    TOKEN = "github_pat_EXAMPLEONLYnotarealtoken0123456789"

    def setUp(self):
        self.sb = Sandbox(hub_world())
        self.token = self.sb.dir / "token"

    def tearDown(self):
        self.sb.close()

    def write_token(self, text, mode=0o600):
        self.token.write_text(text)
        self.token.chmod(mode)

    def test_nothing_to_do_without_a_token_file(self):
        status, _, err = self.sb.run(GITOPS, "repo-access")
        self.assertEqual(status, 0, err)
        self.assertIn("GIT_TOKEN_FILE is not set: nothing to do", err)
        self.assertEqual(self.sb.calls(), [])

    def test_refuses_a_file_that_others_can_read(self):
        self.write_token(self.TOKEN, 0o644)
        status, _, err = self.sb.run(GITOPS, "repo-access", GIT_TOKEN_FILE=str(self.token))
        self.assertEqual(status, 1)
        self.assertIn("mode 644", err)
        self.assertEqual(self.sb.calls(), [])

    def test_refuses_an_empty_file(self):
        self.write_token("\r\n")
        status, _, err = self.sb.run(GITOPS, "repo-access", GIT_TOKEN_FILE=str(self.token))
        self.assertEqual(status, 1)
        self.assertIn("is empty", err)

    def test_creates_the_repository_secret_without_showing_the_token(self):
        self.write_token(self.TOKEN + "\r\n")
        status, out, err = self.sb.run(GITOPS, "repo-access", GIT_TOKEN_FILE=str(self.token),
                                       GIT_REPO_URL=CUSTOM_URL)
        self.assertEqual(status, 0, err)
        (apply,) = [c for c in self.sb.calls("kubectl") if "apply" in c["args"]]
        self.assertIn("--server-side", apply["args"])
        m = apply["stdin"]
        self.assertIn("name: repo-encabulator-fork\n  namespace: argocd", m)
        self.assertIn("argocd.argoproj.io/secret-type: repository", m)
        self.assertIn(f"url: {CUSTOM_URL}", m)
        password = re.search(r"^  password: (\S+)$", m, re.M).group(1)
        self.assertEqual(base64.b64decode(password).decode(), self.TOKEN)  # CR/LF stripped
        # Never in clear: not in any argv, not in the manifest, not in the output.
        for c in self.sb.calls():
            self.assertNotIn(self.TOKEN, " ".join(c["args"]))
        self.assertNotIn(self.TOKEN, m)
        self.assertNotIn(self.TOKEN, out + err)
        self.assertNotIn(password, out + err)

    def test_default_repository_secret_is_named_after_the_repository(self):
        self.write_token(self.TOKEN)
        status, _, err = self.sb.run(GITOPS, "repo-access", GIT_TOKEN_FILE=str(self.token))
        self.assertEqual(status, 0, err)
        (apply,) = [c for c in self.sb.calls("kubectl") if "apply" in c["args"]]
        self.assertIn("name: repo-encabulator\n", apply["stdin"])
        self.assertIn(f"url: {DEFAULT_URL}\n", apply["stdin"])


class CellAccessTest(unittest.TestCase):
    POLICY = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
    TAGS = "auto-delete=no,project=encabulator,talk=opn315"

    def setUp(self):
        self.sb = Sandbox(hub_world())

    def tearDown(self):
        self.sb.close()

    def eks(self, op):
        return [c["args"] for c in self.sb.calls("aws") if c["args"][:2] == ["eks", op]]

    def test_creates_a_tagged_entry_with_cluster_admin_for_the_role_and_its_path(self):
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="use2")
        self.assertEqual(status, 0, err)
        (create,) = self.eks("create-access-entry")
        principal = "arn:aws:iam::111122223333:role/team/ExampleAdmin"
        for flag, value in (("--cluster-name", "encabulator-use2"), ("--region", "us-east-2"),
                            ("--principal-arn", principal), ("--type", "STANDARD"), ("--tags", self.TAGS)):
            self.assertEqual(create[create.index(flag) + 1], value)
        (assoc,) = self.eks("associate-access-policy")
        self.assertEqual(assoc[assoc.index("--policy-arn") + 1], self.POLICY)
        self.assertEqual(assoc[assoc.index("--access-scope") + 1], "type=cluster")
        self.assertIn("aws eks update-kubeconfig --region us-east-2 --name encabulator-use2", err)

    def test_existing_entry_is_tagged_and_given_the_policy(self):
        principal = "arn:aws:iam::111122223333:role/team/ExampleAdmin"
        entry = "arn:aws:eks:us-east-2:111122223333:access-entry/encabulator-use2/role/111122223333/ExampleAdmin/0example"
        self.sb.set_world(hub_world(access_entries={principal: entry}))
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="use2")
        self.assertEqual(status, 0, err)
        self.assertEqual(self.eks("create-access-entry"), [])
        (tag,) = self.eks("tag-resource")
        self.assertEqual(tag[tag.index("--resource-arn") + 1], entry)
        self.assertEqual(tag[tag.index("--tags") + 1], self.TAGS)
        self.assertEqual(len(self.eks("associate-access-policy")), 1)

    def test_without_iam_read_the_role_arn_is_built_and_flagged(self):
        self.sb.set_world(hub_world(roles={}))
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="use2")
        self.assertEqual(status, 0, err)
        (create,) = self.eks("create-access-entry")
        self.assertEqual(create[create.index("--principal-arn") + 1], "arn:aws:iam::111122223333:role/ExampleAdmin")
        self.assertIn("set PRINCIPAL_ARN", err)

    def test_an_iam_user_is_used_as_is(self):
        self.sb.set_world(hub_world(caller="arn:aws:iam::111122223333:user/example-user"))
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="use2")
        self.assertEqual(status, 0, err)
        (create,) = self.eks("create-access-entry")
        self.assertEqual(create[create.index("--principal-arn") + 1], "arn:aws:iam::111122223333:user/example-user")

    def test_refuses_a_cluster_that_is_not_active(self):
        w = hub_world()
        w["clusters"]["encabulator-use2"]["status"] = "CREATING"
        self.sb.set_world(w)
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="use2")
        self.assertEqual(status, 1)
        self.assertIn("not ACTIVE", err)
        self.assertEqual(self.eks("create-access-entry"), [])

    def test_unknown_cell(self):
        status, _, err = self.sb.run(GITOPS, "cell-access", CELL="nosuch")
        self.assertEqual(status, 1)
        self.assertIn("no gitops/hub/cells/nosuch.yaml", err)
        self.assertEqual(self.sb.calls(), [])


class MakefileTest(unittest.TestCase):
    def setUp(self):
        self.sb = Sandbox(hub_world(remote={DEFAULT_URL: {"main": ["gitops/hub/cells/use2.yaml"]}}))

    def tearDown(self):
        self.sb.close()

    def make(self, *args, **settings):
        return self.sb.run("make", "--no-print-directory", *args, **settings)

    def test_targets_are_listed_in_help(self):
        status, out, _ = self.make("help")
        self.assertEqual(status, 0)
        out = re.sub(r"\x1b\[[0-9;]*m", "", out)
        for t in ("gitops-register", "gitops-repo-access", "gitops-bootstrap", "add-cell", "cell-access",
                  "gitops-render-check", "gitops-test"):
            self.assertRegex(out, rf"(?m)^  {re.escape(t)} +\S")

    def test_dry_runs_name_the_scripts(self):
        for target, want in (("gitops-register", "register-hub.sh"), ("gitops-bootstrap", "gitops.sh bootstrap"),
                             ("gitops-repo-access", "gitops.sh repo-access"), ("add-cell", "gitops.sh add-cell"),
                             ("cell-access", "gitops.sh cell-access"), ("gitops-render-check", "gitops.sh check")):
            with self.subTest(target=target):
                status, out, _ = self.make("-n", target, "CELL=use2")
                self.assertEqual(status, 0)
                self.assertIn(want, out)

    def test_settings_reach_the_scripts_from_the_environment(self):
        status, _, err = self.make("gitops-bootstrap", GIT_REPO_URL=CUSTOM_URL, GIT_REVISION="release")
        self.assertEqual(status, 0, err)
        (apply,) = [c for c in self.sb.calls("kubectl") if "apply" in c["args"]]
        self.assertIn(f"repoURL: {CUSTOM_URL}", apply["stdin"])
        status, _, err = self.make("add-cell", "CELL=use2")
        self.assertEqual(status, 0, err)
        self.assertIn("encabulator-use2", err)

    def test_settings_on_the_make_command_line_reach_the_scripts(self):
        status, _, err = self.make("gitops-bootstrap", f"GIT_REPO_URL={CUSTOM_URL}", "GIT_REVISION=release")
        self.assertEqual(status, 0, err)
        (apply,) = [c for c in self.sb.calls("kubectl") if "apply" in c["args"]]
        self.assertIn(f"repoURL: {CUSTOM_URL}", apply["stdin"])
        self.assertIn('targetRevision: "release"', apply["stdin"])

    def test_dry_run_shows_the_settings(self):
        status, out, _ = self.make("-n", "gitops-bootstrap", "GIT_REVISION=release", GIT_REPO_URL=CUSTOM_URL)
        self.assertEqual(status, 0)
        self.assertIn(f"gitops-bootstrap: GIT_REPO_URL={CUSTOM_URL} GIT_REVISION=release", out)
        status, out, _ = self.make("-n", "add-cell", "CELL=use2")
        self.assertIn("GIT_REPO_URL=(default) GIT_REVISION=(default)", out)
        self.assertIn("CELL=use2", out)
        self.assertEqual(self.sb.calls(), [])

    def test_cell_is_not_run_by_the_shell(self):
        marker = self.sb.dir / "ran"
        status, _, err = self.make("add-cell", f"CELL=use2;touch {marker}")
        self.assertNotEqual(status, 0)
        self.assertFalse(marker.exists())
        self.assertIn("usage: make add-cell", err)


if __name__ == "__main__":
    unittest.main(verbosity=2)
