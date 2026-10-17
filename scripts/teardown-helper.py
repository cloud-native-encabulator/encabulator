#!/usr/bin/env python3
"""Helpers for scripts/teardown.sh. No AWS or cluster calls: text in, text out.

  teardown-helper.py render TEMPLATE KIND key=value...
      Print one object of docs/teardown/TEMPLATE (the document whose kind is KIND) with the
      values filled in, labelled encabulator.io/teardown=true. Keys:
        name, namespace    the Kubernetes object
        region             services.k8s.aws/region
        id                 the AWS identifier: adoption-fields and the spec field that names it
        stack              replaces encabulator-use2, the example cell in the templates
        subnets, sgs       comma-separated ids for the <subnet-id-N> and security group lists
        vpc, rtb           the <vpc-id> and <rtb-id> placeholders
      Fails when a placeholder (<...>) is left, so a half-filled object is never applied.

  teardown-helper.py leaked-route-tables NAME TRACKED-IDS < describe-route-tables JSON
      Print the id of every route table that is an untracked leftover of a cell built before
      the NAT fix: Name tag NAME, the three project tags, no subnet association (and not the
      main table), only the local route, and not in TRACKED-IDS (space-separated ids the hub's
      RouteTable objects track). Every other table is left alone.
"""
import json
import re
import sys

CONTRACT = {"auto-delete": "no", "project": "encabulator", "talk": "opn315"}
# The spec field that holds the AWS identifier, per kind.
ID_FIELDS = {
    "DBInstance": "dbInstanceIdentifier",
    "ServerlessCache": "serverlessCacheName",
    "DBSubnetGroup": "name",
    "Role": "name",
}


def die(msg):
    print(f"teardown-helper: {msg}", file=sys.stderr)
    sys.exit(2)


def documents(text):
    doc = []
    for line in text.splitlines():
        if line.strip() == "---":
            if doc:
                yield doc
            doc = []
        else:
            doc.append(line)
    if doc:
        yield doc


def render(path, kind, values):
    with open(path, encoding="utf-8") as f:
        text = f.read()
    picked = None
    for doc in documents(text):
        if f"kind: {kind}" in doc:
            picked = doc
            break
    if picked is None:
        die(f"{path} has no {kind}")
    subnets = [s for s in values.get("subnets", "").split(",") if s]
    sgs = [s for s in values.get("sgs", "").split(",") if s]
    stack = values.get("stack")
    out, section = [], None
    subnets_done = sgs_done = False
    for line in picked:
        if line.lstrip().startswith("#") or not line.strip():
            continue
        if not line.startswith(" "):
            section = line.split(":", 1)[0]
        if stack:
            line = line.replace("encabulator-use2", stack)
        m = re.match(r"^(\s+)- <subnet-id-\d+>$", line)
        if m:
            if not subnets_done:
                if not subnets:
                    die(f"{kind} needs subnets=")
                out.extend(f"{m.group(1)}- {s}" for s in subnets)
                subnets_done = True
            continue
        m = re.match(r"^(\s+)- <encabulator-db-security-group-id>$", line)
        if m:
            if not sgs_done:
                if not sgs:
                    die(f"{kind} needs sgs=")
                out.extend(f"{m.group(1)}- {s}" for s in sgs)
                sgs_done = True
            continue
        if section == "metadata" and re.match(r"^  name: ", line) and "name" in values:
            line = f"  name: {values['name']}"
        elif section == "metadata" and re.match(r"^  namespace: ", line):
            out.append(f"  namespace: {values['namespace']}" if "namespace" in values else line)
            out.append("  labels:")
            out.append('    encabulator.io/teardown: "true"')
            continue
        elif "services.k8s.aws/region:" in line and "region" in values:
            line = re.sub(r"region: .*$", f"region: {values['region']}", line)
        elif "services.k8s.aws/adoption-fields:" in line and "id" in values:
            fields = json.loads(re.search(r"'(\{.*\})'", line).group(1))
            fields = {k: values["id"] for k in fields}
            line = re.sub(r"'\{.*\}'", "'" + json.dumps(fields, separators=(",", ":")) + "'", line)
        elif section == "spec" and "id" in values and re.match(rf"^  {ID_FIELDS.get(kind, '-')}: ", line):
            line = f"  {ID_FIELDS[kind]}: {values['id']}"
        for key, placeholder in (("vpc", "<vpc-id>"), ("rtb", "<rtb-id>"), ("region", "<region>")):
            if placeholder in line and key in values:
                line = line.replace(placeholder, values[key])
        out.append(line)
    left = [l for l in out if re.search(r"<[a-z0-9-]+>", l)]
    if left:
        die(f"placeholders left in {kind}: {left[0].strip()}")
    print("\n".join(out))


def leaked_route_tables(name, tracked, data):
    tracked = set(tracked.split())
    for rt in data.get("RouteTables", []):
        tags = {t["Key"]: t["Value"] for t in rt.get("Tags", [])}
        if tags.get("Name") != name or any(tags.get(k) != v for k, v in CONTRACT.items()):
            continue
        if rt.get("Associations"):
            continue
        if any(r.get("GatewayId") != "local" for r in rt.get("Routes", [])):
            continue
        if rt["RouteTableId"] in tracked:
            continue
        print(rt["RouteTableId"])


def main(argv):
    if len(argv) >= 3 and argv[0] == "render":
        values = {}
        for kv in argv[3:]:
            k, sep, v = kv.partition("=")
            if not sep:
                die(f"expected key=value, got {kv}")
            values[k] = v
        render(argv[1], argv[2], values)
        return 0
    if len(argv) == 3 and argv[0] == "leaked-route-tables":
        leaked_route_tables(argv[1], argv[2], json.load(sys.stdin))
        return 0
    print(__doc__.strip(), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
