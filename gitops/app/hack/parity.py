#!/usr/bin/env python3
"""Compare what the kro RGDs created in a cluster against `helm template charts/encabulator`.

The chart is the reference. For every service the chart renders, this checks the objects
kro created in namespace encabulator:

  Deployment      env var names in the same order; every env entry, container setting
                  (image, ports, probes, resources, security context, volume mounts), pod
                  setting and deployment strategy the chart sets, with the same value
  ServiceAccount, Service, HorizontalPodAutoscaler, PodDisruptionBudget, Role, RoleBinding
                  present exactly where the chart renders one, same spec
  labels          the chart's labels, except managed-by (kro, not Helm) and helm.sh/chart

and, for the stores: the NATS and InfluxDB StatefulSets and Services, the nats-config
ConfigMap, the gp3 StorageClass, and the key names (never the values) of the Secrets; and the
grammeter's settings ConfigMap.

Live objects carry API server defaults the chart never writes (successThreshold, fieldRef
apiVersion, ...), so "same" means every field the chart sets is present with the same
value; quantities compare by value (1000m == 1). Env var names must match exactly.

Deliberately not compared, because the cell does not render them: ServiceMonitor and
PrometheusRule (no Prometheus Operator on a cell), the in-cluster postgres and valkey
StatefulSets (RDS and ElastiCache replace them), and the Secrets the chart only renders
when it generates them itself.

Usage:
  gitops/app/hack/parity.py --kubeconfig PATH [--namespace encabulator] [--report FILE]

Needs helm, kubectl and PyYAML. Exits 0 when everything matches, 1 otherwise.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from decimal import Decimal
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[3]
CHART = REPO / "charts" / "encabulator"
CELL_VALUES = Path(__file__).resolve().parent / "cell-values.yaml"

# Label differences that are the point of the exercise, not drift.
LABEL_EXPECT = {"app.kubernetes.io/managed-by": "kro"}
LABEL_DROP = {"helm.sh/chart"}

# Kinds rendered per service, and how to compare their spec.
PER_SERVICE = ["ServiceAccount", "Service", "HorizontalPodAutoscaler", "PodDisruptionBudget", "Role", "RoleBinding"]
STORE_OBJECTS = [
    ("StatefulSet", "nats"), ("StatefulSet", "influxdb"),
    ("Service", "nats"), ("Service", "nats-headless"),
    ("Service", "influxdb"), ("Service", "influxdb-headless"),
    ("ConfigMap", "nats-config"), ("StorageClass", "encabulator-gp3"),
    # Not a store, but outside the per-service set: where the grammeter keeps its traffic
    # settings. Both sides create it without data, so labels are what is compared.
    ("ConfigMap", "cardinal-grammeter-settings"),
]
# The chart only renders these with existingSecret unset; the cell's names and keys are fixed
# by the contract.
CONTRACT_SECRET_KEYS = {
    "encabulator-db": {"postgres-password", "hydrocoptic-password", "detractor-password", "girdlespring-password"},
    "encabulator-valkey": {"valkey-password"},
}

QUANTITY = re.compile(r"^([+-]?[0-9.]+)([a-zA-Z]*)$")
SUFFIX = {
    "": 1, "m": Decimal("0.001"), "k": 10**3, "M": 10**6, "G": 10**9, "T": 10**12,
    "Ki": 2**10, "Mi": 2**20, "Gi": 2**30, "Ti": 2**40,
}


def quantity(value):
    match = QUANTITY.match(str(value))
    if not match or match.group(2) not in SUFFIX:
        return None
    return Decimal(match.group(1)) * SUFFIX[match.group(2)]


def run(cmd, env=None):
    proc = subprocess.run(cmd, check=True, capture_output=True, text=True, env=env)
    return proc.stdout


def subset(expected, actual, path, problems, quantities=False):
    """Every field in expected is in actual with the same value."""
    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            problems.append(f"{path}: expected an object, got {actual!r}")
            return
        for key, value in expected.items():
            if key not in actual:
                problems.append(f"{path}.{key}: missing (chart has {value!r})")
                continue
            subset(value, actual[key], f"{path}.{key}", problems,
                   quantities or key in ("resources", "sizeLimit", "storage"))
    elif isinstance(expected, list):
        if not isinstance(actual, list) or len(actual) != len(expected):
            problems.append(f"{path}: expected {len(expected)} items, got {actual!r}")
            return
        for i, (exp, act) in enumerate(zip(expected, actual)):
            subset(exp, act, f"{path}[{i}]", problems, quantities)
    else:
        if quantities and quantity(expected) is not None and quantity(expected) == quantity(actual):
            return
        if expected != actual:
            problems.append(f"{path}: chart {expected!r}, kro {actual!r}")


def by_name(items):
    return {item["name"]: item for item in items or []}


def expected_labels(labels):
    out = {k: v for k, v in (labels or {}).items() if k not in LABEL_DROP}
    for key, value in LABEL_EXPECT.items():
        if key in out:
            out[key] = value
    return out


def compare_labels(chart, live, path, problems):
    exp = expected_labels(chart)
    # kro sets managed-by on every object it owns; pod templates carry ours explicitly.
    subset(exp, live or {}, path, problems)


def compare_named_list(chart, live, path, problems, key="name"):
    """Lists keyed by name, compared in the chart's order."""
    chart, live = chart or [], live or []
    names_chart = [item[key] for item in chart]
    names_live = [item[key] for item in live]
    if names_chart != names_live:
        problems.append(f"{path} names: chart {names_chart}, kro {names_live}")
        return
    for item_chart, item_live in zip(chart, live):
        subset(item_chart, item_live, f"{path}[{item_chart[key]}]", problems)


def compare_pod_template(chart, live, path, problems):
    compare_labels(chart["metadata"].get("labels"), live["metadata"].get("labels"), f"{path}.metadata.labels", problems)
    subset(chart["metadata"].get("annotations") or {}, live["metadata"].get("annotations") or {},
           f"{path}.metadata.annotations", problems)
    cspec, lspec = chart["spec"], live["spec"]
    for field in ("serviceAccountName", "automountServiceAccountToken", "securityContext",
                  "terminationGracePeriodSeconds", "nodeSelector", "tolerations", "affinity"):
        if field in cspec:
            subset(cspec[field], lspec.get(field), f"{path}.spec.{field}", problems)
    compare_named_list(cspec.get("volumes"), lspec.get("volumes"), f"{path}.spec.volumes", problems)
    containers_chart, containers_live = by_name(cspec["containers"]), by_name(lspec["containers"])
    if list(containers_chart) != list(containers_live):
        problems.append(f"{path}.spec.containers: chart {list(containers_chart)}, kro {list(containers_live)}")
        return
    for name, cc in containers_chart.items():
        lc = containers_live[name]
        cpath = f"{path}.containers[{name}]"
        env_chart = [e["name"] for e in cc.get("env", [])]
        env_live = [e["name"] for e in lc.get("env", [])]
        if env_chart != env_live:
            missing = [n for n in env_chart if n not in env_live]
            extra = [n for n in env_live if n not in env_chart]
            problems.append(f"{cpath}.env names differ: missing {missing}, extra {extra}, "
                            f"chart order {env_chart}, kro order {env_live}")
        else:
            compare_named_list(cc.get("env"), lc.get("env"), f"{cpath}.env", problems)
        for field, value in cc.items():
            if field in ("env", "name"):
                continue
            if field in ("volumeMounts", "ports"):
                compare_named_list(value, lc.get(field), f"{cpath}.{field}", problems)
            else:
                subset(value, lc.get(field), f"{cpath}.{field}", problems, field == "resources")


def compare_workload(chart, live, problems):
    kind, name = chart["kind"], chart["metadata"]["name"]
    path = f"{kind}/{name}"
    compare_labels(chart["metadata"].get("labels"), live["metadata"].get("labels"), f"{path}.metadata.labels", problems)
    cspec, lspec = chart["spec"], live["spec"]
    for field, value in cspec.items():
        if field == "template":
            compare_pod_template(value, lspec["template"], f"{path}.spec.template", problems)
        elif field == "volumeClaimTemplates":
            for i, (vc, vl) in enumerate(zip(value, lspec.get(field, []))):
                compare_labels(vc["metadata"].get("labels"), vl["metadata"].get("labels"),
                               f"{path}.spec.{field}[{i}].metadata.labels", problems)
                subset(vc["spec"], vl["spec"], f"{path}.spec.{field}[{i}].spec", problems)
            if len(value) != len(lspec.get(field, [])):
                problems.append(f"{path}.spec.{field}: count differs")
        else:
            subset(value, lspec.get(field), f"{path}.spec.{field}", problems)
    # The chart leaves replicas out when an autoscaler owns them; so must kro.
    if "replicas" not in cspec and kind == "Deployment":
        hpa_owned = live.get("_hpa_owned")
        if not hpa_owned:
            problems.append(f"{path}: chart leaves replicas to the HPA but kro has no HPA for it")


def compare_generic(chart, live, problems):
    kind, name = chart["kind"], chart["metadata"]["name"]
    path = f"{kind}/{name}"
    compare_labels(chart["metadata"].get("labels"), live["metadata"].get("labels"), f"{path}.metadata.labels", problems)
    for field in ("spec", "rules", "roleRef", "subjects", "data", "automountServiceAccountToken",
                  "provisioner", "parameters", "reclaimPolicy", "allowVolumeExpansion", "volumeBindingMode"):
        if field in chart:
            subset(chart[field], live.get(field), f"{path}.{field}", problems)


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--kubeconfig", required=True)
    parser.add_argument("--namespace", default="encabulator")
    parser.add_argument("--report", help="also write the report to this file")
    parser.add_argument("--helm-set", action="append", default=[], metavar="KEY=VALUE",
                        help="extra helm --set, to prove the check fails on a real difference")
    args = parser.parse_args()

    kubectl = ["kubectl", "--kubeconfig", args.kubeconfig]
    ns = args.namespace

    stores = json.loads(run(kubectl + ["-n", ns, "get", "configmap", "encabulator-aws", "-o", "json"]))["data"]
    helm_env = dict(os.environ, KUBECONFIG=os.devnull)  # `lookup` must not reach a cluster
    rendered = run(["helm", "template", "encabulator", str(CHART), "--namespace", ns,
                    "-f", str(CELL_VALUES),
                    "--set", f"postgres.external.host={stores['POSTGRES_HOST']}",
                    "--set", f"postgres.external.port={stores['POSTGRES_PORT']}",
                    "--set", f"valkey.external.host={stores['VALKEY_HOST']}",
                    "--set", f"valkey.external.port={stores['VALKEY_PORT']}"]
                   + [arg for value in args.helm_set for arg in ("--set", value)], env=helm_env)
    chart_objects = [d for d in yaml.safe_load_all(rendered) if d]
    chart_index = {(d["kind"], d["metadata"]["name"]): d for d in chart_objects}

    kinds = "deployments,statefulsets,services,serviceaccounts,horizontalpodautoscalers," \
            "poddisruptionbudgets,roles,rolebindings,configmaps"
    live_items = json.loads(run(kubectl + ["-n", ns, "get", kinds, "-o", "json"]))["items"]
    live_items += json.loads(run(kubectl + ["get", "storageclass", "-o", "json"]))["items"]
    live_index = {(d["kind"], d["metadata"]["name"]): d for d in live_items}
    hpa_targets = {d["spec"]["scaleTargetRef"]["name"] for d in live_items if d["kind"] == "HorizontalPodAutoscaler"}

    lines, failed = [], 0
    services = sorted(name for kind, name in chart_index if kind == "Deployment")

    def check(kind, name):
        problems = []
        chart = chart_index.get((kind, name))
        live = live_index.get((kind, name))
        if chart is None and live is None:
            return None
        if chart is None:
            # kro objects the chart does not render for this service (and kro's own helpers).
            return [f"{kind}/{name}: created by kro but not rendered by the chart"]
        if live is None:
            return [f"{kind}/{name}: rendered by the chart but missing in the cluster"]
        if kind in ("Deployment", "StatefulSet"):
            live = dict(live, _hpa_owned=name in hpa_targets)
            compare_workload(chart, live, problems)
        else:
            compare_generic(chart, live, problems)
        return problems

    for svc in services:
        problems = []
        for kind in ["Deployment"] + PER_SERVICE:
            result = check(kind, svc)
            if result:
                problems += result
        env = [e["name"] for e in
               chart_index[("Deployment", svc)]["spec"]["template"]["spec"]["containers"][0].get("env", [])]
        kinds_present = [k for k in PER_SERVICE if (k, svc) in chart_index]
        if problems:
            failed += 1
            lines.append(f"FAIL {svc}")
            lines += [f"     {p}" for p in problems]
        else:
            lines.append(f"ok   {svc}: {len(env)} env vars match in order; Deployment + {', '.join(kinds_present)} match")

    for kind, name in STORE_OBJECTS:
        problems = check(kind, name) or []
        if problems:
            failed += 1
            lines.append(f"FAIL {kind}/{name}")
            lines += [f"     {p}" for p in problems]
        else:
            lines.append(f"ok   {kind}/{name}")

    # Secrets: compare key names only. Values are never read into the report.
    wanted_keys = dict(CONTRACT_SECRET_KEYS)
    for (kind, name), doc in chart_index.items():
        if kind == "Secret":
            wanted_keys[name] = set((doc.get("data") or {}).keys())
    for name, keys in sorted(wanted_keys.items()):
        live = json.loads(run(kubectl + ["-n", ns, "get", "secret", name, "-o", "json"]))
        live_keys = set((live.get("data") or {}).keys())
        if keys - live_keys:
            failed += 1
            lines.append(f"FAIL Secret/{name}: missing keys {sorted(keys - live_keys)}")
        else:
            lines.append(f"ok   Secret/{name}: keys {sorted(keys)}")

    lines.append(f"{len(services)} services compared; {failed} failure(s)")
    report = "\n".join(lines)
    print(report)
    if args.report:
        Path(args.report).write_text(report + "\n")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
