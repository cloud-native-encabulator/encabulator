#!/usr/bin/env python3
"""Stand-in for aws, kubectl, helm and terraform in scripts/teardown_test.sh.

Installed under each of those names; picks its behavior from the name it was called by. It
keeps a small simulated world in $STUB_STATE/world.json: an account with a hub and cells (EKS
clusters, VPCs, route tables, RDS, ElastiCache, IAM roles, ECR) and the hub's and cells'
Kubernetes objects, and plays the parts of kro, ACK, Argo CD and Terraform that the teardown
depends on, in the order the real ones need:

  - kro removes a deleted EncabulatorCell only once its stores are gone, its node role and node
    access entry are kept (deletion-policy retain), and no untracked route table is left in its
    VPC; its capabilities and Argo CD cluster Secret go first;
  - while a cell's controllers run they re-create a deleted StatefulSet, and a StatefulSet
    re-creates a deleted PVC with a new volume;
  - Argo CD re-applies the hub's store objects while hub-brownfield exists, and a store object
    re-applied after its store is deleted makes ACK create a new, empty one;
  - an adopt object syncs only when its AWS resource exists in a region the hub's ACK may use;
    deleting it (deletion-policy delete) deletes the resource; RDS refuses a subnet group delete
    while an instance uses it;
  - a ServerlessCacheSnapshot adopt object never syncs without spec.serverlessCacheName, nor
    once that cache is deleted (ACK reads the snapshot filtered by its cache), as on AWS;
  - a cache snapshot is available or deleting; "expiring" makes it expire between the list and
    the delete, and the delete answers ServerlessCacheSnapshotNotFoundFault.

Every call is appended to $STUB_LOG as "<tool> <args>". A call it does not know fails with
exit 99 and an UNEXPECTED line. STUB_FAIL_ONCE=<regex> fails the first matching call (for the
resume tests), STUB_FAIL=<regex> every matching one. STUB_TF_NO_VERSION=1 makes every terraform
call fail as a version manager's shim with no version set does. `stub.py init [key=value...]`
writes a fresh world. Nothing here reaches a network or a cloud.
"""
import json
import os
import re
import sys

ACCOUNT = "111122223333"
ACK_REGIONS = ("us-west-2", "us-east-2", "eu-west-1")
TAGS = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
HUB = "encabulator-usw2"
HUB_ENDPOINT = "https://HUB.gr7.us-west-2.eks.amazonaws.com"

state_dir = os.environ.get("STUB_STATE", "")
world_path = os.path.join(state_dir, "world.json")


# ------------------------------------------------------------------------------ world
def k(ctx, ns, kind, name):
    return f"{ctx}|{ns}|{kind}|{name}"


def obj(labels=None, annotations=None, spec=None, status=None):
    return {"labels": labels or {}, "annotations": annotations or {}, "spec": spec or {}, "status": status or {}, "deleting": False}


def snapshot(expires):
    """An automatic snapshot of encabulator-valkey, tagged by the hub's tagger."""
    return {"cache": "encabulator-valkey", "tags": dict(TAGS), "status": "available", "expires": expires}


def init(opts):
    cells = [c for c in opts.get("cells", "use2").split(",") if c]
    w = {
        "account": opts.get("account", ACCOUNT),
        "kube": {"current": opts.get("context", HUB), "server": opts.get("server", HUB_ENDPOINT)},
        "k8s": {},
        "eks": {"us-west-2": {HUB: {"endpoint": HUB_ENDPOINT, "caps": ["ARGOCD", "ACK", "KRO"]}}},
        "vpcs": {"us-west-2": {"vpc-hub": {"Name": HUB, **TAGS}}},
        "subnets": {"us-west-2": {"subnet-h1": "vpc-hub", "subnet-h2": "vpc-hub", "subnet-h3": "vpc-hub"}},
        "route_tables": {},
        "rds": {"us-west-2": {"encabulator-pg": {"vpc": "vpc-hub", "status": "available", "tags": dict(TAGS), "protect": False},
                              "other-db": {"vpc": "vpc-other", "status": "available", "tags": {}, "protect": False}}},
        "subnet_groups": {"us-west-2": {"encabulator": {"vpc": "vpc-hub", "subnets": ["subnet-h1", "subnet-h2", "subnet-h3"], "tags": dict(TAGS)}}},
        "caches": {"us-west-2": {"encabulator-valkey": {"subnets": ["subnet-h1", "subnet-h2", "subnet-h3"], "sgs": ["sg-hubdb"], "status": "available", "tags": dict(TAGS)}}},
        "snapshots": {"us-west-2": {"automatic.encabulator-valkey-2026-10-15-09-48": snapshot("2026-10-16T09:48:00+00:00")}},
        "roles": {f"{HUB}-{r}": dict(TAGS) for r in ("cluster", "node", "argocd", "ack", "kro", "tagger")},
        "ecr": {"us-west-2": {}},
        "helm": {"encabulator/encabulator": True, "encabulator-observability/prometheus-stack": True},
        "tf_state": [],
        "events": [],
        "ticks": {},
        "failed_once": [],
        "cell_access": opts.get("cell_access", "yes"),
        "lb_service": opts.get("lb_service", ""),
        "instances": {},
        "tagging": {},
    }
    w["roles"]["other-role"] = {}
    for svc in services():
        w["ecr"]["us-west-2"][f"encabulator/{svc}"] = dict(TAGS)
    w["ecr"]["us-west-2"]["other/app"] = {}
    if opts.get("untagged_repo"):
        w["ecr"]["us-west-2"]["encabulator/untagged"] = {}
    if opts.get("foreign_repo_account"):
        w["ecr"]["us-west-2"]["encabulator/foreign"] = dict(TAGS, _account="444455556666")
    hub = lambda ns, kind, name, **kw: w["k8s"].__setitem__(k(HUB, ns, kind, name), obj(**kw))
    hub("argocd", "secret", HUB, labels={"encabulator.io/hub": "true"})
    for name in ("hub-cells", "hub-cell-apps", "hub-rgds", "hub-brownfield"):
        hub("argocd", "applications.argoproj.io", name)
    for name in ("hub-brownfield", "encabulator", "encabulator-platform"):
        hub("argocd", "applicationsets.argoproj.io", name)
    for kind, name in (("dbinstances.rds.services.k8s.aws", "encabulator-pg"),
                       ("serverlesscaches.elasticache.services.k8s.aws", "encabulator-valkey"),
                       ("dbsubnetgroups.rds.services.k8s.aws", "encabulator")):
        hub("encabulator", kind, name, annotations={"services.k8s.aws/deletion-policy": "retain",
                                                    "services.k8s.aws/adoption-policy": "adopt-or-create"})
    for pvc in ("data-nats-0", "data-influxdb-0"):
        hub("encabulator", "pvc", pvc, labels={"app.kubernetes.io/part-of": "encabulator"})
        hub("", "pv", f"pv-hub-{pvc}")
    regions = {"use2": "us-east-2", "euw1": "eu-west-1", "reh1": "us-east-2"}
    for i, c in enumerate(cells):
        add_cell(w, f"encabulator-{c}", opts.get(f"region_{c}", regions.get(c, "us-east-2")), i, opts)
    if opts.get("pre_handoff"):
        w["tf_state"] = PLAN_ADDRESSES + ["aws_db_instance.pg", "aws_db_subnet_group.encabulator", "aws_elasticache_serverless_cache.valkey"]
        for key in [x for x in w["k8s"] if "|encabulator|" in x and ".services.k8s.aws|" in x]:
            del w["k8s"][key]
        for name in ("hub-brownfield",):
            w["k8s"].pop(k(HUB, "argocd", "applications.argoproj.io", name), None)
            w["k8s"].pop(k(HUB, "argocd", "applicationsets.argoproj.io", name), None)
    else:
        w["tf_state"] = list(PLAN_ADDRESSES)
    if opts.get("state") == "empty":
        w["tf_state"] = []
    w["plan_variant"] = opts.get("plan", "")
    # apply=fail: terraform apply of the destroy plan fails partway with an error and exit 1.
    # tf_elapsed=go: its progress lines show the time so far as older Terraform versions do.
    w["apply"] = opts.get("apply", "")
    w["tf_elapsed"] = opts.get("tf_elapsed", "")
    if opts.get("hub") == "gone":
        w["eks"]["us-west-2"].pop(HUB)
        w["vpcs"]["us-west-2"].pop("vpc-hub")
        for table in ("rds", "caches", "subnet_groups", "snapshots"):
            w[table]["us-west-2"] = {n: v for n, v in w[table]["us-west-2"].items() if "encabulator" not in n}
        w["roles"] = {r: t for r, t in w["roles"].items() if not r.startswith(HUB)}
        w["k8s"] = {x: o for x, o in w["k8s"].items() if not x.startswith(f"{HUB}|")}
        w["helm"] = {}
    if opts.get("stuck"):
        w["rds"][opts.get("stuck")]["encabulator-pg"]["stuck"] = True
    save(w)


def add_cell(w, name, region, i, opts):
    vpc = f"vpc-{name[12:]}"
    w["eks"].setdefault(region, {})[name] = {"endpoint": f"https://{name}.example", "caps": ["KRO", "ACK"]}
    w["vpcs"].setdefault(region, {})[vpc] = {"Name": name, **TAGS}
    subs = {f"subnet-{name[12:]}{j}": vpc for j in range(1, 4)}
    w["subnets"].setdefault(region, {}).update(subs)
    rts = w["route_tables"].setdefault(region, {})
    rts[f"rtb-{name[12:]}-tracked"] = {"vpc": vpc, "tags": {"Name": f"{name}-private", **TAGS}, "assoc": 3, "routes": ["local", "nat-1"]}
    if opts.get("tracked_bare") == "yes":
        # The table the hub's RouteTable object tracks, caught with no association and no NAT
        # route yet: it looks like a leftover, but kro owns it.
        rts[f"rtb-{name[12:]}-tracked"].update(assoc=0, routes=["local"])
    rts[f"rtb-{name[12:]}-public"] = {"vpc": vpc, "tags": {"Name": f"{name}-public", **TAGS}, "assoc": 3, "routes": ["local", "igw-1"]}
    for j in range(int(opts.get("leaked", "2"))):
        rts[f"rtb-{name[12:]}-leak{j}"] = {"vpc": vpc, "tags": {"Name": f"{name}-private", **TAGS}, "assoc": 0, "routes": ["local"]}
    # Looks like a leftover, but carries no project tag: never touched.
    rts[f"rtb-{name[12:]}-notours"] = {"vpc": vpc, "tags": {"Name": f"{name}-private"}, "assoc": 0, "routes": ["local"]}
    w["rds"].setdefault(region, {})["encabulator-pg"] = {"vpc": vpc, "status": "available", "tags": dict(TAGS), "protect": opts.get("protect") == "yes"}
    w["subnet_groups"].setdefault(region, {})["encabulator"] = {"vpc": vpc, "subnets": list(subs), "tags": dict(TAGS)}
    w["caches"].setdefault(region, {})["encabulator-valkey"] = {"subnets": list(subs), "sgs": [f"sg-{name[12:]}db"], "status": "available", "tags": dict(TAGS)}
    w["snapshots"].setdefault(region, {})["automatic.encabulator-valkey-2026-10-15-09-02"] = snapshot("2026-10-16T09:02:00+00:00")
    for r in ("cluster", "node", "ack", "kro"):
        w["roles"][f"{name}-{r}"] = dict(TAGS)
    hub = lambda ns, kind, n, **kw: w["k8s"].__setitem__(k(HUB, ns, kind, n), obj(**kw))
    kro = {"kro.run/instance-name": name}
    hub("encabulator-cells", "encabulatorcells", name, spec={"region": region}, status={"state": "ACTIVE"})
    for r in ("cluster", "node", "ack", "kro"):
        hub("encabulator-cells", "roles.iam.services.k8s.aws", f"{name}-{r}", labels=kro)
    for a in ("node", "ack", "kro", "argocd"):
        hub("encabulator-cells", "accessentries.eks.services.k8s.aws", f"{name}-{a}", labels=kro)
    hub("encabulator-cells", "routetables.ec2.services.k8s.aws", f"{name}-private", labels=kro, status={"routeTableID": f"rtb-{name[12:]}-tracked"})
    hub("encabulator-cells", "routetables.ec2.services.k8s.aws", f"{name}-public", labels=kro, status={"routeTableID": f"rtb-{name[12:]}-public"})
    hub("argocd", "secret", name, labels={"encabulator.io/cell": "true"})
    for sts in ("nats", "influxdb"):
        w["k8s"][k(name, "encabulator", "statefulset", sts)] = obj()
        w["k8s"][k(name, "encabulator", "pvc", f"data-{sts}-0")] = obj()
        w["k8s"][k(name, "", "pv", f"pv-{sts}-1")] = obj(spec={"claim": f"data-{sts}-0"})


def services():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    out = []
    with open(os.path.join(root, "services.txt"), encoding="utf-8") as f:
        for line in f:
            if line.strip() and not line.lstrip().startswith("#"):
                out.append(line.split("\t")[0].split()[0])
    return out


def load():
    with open(world_path, encoding="utf-8") as f:
        return json.load(f)


def save(w):
    os.makedirs(state_dir, exist_ok=True)
    with open(world_path, "w", encoding="utf-8") as f:
        json.dump(w, f, indent=1, sort_keys=True)


def tick(w, key):
    w["ticks"][key] = w["ticks"].get(key, 0) + 1
    return w["ticks"][key]


def event(w, text):
    w["events"].append(text)


# ---------------------------------------------------------------------------- helpers
def out(text=""):
    if text != "":
        print(text)


def fail(msg, rc=1):
    print(msg, file=sys.stderr)
    raise Done(rc)


class Done(Exception):
    def __init__(self, rc):
        self.rc = rc


def unexpected(tool, args):
    with open(os.environ["STUB_LOG"], "a", encoding="utf-8") as f:
        f.write(f"UNEXPECTED {tool} {' '.join(args)}\n")
    fail(f"stub {tool}: unexpected call: {' '.join(args)}", 99)


def opt(args, name, default=None):
    for i, a in enumerate(args):
        if a == name and i + 1 < len(args):
            return args[i + 1]
        if a.startswith(name + "="):
            return a.split("=", 1)[1]
    return default


def text_rows(rows):
    return "\n".join("\t".join(str(c) for c in r) for r in rows)


VALUE_FLAGS = {"-o", "-n", "-l", "-f", "--for", "--timeout", "--context", "--kubeconfig"}


def positional(a):
    """The arguments that are not flags or flag values."""
    out, skip = [], False
    for x in a:
        if skip:
            skip = False
        elif x in VALUE_FLAGS:
            skip = True
        elif not x.startswith("-"):
            out.append(x)
    return out


# ------------------------------------------------------------------------- dynamics
def cell_of_region(w, region, vpc=None):
    for name, c in w["eks"].get(region, {}).items():
        if name != HUB:
            return name
    return None


def kro_progress(w):
    """kro on the hub, for every EncabulatorCell being deleted."""
    for key in [x for x in w["k8s"] if x.startswith(f"{HUB}|encabulator-cells|encabulatorcells|")]:
        o = w["k8s"][key]
        if not o["deleting"]:
            continue
        name = key.split("|")[3]
        region = o["spec"]["region"]
        n = tick(w, f"kro:{name}")
        cl = w["eks"].get(region, {}).get(name)
        if n >= 5:
            w["k8s"].pop(k(HUB, "argocd", "secret", name), None)
            if cl:
                cl["caps"] = []
        node_role = w["k8s"].get(k(HUB, "encabulator-cells", "roles.iam.services.k8s.aws", f"{name}-node"))
        node_entry = w["k8s"].get(k(HUB, "encabulator-cells", "accessentries.eks.services.k8s.aws", f"{name}-node"))
        if n >= 6:
            # kro deletes the node access entry early; without retain, EKS deletes it too.
            if node_entry is not None:
                if node_entry["annotations"].get("services.k8s.aws/deletion-policy") != "retain":
                    w["node_entry_lost"] = w.get("node_entry_lost", []) + [name]
                del w["k8s"][k(HUB, "encabulator-cells", "accessentries.eks.services.k8s.aws", f"{name}-node")]
            if node_role is not None:
                if node_role["annotations"].get("services.k8s.aws/deletion-policy") != "retain":
                    event(w, f"kro waits on the node role of {name}")
                    continue  # the role is in the instance profile: kro waits forever
                del w["k8s"][k(HUB, "encabulator-cells", "roles.iam.services.k8s.aws", f"{name}-node")]
        vpc = next((v for v, t in w["vpcs"].get(region, {}).items() if t.get("Name") == name), None)
        if region_has_store_in(w, region, vpc):
            continue  # the stores keep the cell's encabulator-db security group in use
        tracked = {o["status"].get("routeTableID") for x, o in w["k8s"].items() if "|routetables.ec2.services.k8s.aws|" in x}
        if vpc and any(rt["vpc"] == vpc and rt["assoc"] == 0 and rt["routes"] == ["local"] and rid not in tracked
                       and rt["tags"].get("project") == "encabulator" for rid, rt in w["route_tables"].get(region, {}).items()):
            event(w, f"the VPC of {name} cannot be deleted: an untracked route table is left")
            continue
        if n < 8:
            continue
        # The cell is gone, but the retained node role (and anything else retained).
        del w["k8s"][key]
        w["eks"].get(region, {}).pop(name, None)
        if vpc:
            w["vpcs"][region].pop(vpc)
            for rid in [r for r, rt in w["route_tables"].get(region, {}).items() if rt["vpc"] == vpc and rt["tags"].get("project")]:
                w["route_tables"][region].pop(rid)
        for r in ("cluster", "ack", "kro"):
            w["roles"].pop(f"{name}-{r}", None)
        for x in [x for x in w["k8s"] if x.startswith(f"{HUB}|encabulator-cells|") and w["k8s"][x]["labels"].get("kro.run/instance-name") == name]:
            del w["k8s"][x]
        for x in [x for x in w["k8s"] if x.startswith(f"{name}|")]:
            if x.split("|")[2] == "pv":
                event(w, f"volume {x.split('|')[3]} of {name} left in EC2")
            del w["k8s"][x]
        event(w, f"cell {name} gone")


def region_has_store_in(w, region, vpc):
    if not vpc:
        return False
    if any(d["vpc"] == vpc for d in w["rds"].get(region, {}).values()):
        return True
    subs = {s for s, v in w["subnets"].get(region, {}).items() if v == vpc}
    return any(set(c["subnets"]) & subs for c in w["caches"].get(region, {}).values())


def cell_controllers_running(w, cell):
    for region, cls in w["eks"].items():
        if cell in cls and cls[cell]["caps"]:
            return True
    return k(HUB, "argocd", "secret", cell) in w["k8s"]


def argocd_progress(w):
    """Argo CD on the hub."""
    apps = lambda n: k(HUB, "argocd", "applications.argoproj.io", n) in w["k8s"]
    sets = lambda n: k(HUB, "argocd", "applicationsets.argoproj.io", n) in w["k8s"]
    if apps("hub-brownfield") and not sets("hub-brownfield"):
        if tick(w, "brownfield-cascade") >= 2:
            del w["k8s"][k(HUB, "argocd", "applications.argoproj.io", "hub-brownfield")]
            for kind, name in (("dbinstances.rds.services.k8s.aws", "encabulator-pg"),
                               ("serverlesscaches.elasticache.services.k8s.aws", "encabulator-valkey"),
                               ("dbsubnetgroups.rds.services.k8s.aws", "encabulator")):
                w["k8s"].pop(k(HUB, "encabulator", kind, name), None)
    if apps("hub-cell-apps"):
        for name in ("encabulator", "encabulator-platform"):
            if not sets(name) and w.get("had_appset_" + name):
                w["k8s"][k(HUB, "argocd", "applicationsets.argoproj.io", name)] = obj()
                event(w, f"hub-cell-apps re-created the ApplicationSet {name}")
    if apps("hub-brownfield"):
        # selfHeal: the store objects come back, adopt-or-create.
        for kind, name, store in (("dbinstances.rds.services.k8s.aws", "encabulator-pg", "rds"),
                                  ("serverlesscaches.elasticache.services.k8s.aws", "encabulator-valkey", "caches")):
            key = k(HUB, "encabulator", kind, name)
            if key not in w["k8s"]:
                w["k8s"][key] = obj(annotations={"services.k8s.aws/deletion-policy": "retain"})
            if name not in w[store].get("us-west-2", {}):
                new = {"vpc": "vpc-hub", "status": "creating", "tags": dict(TAGS), "protect": False} if store == "rds" else \
                      {"subnets": ["subnet-h1"], "sgs": ["sg-hubdb"], "status": "creating", "tags": dict(TAGS)}
                w[store].setdefault("us-west-2", {})[name] = new
                event(w, f"ACK created a new, empty {name} (adopt-or-create)")


def stores_progress(w):
    for region, dbs in w["rds"].items():
        for name in list(dbs):
            if dbs[name]["status"] == "deleting" and not dbs[name].get("stuck") and tick(w, f"rds:{region}:{name}") >= 2:
                del dbs[name]
                event(w, f"rds {region} {name} deleted")
    for region, cs in w["caches"].items():
        for name in list(cs):
            if cs[name]["status"] == "deleting" and tick(w, f"cache:{region}:{name}") >= 2:
                del cs[name]
                event(w, f"cache {region} {name} deleted")


def progress(w):
    if HUB not in w["eks"].get("us-west-2", {}):
        stores_progress(w)
        return  # no hub: no kro, no ACK, no Argo CD
    kro_progress(w)
    argocd_progress(w)
    stores_progress(w)


# ---------------------------------------------------------------------------- kubectl
KINDS = {
    "application": "applications.argoproj.io", "applications": "applications.argoproj.io",
    "applications.argoproj.io": "applications.argoproj.io",
    "applicationset": "applicationsets.argoproj.io", "applicationsets.argoproj.io": "applicationsets.argoproj.io",
    "encabulatorcells": "encabulatorcells", "encabulatorcell": "encabulatorcells",
    "statefulset": "statefulset", "statefulsets": "statefulset", "pvc": "pvc", "persistentvolumeclaims": "pvc",
    "pv": "pv", "secret": "secret", "configmap": "configmap", "svc": "svc",
}
YAML_KINDS = {
    "DBInstance": "dbinstances.rds.services.k8s.aws", "ServerlessCache": "serverlesscaches.elasticache.services.k8s.aws",
    "DBSubnetGroup": "dbsubnetgroups.rds.services.k8s.aws", "ServerlessCacheSnapshot": "serverlesscachesnapshots.elasticache.services.k8s.aws",
    "Role": "roles.iam.services.k8s.aws", "RouteTable": "routetables.ec2.services.k8s.aws",
}


def kind_of(s):
    if s in KINDS:
        return KINDS[s]
    if s.endswith(".services.k8s.aws"):
        return s
    fail(f'error: the server doesn\'t have a resource type "{s}"')


def parse_yaml_object(path):
    o = obj()
    section = sub = None
    kind = name = ns = None
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line.strip():
                continue
            if not line.startswith(" "):
                section, _, v = line.partition(":")
                if section == "kind":
                    kind = v.strip()
                continue
            m = re.match(r"^  (\w+):\s*(.*)$", line)
            if m:
                sub = m.group(1)
                if section == "metadata" and sub == "name":
                    name = m.group(2)
                elif section == "metadata" and sub == "namespace":
                    ns = m.group(2)
                elif section == "spec":
                    o["spec"][sub] = m.group(2).strip('"')
                continue
            m = re.match(r"^    ([\w./-]+):\s*(.*)$", line)
            if m and section == "metadata" and sub in ("labels", "annotations"):
                v = m.group(2)
                if v.startswith("'") and v.endswith("'"):
                    v = v[1:-1]
                o[sub][m.group(1)] = v.strip('"')
    return YAML_KINDS[kind], ns or "", name, o


def kubectl(w, args):
    a = list(args)
    if a[:2] == ["config", "current-context"]:
        if not w["kube"]["current"]:
            fail("error: current-context is not set")
        return out(w["kube"]["current"])
    if a[:3] == ["config", "view", "--minify"]:
        return out(w["kube"]["server"])
    cfg = None
    if a[:1] == ["--kubeconfig"]:
        cfg = a[1]
        a = a[2:]
    ctx = w["kube"]["current"]
    if a[:1] == ["--context"]:
        ctx = a[1]
        a = a[2:]
    if cfg is None and ctx != HUB:
        fail(f'error: context "{ctx}" does not exist')
    if cfg is not None:
        if not os.path.exists(cfg) or ctx not in open(cfg, encoding="utf-8").read():
            fail(f'error: context "{ctx}" does not exist')
        if ctx not in {n for cls in w["eks"].values() for n in cls}:
            fail("dial tcp: lookup: no such host")
    ns = ""
    if a[:1] == ["-n"]:
        ns = a[1]
        a = a[2:]
    if a[:2] == ["auth", "can-i"]:
        return out("yes" if w["cell_access"] == "yes" else "no")
    verb = a[0] if a else ""
    progress(w)
    if verb == "get":
        return k_get(w, ctx, ns, a[1:])
    if verb == "annotate":
        rest = positional(a[1:])
        kind, name, kv = kind_of(rest[0]), rest[1], rest[2]
        key = k(ctx, ns, kind, name)
        if key not in w["k8s"]:
            fail(f'Error from server (NotFound): {kind} "{name}" not found')
        kk, _, vv = kv.partition("=")
        w["k8s"][key]["annotations"][kk] = vv
        return out(f"{kind}/{name} annotated")
    if verb == "delete":
        return k_delete(w, ctx, ns, a[1:])
    if verb == "apply":
        kind, ons, name, o = parse_yaml_object(opt(a, "-f"))
        key = k(ctx, ons, kind, name)
        if key in w["k8s"]:
            w["k8s"][key]["annotations"].update(o["annotations"])
        else:
            w["k8s"][key] = o
        return out(f"{kind}/{name} created")
    if verb == "wait":
        target = positional(a[1:])[0]
        kind, name = target.split("/", 1)
        return k_wait(w, ctx, ns, kind_of(kind), name)
    unexpected("kubectl", args)


def k_get(w, ctx, ns, a):
    pos = positional(a)
    jp = opt(a, "-o", "")
    allns = "-A" in a
    kinds = [kind_of(x) for x in pos[0].split(",")]
    names = pos[1:]
    if pos[0] == "pv" or kinds == ["pv"]:
        ns = ""
    rows = []
    for key, o in sorted(w["k8s"].items()):
        c, n2, kd, nm = key.split("|")
        if c != ctx or kd not in kinds or (not allns and n2 != ns) or (names and nm not in names):
            continue
        rows.append((kd, nm, o, n2))
    if names and not rows and "--ignore-not-found" not in a:
        fail(f'Error from server (NotFound): {kinds[0]} "{names[0]}" not found')
    if kinds == ["svc"]:
        return out(w["lb_service"] + " " if w["lb_service"] else "")
    if jp == "name":
        return out("\n".join(f"{kd}/{nm}" for kd, nm, _, _ in rows))
    if jp.startswith("jsonpath="):
        expr = jp[len("jsonpath="):]
        if "range .items" in expr:
            field = re.search(r"\}\{(\.[^}]+)\}", expr).group(1)
            return out("".join(f"{jsonpath(o, nm, field)} " for _, nm, o, _ in rows))
        if not rows:
            return
        return out(jsonpath(rows[0][2], rows[0][1], expr.strip("{}")))
    unexpected("kubectl", ["get"] + a)


def jsonpath(o, name, field):
    field = field.replace("\\.", "\x00")
    parts = [p.replace("\x00", ".") for p in field.strip(".").split(".")]
    if parts == ["metadata", "name"]:
        return name
    if parts == ["metadata", "deletionTimestamp"]:
        return "2026-10-16T00:00:00Z" if o["deleting"] else ""
    cur = o
    for p in parts:
        if p == "metadata":
            continue
        cur = cur.get(p, {}) if isinstance(cur, dict) else {}
    return cur if isinstance(cur, str) else ""


def k_delete(w, ctx, ns, a):
    sel = opt(a, "-l")
    pos = positional(a)
    kind = kind_of(pos[0])
    names = pos[1:]
    if sel:
        lk, _, lv = sel.partition("=")
        names = [key.split("|")[3] for key, o in w["k8s"].items()
                 if key.startswith(f"{ctx}|{ns}|{kind}|") and o["labels"].get(lk) == lv]
    for name in names:
        key = k(ctx, ns, kind, name)
        if key not in w["k8s"]:
            if "--ignore-not-found" in a:
                continue
            fail(f'Error from server (NotFound): {kind} "{name}" not found')
        o = w["k8s"][key]
        if kind == "applicationsets.argoproj.io":
            w["had_appset_" + name] = True
        if kind == "encabulatorcells":
            o["deleting"] = True
            event(w, f"EncabulatorCell {name} delete requested")
            continue
        if ctx != HUB and kind in ("statefulset", "pvc"):
            k_delete_cell(w, ctx, kind, name)
            continue
        if ctx == HUB and kind == "pvc":
            if "encabulator/encabulator" in w["helm"]:
                event(w, f"hub PVC {name} re-created by its StatefulSet")
                continue
            del w["k8s"][key]
            w["k8s"].pop(k(HUB, "", "pv", f"pv-hub-{name}"), None)
            continue
        if kind.endswith(".services.k8s.aws") and o["annotations"].get("services.k8s.aws/deletion-policy") == "delete":
            if not ack_delete(w, kind, o):
                fail(f"error: timed out waiting for the deletion of {kind}/{name}")
        del w["k8s"][key]
        event(w, f"deleted {ctx} {ns} {kind} {name}")
    return out("deleted")


def k_delete_cell(w, cell, kind, name):
    running = cell_controllers_running(w, cell)
    if kind == "statefulset":
        if running:
            event(w, f"{cell} StatefulSet {name} re-created by its controllers")
            return
        w["k8s"].pop(k(cell, "encabulator", "statefulset", name), None)
        return
    sts = name[len("data-"):-len("-0")]
    w["k8s"].pop(k(cell, "", "pv", f"pv-{sts}-1"), None)
    if k(cell, "encabulator", "statefulset", sts) in w["k8s"]:
        n = tick(w, f"newpv:{cell}:{sts}")
        w["k8s"][k(cell, "", "pv", f"pv-{sts}-{n + 1}")] = obj()
        event(w, f"{cell} PVC {name} re-created with a new volume")
        return
    if cell in w.get("node_entry_lost", []):
        w["k8s"][k(cell, "", "pv", f"pv-{sts}-stuck")] = obj()
        event(w, f"{cell} volume of {name} cannot be detached: the nodes lost their access entry")
    w["k8s"].pop(k(cell, "encabulator", "pvc", name), None)


def ack_resource(w, kind, o):
    region = o["annotations"].get("services.k8s.aws/region", "us-west-2")
    fields = json.loads(o["annotations"].get("services.k8s.aws/adoption-fields", "{}") or "{}")
    ident = next(iter(fields.values()), "")
    table = {"dbinstances.rds.services.k8s.aws": "rds", "serverlesscaches.elasticache.services.k8s.aws": "caches",
             "dbsubnetgroups.rds.services.k8s.aws": "subnet_groups",
             "serverlesscachesnapshots.elasticache.services.k8s.aws": "snapshots",
             "routetables.ec2.services.k8s.aws": "route_tables"}.get(kind)
    if kind == "roles.iam.services.k8s.aws":
        return w["roles"], ident, region
    return w[table].setdefault(region, {}), ident, region


def k_wait(w, ctx, ns, kind, name):
    o = w["k8s"].get(k(ctx, ns, kind, name))
    if o is None:
        fail(f'Error from server (NotFound): {kind} "{name}" not found')
    store, ident, region = ack_resource(w, kind, o)
    if o["annotations"].get("services.k8s.aws/adoption-policy") != "adopt" or region not in ACK_REGIONS or ident not in store:
        fail("error: timed out waiting for the condition")
    if kind == "serverlesscachesnapshots.elasticache.services.k8s.aws":
        # ACK.ReferencesResolved stays Unknown without spec.serverlessCacheName, and with it
        # DescribeServerlessCacheSnapshots answers ServerlessCacheNotFoundFault once the cache is gone.
        cache = o["spec"].get("serverlessCacheName")
        if not cache or cache not in w["caches"].get(region, {}):
            fail("error: timed out waiting for the condition")
    return out(f"{kind}/{name} condition met")


def ack_delete(w, kind, o):
    store, ident, region = ack_resource(w, kind, o)
    if ident not in store:
        return True
    if kind == "dbinstances.rds.services.k8s.aws":
        store[ident]["status"] = "deleting"
    elif kind == "serverlesscaches.elasticache.services.k8s.aws":
        store[ident]["status"] = "deleting"
    elif kind == "dbsubnetgroups.rds.services.k8s.aws":
        if any(d["vpc"] == store[ident]["vpc"] for d in w["rds"].get(region, {}).values()):
            return False  # RDS refuses while an instance uses the group
        del store[ident]
    elif kind == "roles.iam.services.k8s.aws":
        cell = ident[: -len("-node")]
        if any(cell in cls for cls in w["eks"].values()):
            return False  # still in the cluster's instance profile
        del store[ident]
    else:
        del store[ident]
    event(w, f"ACK deleted {kind.split('.')[0]} {region} {ident}")
    return True


# -------------------------------------------------------------------------------- aws
def not_found(code, op):
    fail(f"An error occurred ({code}) when calling the {op} operation: not found", 254)


def aws(w, args):
    if args[:1] == ["--version"]:
        return out("aws-cli/2.31.0")
    svc, op = (args + ["", ""])[:2]
    a = args[2:]
    q = opt(a, "--query", "")
    region = opt(a, "--region", "us-west-2")
    progress(w)
    call = f"{svc} {op}"
    if call == "sts get-caller-identity":
        return out(w["account"] if q == "Account" else f"arn:aws:sts::{w['account']}:assumed-role/Admin/tester")
    if call == "eks describe-cluster":
        cl = w["eks"].get(region, {}).get(opt(a, "--name"))
        if not cl:
            not_found("ResourceNotFoundException", "DescribeCluster")
        return out({"cluster.status": "ACTIVE", "cluster.endpoint": cl["endpoint"]}.get(q, json.dumps({"cluster": {"status": "ACTIVE"}})))
    if call == "eks list-capabilities":
        cl = w["eks"].get(region, {}).get(opt(a, "--cluster-name"))
        if not cl:
            not_found("ResourceNotFoundException", "ListCapabilities")
        return out("\t".join(cl["caps"]))
    if call == "eks update-kubeconfig":
        with open(opt(a, "--kubeconfig"), "w", encoding="utf-8") as f:
            f.write(f"contexts: [{opt(a, '--alias')}]\n")
        return out(f"Added new context {opt(a, '--alias')}")
    if call == "eks list-clusters":
        return out("\t".join(n for n in w["eks"].get(region, {}) if n.startswith("encabulator")))
    if call == "eks describe-access-entry":
        not_found("ResourceNotFoundException", "DescribeAccessEntry")
    if call in ("eks create-access-entry", "eks associate-access-policy", "eks tag-resource"):
        w["cell_access"] = "yes"
        return out("{}")
    if call == "ec2 describe-vpcs":
        filters = " ".join(x for x in a if x.startswith("Name="))
        ids = []
        for vid, t in w["vpcs"].get(region, {}).items():
            m = re.search(r"Name=tag:Name,Values=(\S+)", filters)
            if m and not re.fullmatch(m.group(1).replace("*", ".*"), t.get("Name", "")):
                continue
            if "tag:project" in filters and t.get("project") != "encabulator":
                continue
            ids.append(vid)
        return out("\t".join(ids))
    if call == "ec2 describe-subnets":
        if "--subnet-ids" in a:
            ids = a[a.index("--subnet-ids") + 1:]
            ids = ids[: next((i for i, x in enumerate(ids) if x.startswith("--")), len(ids))]
            return out("\t".join(w["subnets"].get(region, {}).get(s, "vpc-unknown") for s in ids))
        return out("")
    if call == "ec2 describe-route-tables" and opt(a, "--output") == "json":
        f = " ".join(x for x in a if x.startswith("Name="))
        vpc = re.search(r"vpc-id,Values=(\S+)", f).group(1)
        name = re.search(r"tag:Name,Values=(\S+)", f).group(1)
        rts = [{"RouteTableId": rid, "VpcId": rt["vpc"], "Tags": [{"Key": kk, "Value": vv} for kk, vv in rt["tags"].items()],
                "Associations": [{"Main": False}] * rt["assoc"], "Routes": [{"GatewayId": g} for g in rt["routes"]]}
               for rid, rt in w["route_tables"].get(region, {}).items() if rt["vpc"] == vpc and rt["tags"].get("Name") == name]
        return out(json.dumps({"RouteTables": rts}))
    if call == "rds describe-db-instances" and "--db-instance-identifier" in a:
        d = w["rds"].get(region, {}).get(opt(a, "--db-instance-identifier"))
        if not d:
            not_found("DBInstanceNotFound", "DescribeDBInstances")
        arn = f"arn:aws:rds:{region}:{w['account']}:db:{opt(a, '--db-instance-identifier')}"
        return out({"DBInstances[0].[DBInstanceArn,DBInstanceStatus,DBSubnetGroup.VpcId,DeletionProtection]":
                    f"{arn}\t{d['status']}\t{d['vpc']}\t{d['protect']}",
                    "DBInstances[0].DBInstanceStatus": d["status"],
                    "DBInstances[0].DBInstanceIdentifier": opt(a, "--db-instance-identifier")}.get(q, json.dumps({"DBInstances": [{}]})))
    if call == "rds describe-db-instances":
        return out("\t".join(n for n in w["rds"].get(region, {}) if n.startswith("encabulator")))
    if call in ("rds list-tags-for-resource", "elasticache list-tags-for-resource", "ecr list-tags-for-resource"):
        return out(tags_of(w, opt(a, "--resource-name") or opt(a, "--resource-arn")).get("project", ""))
    if call == "rds describe-db-subnet-groups" and "--db-subnet-group-name" in a:
        g = w["subnet_groups"].get(region, {}).get(opt(a, "--db-subnet-group-name"))
        if not g:
            not_found("DBSubnetGroupNotFoundFault", "DescribeDBSubnetGroups")
        arn = f"arn:aws:rds:{region}:{w['account']}:subgrp:{opt(a, '--db-subnet-group-name')}"
        return out({"DBSubnetGroups[0].[DBSubnetGroupArn,VpcId]": f"{arn}\t{g['vpc']}",
                    "DBSubnetGroups[0].Subnets[].SubnetIdentifier": "\t".join(g["subnets"])}.get(q, "{}"))
    if call == "rds describe-db-subnet-groups":
        return out("\t".join(n for n in w["subnet_groups"].get(region, {}) if n.startswith("encabulator")))
    if call == "rds delete-db-instance":
        return cli_delete(w, "rds", region, opt(a, "--db-instance-identifier"))
    if call == "rds delete-db-subnet-group":
        return cli_delete(w, "subnet_groups", region, opt(a, "--db-subnet-group-name"))
    if call == "elasticache describe-serverless-caches" and "--serverless-cache-name" in a:
        c = w["caches"].get(region, {}).get(opt(a, "--serverless-cache-name"))
        if not c:
            if "length(" in q:
                return out("0")
            not_found("ServerlessCacheNotFoundFault", "DescribeServerlessCaches")
        arn = f"arn:aws:elasticache:{region}:{w['account']}:serverlesscache:{opt(a, '--serverless-cache-name')}"
        return out({"ServerlessCaches[0].[ARN,Status]": f"{arn}\t{c['status']}", "ServerlessCaches[0].Status": c["status"],
                    "ServerlessCaches[0].SubnetIds": "\t".join(c["subnets"]),
                    "ServerlessCaches[0].SecurityGroupIds": "\t".join(c["sgs"])}.get(q, "1"))
    if call == "elasticache describe-serverless-caches":
        return out("\t".join(n for n in w["caches"].get(region, {}) if n.startswith("encabulator")))
    if call == "elasticache describe-serverless-cache-snapshots":
        snaps = w["snapshots"].get(region, {})
        if "--serverless-cache-snapshot-name" in a:
            return out("1" if opt(a, "--serverless-cache-snapshot-name") in snaps else "0")
        if "ServerlessCacheConfiguration" in q:
            # The columns the query asks for: [Name,ARN] or [Name,ARN,Status].
            return out(text_rows((n, f"arn:aws:elasticache:{region}:{w['account']}:serverlesscachesnapshot:{n}")
                                 + ((s["status"],) if q.endswith(",Status]") else ())
                                 for n, s in snaps.items() if s["cache"] == "encabulator-valkey"))
        if q.endswith(".[ServerlessCacheSnapshotName,ExpiryTime]"):
            return out(text_rows((n, s["expires"]) for n, s in snaps.items() if "encabulator" in n))
        return out("\t".join(n for n in snaps if "encabulator" in n))
    if call == "elasticache delete-serverless-cache":
        return cli_delete(w, "caches", region, opt(a, "--serverless-cache-name"))
    if call == "elasticache delete-serverless-cache-snapshot":
        name = opt(a, "--serverless-cache-snapshot-name")
        s = w["snapshots"].get(region, {}).get(name)
        if s and s.get("expiring"):
            del w["snapshots"][region][name]
            event(w, f"snapshot {region} {name} expired")
            s = None
        if not s:
            not_found("ServerlessCacheSnapshotNotFoundFault", "DeleteServerlessCacheSnapshot")
        if s["status"] == "deleting":
            fail("An error occurred (InvalidServerlessCacheSnapshotStateFault) when calling the DeleteServerlessCacheSnapshot "
                 "operation: the snapshot is being deleted", 254)
        return cli_delete(w, "snapshots", region, name)
    if call == "iam get-role":
        name = opt(a, "--role-name")
        if name not in w["roles"]:
            not_found("NoSuchEntity", "GetRole")
        return out(name if q == "Role.RoleName" else f"arn:aws:iam::{w['account']}:role/{name}")
    if call == "iam list-role-tags":
        return out(w["roles"].get(opt(a, "--role-name"), {}).get("project", ""))
    if call == "iam list-roles":
        return out("\t".join(r for r in w["roles"] if r.startswith("encabulator")))
    if call == "ecr describe-repositories":
        repos = {n: t for n, t in w["ecr"].get(region, {}).items() if n.startswith("encabulator")}
        if q.startswith("length("):
            return out(str(sum(1 for n in repos if n.startswith("encabulator/"))))
        if q.endswith(".[repositoryName,repositoryArn]"):
            return out(text_rows((n, f"arn:aws:ecr:{region}:{t.get('_account', w['account'])}:repository/{n}")
                                 for n, t in repos.items() if n.startswith("encabulator/")))
        return out("\t".join(repos))
    if call == "ecr delete-repository":
        name = opt(a, "--repository-name")
        w["ecr"].get(region, {}).pop(name, None)
        event(w, f"ECR deleted {name}")
        return out("{}")
    if call == "resourcegroupstaggingapi get-resources":
        arns = w["tagging"].get(region, [])
        return out(str(len(arns)) if q.startswith("length(") else "\t".join(arns))
    if call == "ec2 describe-instances":
        if "--instance-ids" in a:
            i = w["instances"].get(opt(a, "--instance-ids"))
            return out(i["state"] if i else "")
        managed = "--include-managed-resources" in a
        return out("\t".join(i for i, x in w["instances"].items() if x["region"] == region and (managed or not x["managed"])
                             and x["state"] == "running"))
    if call == "ec2 describe-network-interfaces" and "--network-interface-ids" in a:
        not_found("InvalidNetworkInterfaceID.NotFound", "DescribeNetworkInterfaces")
    if call == "ec2 describe-regions":
        return out("\t".join(w.get("regions", ["us-west-2", "us-east-2", "eu-west-1", "ap-south-1"])))
    if svc in ("ec2", "rds", "elasticache", "logs", "elbv2", "kms", "secretsmanager", "ssm", "cloudformation", "iam", "s3api") \
            and op.split("-")[0] in ("describe", "list", "get"):
        return out("")
    unexpected("aws", args)


def tags_of(w, arn):
    parts = arn.split(":")
    region, kind, ident = parts[3], parts[5], parts[-1]
    if parts[2] == "ecr":
        return w["ecr"].get(region, {}).get(arn.split(":repository/")[1], {})
    table = {"db": "rds", "subgrp": "subnet_groups", "serverlesscache": "caches", "serverlesscachesnapshot": "snapshots"}[kind]
    return w[table].get(region, {}).get(ident, {}).get("tags", {})


def cli_delete(w, table, region, ident):
    store = w[table].get(region, {})
    if ident not in store:
        not_found("NotFound", "Delete")
    if table in ("rds", "caches"):
        store[ident]["status"] = "deleting"
    elif table == "subnet_groups" and any(d["vpc"] == store[ident]["vpc"] for d in w["rds"].get(region, {}).values()):
        fail("An error occurred (InvalidDBSubnetGroupStateFault): the subnet group is in use", 254)
    else:
        del store[ident]
    event(w, f"CLI deleted {table} {region} {ident}")
    return out("{}")


# -------------------------------------------------------------------------- terraform
PLAN_ADDRESSES = [l for l in """
aws_ec2_tag.cluster_security_group["auto-delete"]
aws_ec2_tag.cluster_security_group["project"]
aws_ec2_tag.cluster_security_group["talk"]
aws_eks_access_entry.admin
aws_eks_access_entry.capability["ack"]
aws_eks_access_entry.capability["argocd"]
aws_eks_access_entry.kro
aws_eks_access_entry.node
aws_eks_access_policy_association.admin
aws_eks_capability.ack
aws_eks_capability.argocd
aws_eks_capability.kro
aws_eks_cluster.hub
aws_iam_role.capability["ack"]
aws_iam_role.cluster
aws_iam_role.node
aws_iam_role_policy.ack
aws_iam_role_policy_attachment.node["AmazonEKSWorkerNodeMinimalPolicy"]
aws_security_group.db
aws_vpc_security_group_ingress_rule.db["postgres"]
helm_release.nodes
kubernetes_config_map_v1.aws
kubernetes_namespace_v1.encabulator
module.vpc.aws_nat_gateway.this[0]
module.vpc.aws_route.private_nat_gateway[0]
module.vpc.aws_route_table.private[0]
module.vpc.aws_route_table_association.private[0]
module.vpc.aws_subnet.private[0]
module.vpc.aws_vpc.this[0]
random_password.postgres
""".split("\n") if l]


def before(address, account, region):
    t = re.sub(r"\[[^\]]*\]", "", address).split(".")[-2]
    tags = dict(TAGS)
    b = {}
    arn = lambda svc, res, reg=region: f"arn:aws:{svc}:{reg}:{account}:{res}"
    if t == "aws_vpc":
        b = {"id": "vpc-hub", "arn": arn("ec2", "vpc/vpc-hub"), "tags_all": dict(tags, Name=HUB)}
    elif t == "aws_subnet":
        b = {"id": "subnet-h1", "vpc_id": "vpc-hub", "arn": arn("ec2", "subnet/subnet-h1"), "tags_all": dict(tags, Name=f"{HUB}-private-a")}
    elif t == "aws_route_table":
        b = {"id": "rtb-hub", "vpc_id": "vpc-hub", "arn": arn("ec2", "route-table/rtb-hub"), "tags_all": dict(tags, Name=f"{HUB}-private")}
    elif t == "aws_route":
        b = {"route_table_id": "rtb-hub"}
    elif t == "aws_route_table_association":
        b = {"route_table_id": "rtb-hub", "subnet_id": "subnet-h1"}
    elif t == "aws_nat_gateway":
        b = {"id": "nat-1", "tags_all": dict(tags, Name=HUB)}
    elif t == "aws_eks_cluster":
        b = {"name": HUB, "arn": arn("eks", f"cluster/{HUB}"), "tags_all": tags, "vpc_config": [{"cluster_security_group_id": "sg-cluster"}]}
    elif t.startswith("aws_eks_"):
        b = {"cluster_name": HUB, "arn": arn("eks", f"access-entry/{HUB}/x"), "tags_all": tags} if t != "aws_eks_access_policy_association" else {"cluster_name": HUB}
    elif t == "aws_iam_role":
        name = {"aws_iam_role.cluster": f"{HUB}-cluster", "aws_iam_role.node": f"{HUB}-node"}.get(address, f"{HUB}-ack")
        b = {"name": name, "arn": f"arn:aws:iam::{account}:role/{name}", "tags_all": tags}
    elif t in ("aws_iam_role_policy", "aws_iam_role_policy_attachment"):
        b = {"role": f"{HUB}-node" if "node" in address else f"{HUB}-ack"}
    elif t == "aws_security_group":
        b = {"id": "sg-hubdb", "name": "encabulator-db", "vpc_id": "vpc-hub", "arn": arn("ec2", "security-group/sg-hubdb"), "tags_all": tags}
    elif t == "aws_vpc_security_group_ingress_rule":
        b = {"security_group_id": "sg-hubdb", "tags_all": tags}
    elif t == "aws_ec2_tag":
        b = {"resource_id": "sg-cluster"}
    elif t == "aws_db_instance":
        b = {"identifier": "encabulator-pg", "arn": arn("rds", "db:encabulator-pg"), "tags_all": tags}
    elif t in ("aws_db_subnet_group", "aws_elasticache_serverless_cache"):
        b = {"name": "encabulator" if t == "aws_db_subnet_group" else "encabulator-valkey",
             "arn": arn("rds" if t == "aws_db_subnet_group" else "elasticache", "x:y"), "tags_all": tags}
    return b


def make_plan(w):
    v = w.get("plan_variant", "")
    account = "444455556666" if v == "other-account" else w["account"]
    region = "eu-west-1" if v == "other-region" else "us-west-2"
    addresses = list(w["tf_state"])
    if v == "foreign":
        addresses.append("aws_instance.someone_elses")
    changes = []
    for addr in addresses:
        base = re.sub(r"\[[^\]]*\]", "", addr)
        t = base.split(".")[-2]
        b = before(addr, account, region)
        if v == "untagged" and t == "aws_vpc":
            b["tags_all"].pop("auto-delete")
        if v == "other-name" and t == "aws_vpc":
            b["tags_all"]["Name"] = "prod-vpc"
        provider = {"aws": "aws", "kubernetes": "kubernetes", "helm": "helm", "random": "random"}.get(t.split("_")[0], "aws")
        actions = ["delete", "create"] if (v == "replace" and t == "aws_vpc") else ["delete"]
        changes.append({"address": addr, "mode": "managed", "type": t, "provider_name": f"registry.terraform.io/hashicorp/{provider}",
                        "change": {"actions": actions, "before": b}})
    root = sorted({re.sub(r"\[[^\]]*\]", "", a) for a in PLAN_ADDRESSES + ["aws_db_instance.pg", "aws_db_subnet_group.encabulator",
                                                                           "aws_elasticache_serverless_cache.valkey"]})
    config = {"root_module": {"resources": [{"address": a, "mode": "managed"} for a in root if not a.startswith("module.")],
                              "module_calls": {"vpc": {"module": {"resources": [{"address": a[len("module.vpc."):], "mode": "managed"}
                                                                                 for a in root if a.startswith("module.vpc.")]}}}}}
    return {"resource_changes": changes, "configuration": config}


# What terraform apply prints while it destroys the hub, for the resources that show its timing:
# (address, id, start second, seconds it takes). The capabilities go first, the cluster after.
APPLY_TIMELINE = [
    ("aws_eks_access_entry.admin", f"{HUB}:arn:aws:iam::{ACCOUNT}:role/Admin", 0, 1),
    ("aws_eks_capability.argocd", None, 0, 110),
    ("module.vpc.aws_nat_gateway.this[0]", "nat-1", 0, 71),
    ("aws_eks_cluster.hub", HUB, 110, 250),
]
# How a version manager's shim fails when no version is set (mise 2026.9).
NO_VERSION = """mise ERROR No version is set for shim: terraform
Set a global default version with one of the following:
mise use -g terraform@1.14.9
mise ERROR Version: 2026.9.18 linux-x64 (2026-09-30)
mise ERROR Run with --verbose or MISE_VERBOSE=1 for more information"""


def apply_lines(go_format=False, stop_at=None):
    """The lines of APPLY_TIMELINE in time order. Every 10 seconds a resource still going prints
    Still destroying with the time so far: [01m10s elapsed] as Terraform 1.14 prints it, [1m10s
    elapsed] with go_format, as older Terraform versions do. stop_at ends the timeline at that second."""
    def elapsed(s):
        m, sec = divmod(s, 60)
        if go_format:
            return f"{m}m{sec}s" if m else f"{sec}s"
        return f"{m:02d}m{sec:02d}s"

    def took(s):
        m, sec = divmod(s, 60)
        return f"{m}m{sec}s" if m else f"{sec}s"

    events = []
    for i, (addr, ident, start, length) in enumerate(APPLY_TIMELINE):
        id_part = f"id={ident}, " if ident else ""
        events.append((start, i, 0, f"{addr}: Destroying..." + (f" [id={ident}]" if ident else "")))
        for s in range(10, length, 10):
            events.append((start + s, i, 1, f"{addr}: Still destroying... [{id_part}{elapsed(s)} elapsed]"))
        events.append((start + length, i, 2, f"{addr}: Destruction complete after {took(length)}"))
    return [line for t, _, _, line in sorted(events) if stop_at is None or t < stop_at]


def terraform(w, args):
    if args[:1] == ["version"]:
        return out("Terraform v1.14.9")
    if not args or not args[0].startswith("-chdir="):
        unexpected("terraform", args)
    tf_dir = args[0][len("-chdir="):]
    a = args[1:]
    if a[:2] == ["state", "list"]:
        return out("\n".join(w["tf_state"]))
    if a[:2] == ["state", "pull"]:
        return out(json.dumps({"version": 4, "resources": len(w["tf_state"]), "secret": "generated-password-stub"}))
    if a[:1] == ["plan"]:
        path = next(x.split("=", 1)[1] for x in a if x.startswith("-out="))
        with open(path, "w", encoding="utf-8") as f:
            json.dump(make_plan(w), f)
        return out(f"Plan: 0 to add, 0 to change, {len(w['tf_state'])} to destroy.")
    if a[:2] == ["show", "-json"]:
        with open(a[2], encoding="utf-8") as f:
            return out(f.read())
    if a[:1] == ["apply"]:
        n = len(w["tf_state"])
        go_format = w.get("tf_elapsed") == "go"
        if w.get("apply") == "fail":
            # The cluster fails a minute in; nothing of the hub is deleted.
            for line in apply_lines(go_format, stop_at=175):
                print(line, flush=True)
            fail("\nError: deleting EKS Cluster (encabulator-usw2): operation error EKS: DeleteCluster, "
                 "https response error StatusCode: 409, ResourceInUseException: Cluster has resources attached\n\n"
                 "  with aws_eks_cluster.hub,\n  on main.tf line 1, in resource \"aws_eks_cluster\" \"hub\":\n", 1)
        for line in apply_lines(go_format):
            print(line, flush=True)
        if "aws_db_instance.pg" in w["tf_state"]:
            # Before the hand-off Terraform owns the stores; the cache's snapshots outlive it.
            for table, name in (("rds", "encabulator-pg"), ("caches", "encabulator-valkey"), ("subnet_groups", "encabulator")):
                w[table]["us-west-2"].pop(name, None)
        w["tf_state"] = []
        w["eks"]["us-west-2"].pop(HUB, None)
        w["vpcs"]["us-west-2"].pop("vpc-hub", None)
        w["k8s"] = {x: o for x, o in w["k8s"].items() if not x.startswith(f"{HUB}|")}
        for r in [r for r in w["roles"] if r.startswith(HUB + "-")]:
            del w["roles"][r]
        w["kube"]["current"] = HUB
        state = os.path.join(tf_dir, "terraform.tfstate")
        with open(state, "w", encoding="utf-8") as f:
            f.write('{"version": 4, "resources": []}\n')
        os.chmod(state, 0o644)
        event(w, "terraform destroyed the hub")
        return out(f"Apply complete! Resources: 0 added, 0 changed, {n} destroyed.")
    unexpected("terraform", args)


# ------------------------------------------------------------------------------- helm
def helm(w, args):
    if args[:1] == ["status"]:
        if f"{opt(args, '--namespace')}/{args[1]}" not in w["helm"]:
            fail("Error: release: not found")
        return out("STATUS: deployed")
    if args[:1] == ["uninstall"]:
        ns = opt(args, "--namespace") or opt(args, "-n")
        key = f"{ns}/{args[1]}"
        if key not in w["helm"]:
            fail(f"Error: uninstall: Release not loaded: {args[1]}: release: not found")
        del w["helm"][key]
        return out(f'release "{args[1]}" uninstalled')
    unexpected("helm", args)


# ------------------------------------------------------------------------------- main
def main():
    tool = os.path.basename(sys.argv[0])
    args = sys.argv[1:]
    if tool in ("stub.py",) and args[:1] == ["init"]:
        init(dict(x.split("=", 1) for x in args[1:]))
        return 0
    if tool in ("stub.py",) and args[:1] == ["plan"]:
        # The JSON `terraform show -json` gives for a destroy plan of the hub (stub.py plan [variant]).
        w = {"account": ACCOUNT, "tf_state": list(PLAN_ADDRESSES), "plan_variant": (args + [""])[1]}
        if w["plan_variant"] == "empty":
            w["tf_state"] = []
        print(json.dumps(make_plan(w)))
        return 0
    with open(os.environ["STUB_LOG"], "a", encoding="utf-8") as f:
        f.write(f"{tool} {' '.join(args)}\n")
    w = load()
    line = f"{tool} {' '.join(args)}"
    try:
        once = os.environ.get("STUB_FAIL_ONCE")
        if once and re.search(once, line) and once not in w["failed_once"]:
            w["failed_once"].append(once)
            fail("An error occurred (ExpiredToken) when calling the operation: The security token included in the request is expired", 254)
        if os.environ.get("STUB_FAIL") and re.search(os.environ["STUB_FAIL"], line):
            fail("An error occurred (AccessDenied) when calling the operation: stub", 254)
        if tool == "terraform" and os.environ.get("STUB_TF_NO_VERSION"):
            fail(NO_VERSION, 1)
        {"aws": aws, "kubectl": kubectl, "helm": helm, "terraform": terraform}[tool](w, args)
        rc = 0
    except Done as d:
        rc = d.rc
    save(w)
    return rc


if __name__ == "__main__":
    sys.exit(main())
