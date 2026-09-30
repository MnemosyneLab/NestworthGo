#!/usr/bin/env python3
"""Check the portable skill's references, examples and current MCP tool names."""
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent.parent
SKILL = ROOT / "skills/nestworth"


def check():
    entry = (SKILL / "SKILL.md").read_text()
    if not entry.startswith("---\nname: nestworth\ndescription: ") or "\n---\n" not in entry[4:]:
        raise ValueError("Missing name/description frontmatter")
    if len(entry.split()) > 900:
        raise ValueError("Move conditional detail into references")
    yaml = (SKILL / "agents/openai.yaml").read_text()
    if "$nestworth" not in yaml or "allow_implicit_invocation: true" not in yaml:
        raise ValueError("Missing invocation metadata")
    sources = "\n".join(p.read_text() for p in (ROOT / "internal/mcpserver").glob("*.go") if not p.name.endswith("_test.go"))
    tools = set(re.findall(r'(?:readTool|writeTool|readAttributionTool)\([^\n]*?"([a-z_]+)"', sources))
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
            if re.match(r'(?:get|list|create|update|archive|set|search|preview|commit|scan|start|import)_', name) and name not in tools:
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
    check()
