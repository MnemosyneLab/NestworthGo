#!/usr/bin/env python3
"""Check the portable skill's references, examples and current MCP tool names."""
import argparse
import json
import os
import subprocess
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent.parent
SKILL = ROOT / "skills/nestworth"


# These are the source inputs embedded by package-nestworth-skill.py.
DISTRIBUTION_PATHS = ("skills/nestworth", "tools/install-nestworth-skill.sh",
                      "docs/user/nestworth-skill.md")


def version_tuple(value):
    if not re.fullmatch(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", value):
        raise ValueError(f"Expected a stable skill MAJOR.MINOR.PATCH version: {value!r}")
    return tuple(map(int, value.split(".")))


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def ci_base():
    # Compare pushes with their actual previous tip (also covers multi-commit
    # pushes). New branches/manual dispatch compare with their main merge base.
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    before = event.get("before", "")
    if os.environ.get("GITHUB_EVENT_NAME") == "push" and before and set(before) != {"0"}:
        return before
    return git("merge-base", "origin/main", "HEAD")


def check_version(base):
    # Resolve first: a missing/shallow baseline is an error, never a silent pass.
    base = git("rev-parse", "--verify", base + "^{commit}")
    changed = git("diff", "--name-only", base, "--", *DISTRIBUTION_PATHS)
    current = (SKILL / "VERSION").read_text().strip()
    current_tuple = version_tuple(current)
    if not changed:
        print("No distributable skill changes since " + base)
        return
    previous = git("show", base + ":skills/nestworth/VERSION")
    if current_tuple <= version_tuple(previous):
        raise ValueError(f"Distributable skill changed: bump skills/nestworth/VERSION above {previous} (got {current})")
    print(f"Distributable skill version: {previous} -> {current}")


def check():
    version = (SKILL / "VERSION").read_text().strip()
    version_tuple(version)
    guide = (ROOT / "docs/user/nestworth-skill.md").read_text()
    if f"### Current repository skill: {version}; published skill:" not in guide:
        raise ValueError("Installation guide must identify the current skill VERSION")
    entry = (SKILL / "SKILL.md").read_text()
    if not entry.startswith("---\nname: nestworth\ndescription: ") or "\n---\n" not in entry[4:]:
        raise ValueError("Missing name/description frontmatter")
    if len(entry.split()) > 900:
        raise ValueError("Move conditional detail into references")
    yaml = (SKILL / "agents/openai.yaml").read_text()
    if "$nestworth" not in yaml or "allow_implicit_invocation: true" not in yaml:
        raise ValueError("Missing invocation metadata")
    sources = "\n".join(p.read_text() for p in (ROOT / "internal/mcpserver").glob("*.go") if not p.name.endswith("_test.go"))
    tools = set(re.findall(r'(?:readTool|writeTool|readAttributionTool|analysisTool|ledgerWriteTool)\([^\n]*?"([a-z_]+)"', sources))
    tools.update(re.findall(r'Name:\s*"([a-z_]+)"', sources))
    examples, linked = set(), set()
    for path in [SKILL / "SKILL.md", *sorted((SKILL / "references").glob("*.md"))]:
        text = path.read_text()
        for target in re.findall(r'\[[^\]]*\]\(([^)]+)\)', text):
            resolved = (path.parent / target.split("#")[0]).resolve()
            if not resolved.is_relative_to(SKILL) or not resolved.is_file():
                raise ValueError(f"Broken/nonportable reference in {path.name}: {target}")
            linked.add(resolved)
        for name in re.findall(r'`([a-z_]+)`', text):
            if re.match(r'(?:get|list|create|update|archive|set|search|preview|commit|scan|start|import|compare)_', name) and name not in tools:
                raise ValueError(f"Unavailable tool {name} in {path.name}")
        for block in re.findall(r'```json\n(.*?)\n```', text, re.S):
            json.loads(block)
        for example in re.findall(r'<!-- example: ([a-z-]+) -->', text):
            if example in examples:
                raise ValueError(f"Duplicate example {example}")
            examples.add(example)
        if "/Users/" in text or "localhost:1234" in text:
            raise ValueError(f"Machine-specific content in {path.name}")
    unlinked = set((SKILL / "references").glob("*.md")) - linked
    if unlinked:
        raise ValueError(f"Unreachable references: {unlinked}")
    print(f"Skill references and {len(examples)} JSON examples match current MCP tool names.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    baseline = parser.add_mutually_exclusive_group()
    baseline.add_argument("--base", help="Git commit/ref to compare with the working tree")
    baseline.add_argument("--ci-base", action="store_true", help="Use push before SHA or main merge base")
    args = parser.parse_args()
    check()
    if args.base or args.ci_base:
        check_version(ci_base() if args.ci_base else args.base)
