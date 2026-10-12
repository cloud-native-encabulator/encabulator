#!/usr/bin/env python3
"""Prove the RBAC in gitops/app/rbac is enough for kro, without trusting kro's own install.

Self-managed kro often runs with broad permissions (the test cluster's does), so kro
succeeding proves nothing about gitops/app/rbac. This replays kro's work through
impersonation instead, as a check-only group bound to exactly the ClusterRoles in
gitops/app/rbac/kro.yaml (temporary copies of its bindings, removed at the end), so other
bindings on a shared cluster can neither make a check pass nor make one fail:

  1. `kubectl auth can-i` get/list/watch, cluster-wide, on every kind kro manages
  2. a server-side dry-run apply of every object kro created (label kro.run/owned=true):
     the API server runs the same authorization, admission and RBAC escalation checks
     (a Role kro creates may only grant what kro holds) and writes nothing
  3. `kubectl auth can-i` create and delete for each of those objects
  4. what must be refused: reading Secrets, writing outside namespace encabulator, other
     StorageClasses, ClusterRoles

Needs an identity allowed to impersonate and to create RBAC (cluster admin on kind), and
gitops/app/rbac already applied.

Usage: gitops/app/hack/rbac-check.py --kubeconfig PATH [--namespace encabulator]
"""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

import yaml

RBAC = Path(__file__).resolve().parents[1] / "rbac" / "kro.yaml"
REAL_GROUP = "encabulator:kro"
CHECK_GROUP = "encabulator:kro:rbac-check"
USER = "encabulator-kro-rbac-check"

RESOURCE_OF = {
    # kind: (resource, apiGroup) for every kind the two RGDs create
    "ConfigMap": ("configmaps", ""), "Service": ("services", ""), "ServiceAccount": ("serviceaccounts", ""),
    "Deployment": ("deployments", "apps"), "StatefulSet": ("statefulsets", "apps"), "Job": ("jobs", "batch"),
    "HorizontalPodAutoscaler": ("horizontalpodautoscalers", "autoscaling"),
    "PodDisruptionBudget": ("poddisruptionbudgets", "policy"),
    "Role": ("roles", "rbac.authorization.k8s.io"), "RoleBinding": ("rolebindings", "rbac.authorization.k8s.io"),
    "DBInstance": ("dbinstances", "rds.services.k8s.aws"), "DBSubnetGroup": ("dbsubnetgroups", "rds.services.k8s.aws"),
    "ServerlessCache": ("serverlesscaches", "elasticache.services.k8s.aws"),
    "Encabulator": ("encabulators", "kro.run"), "EncabulatorService": ("encabulatorservices", "kro.run"),
    "StorageClass": ("storageclasses", "storage.k8s.io"),
}
CLUSTER_SCOPED = {"storageclasses"}
DROP_METADATA = ("uid", "resourceVersion", "generation", "creationTimestamp", "managedFields",
                 "selfLink", "ownerReferences", "finalizers", "deletionTimestamp")


def check_bindings():
    """Copies of the bindings in rbac/kro.yaml with the check-only group as the subject."""
    out = []
    for doc in yaml.safe_load_all(RBAC.read_text()):
        if doc and doc["kind"] in ("ClusterRoleBinding", "RoleBinding"):
            assert [s["name"] for s in doc["subjects"]] == [REAL_GROUP], doc["metadata"]["name"]
            doc["metadata"]["name"] += ":rbac-check"
            doc["metadata"].pop("annotations", None)
            doc["subjects"] = [{"apiGroup": "rbac.authorization.k8s.io", "kind": "Group", "name": CHECK_GROUP}]
            out.append(doc)
    return out


def static_checks(failures):
    for doc in yaml.safe_load_all(RBAC.read_text()):
        if not doc or doc["kind"] != "ClusterRole":
            continue
        for rule in doc["rules"]:
            for field in ("apiGroups", "resources", "verbs"):
                if "*" in rule.get(field, []):
                    failures.append(f"{doc['metadata']['name']}: wildcard in {field}")
            if "secrets" in rule["resources"] and set(rule["verbs"]) & {"list", "watch"}:
                failures.append(f"{doc['metadata']['name']}: list/watch on secrets")
    print(f"{'ok  ' if not failures else 'FAIL'} static: no wildcards, no list/watch on secrets in {RBAC.name}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--kubeconfig", required=True)
    parser.add_argument("--namespace", default="encabulator")
    args = parser.parse_args()
    base = ["kubectl", "--kubeconfig", args.kubeconfig]
    as_kro = ["--as", USER, "--as-group", CHECK_GROUP]
    ns = args.namespace
    failures = []

    static_checks(failures)
    bindings = check_bindings()
    manifest = json.dumps({"apiVersion": "v1", "kind": "List", "items": bindings})
    subprocess.run(base + ["apply", "--server-side", "--field-manager=kro-rbac-check", "-f", "-"],
                   input=manifest, check=True, capture_output=True, text=True)
    print(f"bound {len(bindings)} temporary copies of the rbac/kro.yaml bindings to group {CHECK_GROUP}")

    def can_i(verb, resource, namespace=None, name=None, expect=True):
        target = f"{resource}/{name}" if name else resource
        cmd = base + ["auth", "can-i", verb, target] + as_kro
        cmd += ["-n", namespace] if namespace else ["--all-namespaces"]
        answer = subprocess.run(cmd, capture_output=True, text=True).stdout.strip()
        ok = (answer == "yes") == expect
        where = namespace or "all namespaces"
        print(f"{'ok  ' if ok else 'FAIL'} can-i {verb:6} {target} in {where}: {answer} (want {'yes' if expect else 'no'})")
        if not ok:
            failures.append(f"can-i {verb} {target} {where}")

    try:
        for resource, group in RESOURCE_OF.values():
            qualified = f"{resource}.{group}" if group else resource
            for verb in ("get", "list", "watch"):
                can_i(verb, qualified)

        kinds = ",".join(f"{r}.{g}" if g else r for r, g in RESOURCE_OF.values() if r not in CLUSTER_SCOPED)
        owned = json.loads(subprocess.run(base + ["-n", ns, "get", kinds, "-l", "kro.run/owned=true", "-o", "json"],
                                          check=True, capture_output=True, text=True).stdout)["items"]
        owned += json.loads(subprocess.run(base + ["get", "storageclasses", "-l", "kro.run/owned=true", "-o", "json"],
                                           check=True, capture_output=True, text=True).stdout)["items"]
        print(f"replaying {len(owned)} kro-owned objects as group {CHECK_GROUP}")
        for obj in owned:
            obj.pop("status", None)
            for key in DROP_METADATA:
                obj["metadata"].pop(key, None)
            kind, name = obj["kind"], obj["metadata"]["name"]
            proc = subprocess.run(base + ["apply", "--server-side", "--dry-run=server", "--force-conflicts",
                                          "--field-manager=kro-rbac-check", "-f", "-"] + as_kro,
                                  input=json.dumps(obj), capture_output=True, text=True)
            if proc.returncode != 0:
                failures.append(f"apply {kind}/{name}: {proc.stderr.strip()}")
                print(f"FAIL apply   {kind}/{name}: {proc.stderr.strip()}")
            else:
                print(f"ok   apply   {kind}/{name}")
            resource, group = RESOURCE_OF[kind]
            qualified = f"{resource}.{group}" if group else resource
            scope = None if resource in CLUSTER_SCOPED else ns
            can_i("create", qualified, scope)
            can_i("delete", qualified, scope, name)

        # What must be refused.
        can_i("get", "secrets", ns, expect=False)
        can_i("list", "secrets", expect=False)
        can_i("get", "secrets", ns, "some-other-secret", expect=False)
        can_i("create", "deployments.apps", "default", expect=False)
        can_i("delete", "configmaps", "kube-system", expect=False)
        can_i("delete", "storageclasses.storage.k8s.io", None, "standard", expect=False)
        can_i("create", "clusterroles.rbac.authorization.k8s.io", expect=False)
        can_i("escalate", "roles.rbac.authorization.k8s.io", ns, expect=False)
    finally:
        subprocess.run(base + ["delete", "--ignore-not-found", "-f", "-"], input=manifest,
                       capture_output=True, text=True)
        print("removed the temporary bindings")

    print(f"{len(failures)} failure(s)")
    for failure in failures:
        print(f"  {failure}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
