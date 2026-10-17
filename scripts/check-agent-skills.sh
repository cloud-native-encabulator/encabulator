#!/usr/bin/env bash
# The agent skills live once, in .agents/skills (Codex reads it there). Claude Code reads
# .claude/skills and Kiro reads .kiro/skills, so both are symbolic links to ../.agents/skills.
# This checks that each one is still that link and that every skill reads through it.
#
#   scripts/check-agent-skills.sh     exit 1, naming the problem and the fix (CI runs it)
#
# On Windows, Git checks a committed symlink out as a small text file unless core.symlinks is
# on; this check then fails with the fix, `git config core.symlinks true` and a fresh checkout.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
want="../.agents/skills"
bad=0 count=0

for link in .claude/skills .kiro/skills; do
  if [[ ! -L "$link" ]]; then
    echo "$link is not a symbolic link (on Windows: git config core.symlinks true, then check out again)"
    bad=1; continue
  fi
  target="$(readlink "$link")"
  if [[ "$target" != "$want" ]]; then
    echo "$link points at $target, not $want"
    bad=1
  fi
done

for skill in .agents/skills/*/; do
  name="$(basename "$skill")"
  [[ -f "$skill/SKILL.md" ]] || { echo "$skill has no SKILL.md"; bad=1; continue; }
  count=$((count + 1))
  for dir in .claude/skills .kiro/skills; do
    [[ -f "$dir/$name/SKILL.md" ]] || { echo "$dir/$name/SKILL.md does not resolve"; bad=1; }
  done
done

if [[ $bad -ne 0 ]]; then
  echo "Fix: rm -f .claude/skills .kiro/skills && ln -s $want .claude/skills && ln -s $want .kiro/skills"
  exit 1
fi
echo "agent skills: $count in .agents/skills, read through .claude/skills and .kiro/skills (links to $want)"
