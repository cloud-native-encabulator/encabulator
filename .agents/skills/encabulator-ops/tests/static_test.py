#!/usr/bin/env python3
"""Static checks for the encabulator-ops skill, offline, standard library only.

    python3 .agents/skills/encabulator-ops/tests/static_test.py

- SKILL.md frontmatter is valid in all three locations (.agents, and .claude and .kiro, which
  are symbolic links to .agents/skills, checked by scripts/check-agent-skills.sh)
- every make target and every repository path the skill names exists, and every link resolves
- every Kubernetes kind it names appears in the repository's manifests (or is a core kind)
- no account id other than 111122223333, no dashes as punctuation, no secrets, no internal
  hostnames or local home paths, and the health check script calls no command that changes anything
- AGENTS.md and CLAUDE.md say what they must; every fire drill has its world, its answer key and
  a test case
"""

import re
import subprocess
import sys
import unittest
from pathlib import Path

SKILL = Path(__file__).resolve().parents[1]
ROOT = SKILL.parents[2]
NAME = "encabulator-ops"
COPIES = [ROOT / d / "skills" / NAME for d in (".agents", ".claude", ".kiro")]

TEXT_SUFFIXES = {".md", ".sh", ".py", ".txt", ".json", ".expected"}


def skill_files(suffixes=TEXT_SUFFIXES):
    return sorted(p for p in SKILL.rglob("*") if p.is_file() and p.suffix in suffixes and "__pycache__" not in p.parts)


def own_prose_files():
    """The Markdown this change wrote: the skill's, AGENTS.md, CLAUDE.md."""
    return [p for p in skill_files({".md"})] + [ROOT / "AGENTS.md", ROOT / "CLAUDE.md"]


def readme_agent_section():
    text = (ROOT / "README.md").read_text(encoding="utf-8")
    m = re.search(r"^## Work with the project using an AI agent\n(.*?)^## ", text, re.S | re.M)
    return m.group(1) if m else ""


def code_spans(text):
    """Inline code spans and fenced code blocks of a Markdown text."""
    blocks = re.findall(r"^```[^\n]*\n(.*?)^```", text, re.S | re.M)
    rest = re.sub(r"^```[^\n]*\n.*?^```", "", text, flags=re.S | re.M)
    return blocks + re.findall(r"`([^`\n]+)`", rest)


def prose_lines(text):
    """Lines outside fenced code, with inline code removed."""
    out, fenced = [], False
    for line in text.splitlines():
        if line.startswith("```"):
            fenced = not fenced
            continue
        if not fenced:
            out.append(re.sub(r"`[^`]*`", "", line))
    return out


def frontmatter(path):
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] != "---":
        raise AssertionError(f"{path}: no frontmatter")
    end = lines.index("---", 1)
    fields = {}
    for line in lines[1:end]:
        key, sep, value = line.partition(":")
        if not sep or not re.fullmatch(r"[a-z][a-z-]*", key):
            raise AssertionError(f"{path}: frontmatter line is not 'key: value': {line!r}")
        fields[key] = value.strip()
    return fields, lines[end + 1:]


def make_targets():
    text = (ROOT / "Makefile").read_text(encoding="utf-8")
    return set(re.findall(r"^([a-zA-Z0-9_.-]+):(?!=)", text, re.M))


def repo_kinds():
    """(group, kind) pairs from every manifest and template in the repository."""
    pairs = set()
    for base in ("gitops", "charts", "platform", "terraform", "docs/teardown"):
        for path in (ROOT / base).rglob("*.yaml"):
            lines = path.read_text(encoding="utf-8").splitlines()
            for i, line in enumerate(lines):
                m = re.match(r"^(\s*)(?:- )?kind:\s*([A-Za-z]+)\s*$", line)
                if not m:
                    continue
                indent = len(m.group(1))
                for j in list(range(i - 1, max(i - 6, -1), -1)) + list(range(i + 1, min(i + 6, len(lines)))):
                    a = re.match(r"^(\s*)(?:- )?apiVersion:\s*([^\s]+)", lines[j])
                    if a and len(a.group(1)) == indent:
                        group = a.group(2).split("/")[0] if "/" in a.group(2) else ""
                        pairs.add((group, m.group(2)))
                        break
    return pairs


CORE_KINDS = {
    "Pod", "Node", "Event", "Secret", "ConfigMap", "Service", "Namespace", "Deployment",
    "StatefulSet", "ReplicaSet", "Job", "CronJob", "PersistentVolume", "PersistentVolumeClaim",
    "ResourceQuota", "HorizontalPodAutoscaler", "NetworkPolicy", "ServiceAccount", "NodeClaim",
}
NOT_KINDS = {
    "ImagePullBackOff", "ErrImagePull", "CrashLoopBackOff", "OutOfSync", "OOMKilled", "BackOff",
    "InsufficientInstanceCapacity", "InsufficientCapacityError", "ServerlessCacheNotFoundFault",
    "ExpiredToken", "DependencyViolation", "DeleteConflict", "VpcLimitExceeded",
    "AddressLimitExceeded", "ContainerCreating", "Synced", "Healthy", "Degraded", "Progressing",
    "Running", "Pending", "Failed", "Succeeded", "Active", "Inactive", "Modifying", "Unknown",
    "None",
}
# Files that exist only on a machine that ran the stack, never in Git.
RUNTIME_ONLY = (".tfstate", ".lock.info")


class Frontmatter(unittest.TestCase):
    def test_valid_in_all_three_locations(self):
        for copy in COPIES:
            with self.subTest(copy=str(copy.relative_to(ROOT))):
                fields, body = frontmatter(copy / "SKILL.md")
                self.assertEqual(set(fields), {"name", "description"})
                self.assertEqual(fields["name"], NAME)
                self.assertEqual(copy.name, fields["name"], "the folder is named after the skill")
                self.assertRegex(fields["name"], r"^[a-z0-9]+(-[a-z0-9]+)*$")
                self.assertLessEqual(len(fields["name"]), 64)
                self.assertTrue(0 < len(fields["description"]) <= 1024, len(fields["description"]))
                self.assertNotRegex(fields["description"], r"[<>]")
                self.assertTrue(any(line.startswith("# ") for line in body))

    def test_description_carries_the_triggers(self):
        fields, _ = frontmatter(SKILL / "SKILL.md")
        for word in ("Argo CD", "ACK", "kro", "EKS", "cell", "ImagePullBackOff", "CrashLoopBackOff",
                     "Degraded", "make hub", "CI", "explain", "troubleshoot"):
            self.assertIn(word, fields["description"])

    def test_skill_md_is_short(self):
        self.assertLessEqual(len((SKILL / "SKILL.md").read_text(encoding="utf-8").splitlines()), 120)

    def test_one_copy_two_links(self):
        for d in (".claude", ".kiro"):
            link = ROOT / d / "skills"
            self.assertTrue(link.is_symlink(), f"{d}/skills must be a symbolic link")
            self.assertEqual(str(link.readlink()), "../.agents/skills")
            self.assertEqual((link / NAME).resolve(), SKILL)
        r = subprocess.run([str(ROOT / "scripts/check-agent-skills.sh")], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        r = subprocess.run(["git", "-C", str(ROOT), "ls-files", "-s", ".claude/skills", ".kiro/skills"],
                           capture_output=True, text=True)
        modes = [line.split()[0] for line in r.stdout.splitlines()]
        if modes:  # once committed, Git stores both as links (mode 120000), not as folders
            self.assertEqual(modes, ["120000", "120000"], r.stdout)


class References(unittest.TestCase):
    def test_make_targets_exist(self):
        targets = make_targets()
        texts = [p.read_text(encoding="utf-8") for p in own_prose_files()] + [readme_agent_section()]
        named = set()
        for text in texts:
            for span in code_spans(text):
                named |= set(re.findall(r"\bmake\s+(?:-n\s+)?([a-z][a-z0-9-]+)", span))
        for path in skill_files({".sh"}):
            named |= set(re.findall(r"\bmake\s+(?:-n\s+)?([a-z][a-z0-9-]+)", path.read_text(encoding="utf-8")))
        self.assertGreater(len(named), 30)
        self.assertEqual(sorted(named - targets), [])

    def test_paths_exist(self):
        prefixes = r"(?:scripts|docs|gitops|terraform|charts|platform|services|libs|fixtures|references|tests|\.agents|\.claude|\.kiro|\.github)"
        missing = []
        for path in own_prose_files():
            text = path.read_text(encoding="utf-8")
            for span in code_spans(text):
                for ref in re.findall(rf"(?<![\w./-]){prefixes}/[\w./-]+", span):
                    ref = ref.rstrip(".,:;)")
                    if any(c in ref for c in "<>*") or ref.endswith("-") or ref.endswith(RUNTIME_ONLY):
                        continue
                    if not ((ROOT / ref).exists() or (SKILL / ref).exists()):
                        missing.append(f"{path.relative_to(ROOT)}: {ref}")
            for link in re.findall(r"\]\(([^)]+)\)", text):
                if link.startswith(("http://", "https://", "#")):
                    continue
                target = (path.parent / link.split("#")[0]).resolve()
                if not target.exists():
                    missing.append(f"{path.relative_to(ROOT)}: link {link}")
        self.assertEqual(missing, [])

    def test_scripts_the_skill_runs_exist_and_are_executable(self):
        for rel in ("scripts/ops-health.sh", "scripts/ops-health_test.sh", "tests/jsonpath_test.sh",
                    "fixtures/fire-drill/stub.sh", "fixtures/fire-drill/run.sh"):
            p = SKILL / rel
            self.assertTrue(p.is_file(), rel)
            self.assertTrue(p.stat().st_mode & 0o100, f"{rel} is not executable")
        self.assertTrue((ROOT / "scripts/check-agent-skills.sh").stat().st_mode & 0o100)

    def test_kinds_exist_in_the_manifests(self):
        pairs = repo_kinds()
        kinds = {k for _, k in pairs} | CORE_KINDS
        self.assertIn(("rds.services.k8s.aws", "DBInstance"), pairs)
        unknown = []
        for path in skill_files({".md", ".sh"}):
            text = path.read_text(encoding="utf-8")
            spans = code_spans(text) if path.suffix == ".md" else []
            for span in spans:
                word = span.strip()
                if re.fullmatch(r"[A-Z][a-z]+(?:[A-Z][a-zA-Z]*)+|[A-Z][a-z]+", word) and word not in NOT_KINDS and word not in kinds:
                    unknown.append(f"{path.name}: `{word}`")
            for plural, group in re.findall(
                        r"\b([a-z]+)\.((?:rds|elasticache|iam|ec2|eks)\.services\.k8s\.aws|kro\.run|argoproj\.io|karpenter\.sh|eks\.amazonaws\.com|keda\.sh)\b(?![/\w])", text):
                if plural in ("services", "eks"):
                    continue
                ok = any(g == group and plural in (k.lower() + "s", k.lower() + "es", k.lower()[:-1] + "ies")
                         for g, k in pairs)
                if not ok:
                    unknown.append(f"{path.name}: {plural}.{group}")
        self.assertEqual(sorted(set(unknown)), [])


class PublicSafe(unittest.TestCase):
    def files(self):
        out = [(p, p.read_text(encoding="utf-8")) for p in skill_files()]
        out += [(ROOT / n, (ROOT / n).read_text(encoding="utf-8")) for n in ("AGENTS.md", "CLAUDE.md", "scripts/check-agent-skills.sh")]
        out.append((ROOT / "README.md", readme_agent_section()))
        return out

    def test_no_account_ids(self):
        bad = [f"{p.name}: {m}" for p, t in self.files() for m in re.findall(r"(?<!\d)\d{12}(?!\d)", t) if m != "111122223333"]
        self.assertEqual(bad, [])

    def scanned(self):
        """Every file but this one, which has to spell out the patterns it looks for."""
        return [(p, t) for p, t in self.files() if p.resolve() != Path(__file__).resolve()]

    def test_no_secrets(self):
        patterns = [r"AKIA[0-9A-Z]{16}", r"ASIA[0-9A-Z]{16}", r"-----BEGIN [A-Z ]*PRIVATE KEY", r"ghp_[A-Za-z0-9]{20,}",
                    r"github_pat_[A-Za-z0-9_]{20,}", r"xox[abps]-[A-Za-z0-9-]{10,}", r"(?i)aws_secret_access_key\s*=",
                    r"(?i)password\s*[:=]\s*['\"]?[A-Za-z0-9!@#$%^&*]{8,}"]
        bad = [f"{p.name}: {pat}" for p, t in self.scanned() for pat in patterns if re.search(pat, t)]
        self.assertEqual(bad, [])

    def test_no_internal_hosts_or_local_paths(self):
        words = [r"amazon\.com", r"a2z\.com", r"amazon\.dev", r"aws\.dev", r"\bcorp\b", r"(?i)\binternal\.",
                 r"(?i)\bstrongest\b", r"(?i)/home/", r"(?i)/Users/"]
        bad = [f"{p.name}: {w}" for p, t in self.scanned() for w in words if re.search(w, t)]
        self.assertEqual(bad, [])

    def test_no_dashes_as_punctuation(self):
        bad = []
        for path in own_prose_files():
            text = path.read_text(encoding="utf-8")
            if re.search("[\u2013\u2014]", text):
                bad.append(f"{path.name}: an en or em dash")
            for line in prose_lines(text):
                body = re.sub(r"^\s*(?:[-*]|\d+\.)\s", "", line)
                if re.fullmatch(r"\s*\|?[\s|:-]*\|?\s*", body):
                    continue
                if re.search(r"\s--?\s", body):
                    bad.append(f"{path.name}: {line.strip()[:80]}")
        section = readme_agent_section()
        for line in prose_lines(section):
            if re.search(r"[\u2013\u2014]|\s--?\s", re.sub(r"^\s*[-*]\s", "", line)):
                bad.append(f"README.md: {line.strip()[:80]}")
        self.assertEqual(bad, [])

    def test_health_check_only_reads(self):
        text = (SKILL / "scripts/ops-health.sh").read_text(encoding="utf-8")
        code = "\n".join(line for line in text.splitlines() if not line.lstrip().startswith("#"))
        verbs = r"(apply|delete|patch|edit|annotate|label|scale|create|replace|rollout|cordon|drain|exec|port-forward|cp|run|set)"
        self.assertEqual(re.findall(rf"\bk(?:get)?\s+{verbs}\b", code), [])
        aws_calls = re.findall(r"\ba\s+(eks|ec2|iam|rds|elasticache|sts|s3|ecr)\s+([a-z-]+)", code)
        self.assertGreaterEqual(len(aws_calls), 3)
        self.assertEqual([c for c in aws_calls if not c[1].startswith(("describe-", "list-"))], [])
        self.assertNotRegex(code, r"(?i)\bkget\s+[\w.,]*secret")


class RepoFiles(unittest.TestCase):
    def test_claude_md_only_imports_agents_md(self):
        self.assertEqual((ROOT / "CLAUDE.md").read_text(encoding="utf-8"), "@AGENTS.md\n")

    def test_agents_md_points_at_the_skill(self):
        text = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
        for needle in ("encabulator-ops", "make ops-health", "make skills-check", "symbolic links", "git commit -s", "auto-delete=no"):
            self.assertIn(needle, text)

    def test_readme_has_the_agent_section(self):
        self.assertIn("make ops-health", readme_agent_section())

    def test_kiro_skills_are_tracked_not_ignored(self):
        r = subprocess.run(["git", "-C", str(ROOT), "check-ignore", "-q", ".kiro/skills"])
        self.assertEqual(r.returncode, 1, ".kiro/skills must not be gitignored")
        r = subprocess.run(["git", "-C", str(ROOT), "check-ignore", "-q", ".kiro/settings/mcp.json"])
        self.assertEqual(r.returncode, 0, "other .kiro files stay ignored")

    def test_every_drill_is_complete_and_tested(self):
        tests = (SKILL / "scripts/ops-health_test.sh").read_text(encoding="utf-8")
        drills = [d for d in (SKILL / "fixtures/fire-drill").iterdir() if d.is_dir()]
        self.assertGreaterEqual(len(drills), 8)
        for d in drills:
            with self.subTest(drill=d.name):
                world = (d / "world.txt").read_text(encoding="utf-8")
                self.assertTrue(d.name == "healthy" or "## base healthy" in world)
                drill = (d / "drill.md").read_text(encoding="utf-8")
                for section in ("## Presenter says", "## Expected diagnosis", "## Safe fix", "## Must not"):
                    self.assertIn(section, drill)
                self.assertRegex(tests, rf'run "[^"]+" {re.escape(d.name)}\b')


if __name__ == "__main__":
    unittest.main(verbosity=1)
