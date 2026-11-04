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
    # Only origin branch pushes/manual runs execute on the native runner. Never
    # turn PR payloads or tags into trusted executable events or fallback bases.
    event_name = os.environ.get("GITHUB_EVENT_NAME")
    ref = os.environ.get("GITHUB_REF", "")
    if event_name not in ("push", "workflow_dispatch") or not ref.startswith("refs/heads/"):
        raise ValueError("Version CI requires an origin branch push/manual event")
    if git("rev-parse", "--is-shallow-repository") != "false":
        raise ValueError("Version CI requires full history; shallow checkout is unsafe")
    head = git("rev-parse", "HEAD")
    if os.environ.get("GITHUB_SHA") != head:
        raise ValueError("Version CI must check the exact event head")
    if ref != "refs/heads/main":
        # Cover the entire proposed change on EVERY push, not just the latest
        # commit. A prior failed push must not be laundered by an unrelated one.
        bases = git("merge-base", "--all", "refs/remotes/origin/main", head).splitlines()
        if len(bases) != 1:
            raise ValueError("Version CI requires one unambiguous main merge base")
        return bases[0]
    if event_name == "workflow_dispatch":
        # On main, compare the most recent integration with its first parent.
        return git("rev-parse", "--verify", "HEAD^1^{commit}")
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    before = event.get("before", "")
    if not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", before) or set(before) == {"0"}:
        raise ValueError("Main push has no prior tip; a reviewed historical baseline is required")
    before = git("rev-parse", "--verify", before + "^{commit}")
    if before == head:
        raise ValueError("Main push baseline must precede the event head")
    # Force-pushed unrelated history cannot establish a safe baseline.
    git("merge-base", "--is-ancestor", before, head)
    return before


def check_ci_version():
    base = ci_base()
    floor = "refs/remotes/origin/main" if os.environ["GITHUB_REF"] != "refs/heads/main" else None
    check_version(base, minimum=floor)
    if floor is None:
        # main is already integrated: do not let an unrelated later push hide
        # content drift since its last first-parent version change either.
        boundary = git("log", "--first-parent", "-1", "--format=%H", "HEAD", "--",
                       "skills/nestworth/VERSION")
        if not boundary:
            raise ValueError("Main has no historical skill version boundary")
        check_version(boundary)


def check_version(base, minimum=None):
    # Resolve first: a missing/shallow baseline is an error, never a silent pass.
    base = git("rev-parse", "--verify", base + "^{commit}")
    changed = git("diff", "--name-only", base, "--", *DISTRIBUTION_PATHS)
    current = (SKILL / "VERSION").read_text().strip()
    current_tuple = version_tuple(current)
    previous = git("show", base + ":skills/nestworth/VERSION")
    version_tuple(previous)
    if not changed:
        print("No distributable skill changes since " + base)
        return
    if minimum:
        # Concurrent main releases can allocate a version after this branch's
        # merge base. Require a version above that tip when this branch changes
        # the distribution, without penalizing unrelated application-only PRs.
        main_version = git("show", minimum + ":skills/nestworth/VERSION")
        if version_tuple(main_version) > version_tuple(previous):
            previous = main_version
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
    baseline.add_argument("--ci-base", action="store_true", help="Check the complete branch change or main integration")
    args = parser.parse_args()
    check()
    if args.ci_base:
        check_ci_version()
    elif args.base:
        check_version(args.base)
