# Nestworth skill for Codex

Owner: Nestworth maintainers. Scope: local App users and skill distribution.
Status: implemented; standalone installer tests and isolated MCP examples are
the validation surface. External Codex discovery and native App acceptance are
separate checks.

The `nestworth` skill teaches Codex how to operate a running Nestworth App
through its MCP: accounts, investments, completed transactions, cash/FX,
reconciliation, sourced quotes, data health and financial analysis. Installation
does not connect MCP, enable write permission, or change financial records.

## Install from a standalone bundle

Download **both** `nestworth-skill.tar.gz` and `nestworth-skill.tar.gz.sha256`
from the same Nestworth GitHub release into an empty working directory. The
bundle includes an installer, the skill, and this guide; no repository clone,
Go, Node, Python or Wails is required by the installer. macOS supplies Bash,
tar and shasum; release downloading also uses curl.

Verify the archive before extracting or executing its installer:

```bash
shasum -a 256 -c nestworth-skill.tar.gz.sha256
tar -xzf nestworth-skill.tar.gz
bash nestworth-skill/install.sh
```

The same command installs initially or updates. Default destination is
`${CODEX_HOME:-$HOME/.codex}/skills/nestworth`. The installer only touches this
skill and its own temporary/backup directories; it does not change Codex MCP
configuration, tokens, other skills, or Nestworth data. No skill is auto-installed
by App builds or by the maintainer check/package tasks.

To use another skills root (the script appends `/nestworth`):

```bash
bash nestworth-skill/install.sh --skills-dir /path/to/codex/skills
```

Once you have a trusted copy of the installer, a single command downloads and
installs/updates from the newest release containing these assets:

```bash
bash nestworth-skill/install.sh --version latest
```

Pin a published **App release tag** instead with `--version vX.Y.Z`. This selects
the GitHub release, not the skill's own `VERSION`. The latest published
[v0.3.10 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10)
contains skill 2.0.0; v0.3.9 contains 1.11.0, v0.3.8 contains 1.9.0 and
v0.3.7 contains 1.6.0. Each release must include the two named assets. If the
latest App release lacks them, use a release that includes them or a local
bundle; there is no fallback to unreleased main-branch content.

### Current repository skill: 2.2.0; published skill: 2.0.0

Repository skill 2.2.0 adds compatible guidance for inclusive batch-history
`fromDate`/`toDate` windows and opt-in cross-target `historyLayout=records` paging.
It preserves exact original FX direction and NAV/close basis, distinguishes
unread pages from missing data, and explains unavailable reverse-FX projections.
This minor bump identifies expanded query capabilities and contract guidance;
existing per-target history calls remain compatible. Import comparison is still
a design, not an available tool.

It also retains skill 2.1.0's post-v0.3.10 asset-change guidance:
`endpoint_bridge/2`, separately paged reconciliation details, paired trade
principal and valuation semantics, execution-price spreads and quantity
remeasurement. The earlier bounded read cutover required major version 2.0.0.
Skill 2.2.0 is not yet published; `--version latest` still selects App v0.3.10
and skill 2.0.0.

The App and skill are separate components: installing a newer skill does not
add tools to an older App. Actual discovery and returned schemas remain the
authority. Examples remain synthetic and do not establish external Codex
discovery, client reasoning or live Mac acceptance.

Existing clients do **not** update their installed skill automatically when the
App or repository changes. Update explicitly, check `--status` for the intended
skill version and reopen the chat if discovery is stale. A locally built archive
is not a published release asset unless uploaded as part of a release.

For Inspector CLI 2.10.0, the shipped connection reference includes a separate
configuration example with explicit `"type": "http"`; preserve the App's endpoint
and bearer header. The adaptation follows the Inspector's
[server configuration format](https://github.com/modelcontextprotocol/inspector/blob/main/docs/mcp-server-configuration.md)
and does not change the App's copied configuration for other clients. Nullable
array portability warnings alone do not establish a permissions/tool failure;
check discovery and actual argument validation.

## Preview, inspect and update

```bash
bash nestworth-skill/install.sh --dry-run
bash nestworth-skill/install.sh --status
bash nestworth-skill/install.sh --source /path/to/nestworth-skill.tar.gz
```

An archive source requires its adjacent `.sha256` file, or an explicit trusted
`--sha256` digest. A source can also be a skill directory or extracted bundle.
Dry run validates the source and reports the proposed target without changing
the installed skill. Status makes no network requests.

Identical content is a no-op. Replacement preserves the **entire previous
directory, including user edits**, under `nestworth-skill-backups` beside the
actual skills root (outside discovery). Updates do not merge edits into the new
official content. Review/copy desired edits after updating; keep custom material
in your own separate skill if it should survive every upgrade.

An existing unmanaged `nestworth` directory is left untouched unless you pass
`--replace-unmanaged`; that opt-in still preserves a backup. Symbolic-link
destinations are rejected rather than followed. To roll back, close affected
Codex chats, move the current `nestworth` directory aside, and move the desired
backup back into its place. Do not put multiple backup skills under the skills
root. The installer reports both installation and backup paths.

## Connect and use

1. Finish household setup. Start history in the App when recording ledger data
   or requesting historical/period analysis; a current financial context works
   without a history starting point.
2. Open **Settings → AI / MCP**, choose read-only, directory maintenance or
   ledger recording, and enable MCP. Copy its exact local connection
   configuration into Codex's MCP settings. Keep the App running.
3. Reopen the Codex chat if needed for skill/tool discovery. Ask, for example:
   “用 $nestworth 查看我的账户，并解释上个月的投资收益。”
4. For recording changes choose the necessary App permission. The skill checks
   context, identities and previews; it follows a clearly authorized request
   without requiring a repeated confirmation for every tool.

The App's loopback endpoint needs a client on the same machine. A remote/cloud
Codex runtime cannot access it automatically. A missing tool may reflect
permission mode or an older App; actual tool schemas and capabilities take
precedence over this skill's examples. Changing permissions or disabling and
reenabling MCP rotates credentials; update client configuration accordingly.

The skill covers ordinary stocks/ETFs, crypto, funds, NAV-priced wealth products,
bonds and gold/silver when their identity and units are explicit. App-managed
term deposits/locked products use dedicated guarded MCP lifecycle tools when
exposed by the connected App and permission mode; the skill does not imitate
their lifecycle with ordinary trades. Reservations remain an App workflow.
Upcoming actions is an App-only read interface, with no new MCP tool.
Setup, backups/restores, provider credentials and history resets also remain App
workflows. No brokerage orders are placed through the skill.

The App version, skill version and returned MCP schema identifiers (including
`mcp-read/1` and `historical-read/2`) are separate contracts. Installed clients require
an explicit skill update. A missing item tool means the connected App lacks this
capability; do not silently fall back to identity-bearing reads.

## Maintainer commands

From a checkout, an end user can install/update directly without packaging:

```bash
bash tools/install-nestworth-skill.sh
```

Maintainers use Python 3.9+ standard library for validation/tests/packaging:

```bash
python3 tools/check-nestworth-skill.py
python3 -m unittest discover -s tools/tests -v
python3 tools/package-nestworth-skill.py
```

Equivalent tasks are `wails3 task skill:check` and `wails3 task skill:package`.
Packaging writes `dist/skills/nestworth-skill.tar.gz` and its `.sha256`, with
reproducible contents. It does not install anything. The Mac release task also
runs this package step and verifies the adjacent checksum. Upload **both**
unchanged assets to the intended App release; verify skill version/content
when updating.

MCP argument examples are exercised by `go test ./internal/mcpserver -run Skill`
against temporary databases and loopback HTTP. These tests validate examples and
effects, not external Codex reasoning, discovery or a live provider. When MCP
contracts change, update the relevant reference/example and rerun these checks.
See [Local MCP](https://github.com/MnemosyneLab/NestworthGo/blob/main/docs/development/mcp.md)
for the implementation contract; the web copy follows the main branch.

Account reconciliation sessions, added in skill 1.10.0, are included in the
published v0.3.9 skill 1.11.0 bundle. Discover reconciliation_session_read
and reconciliation_session_write in the connected App before using it. It can
start/pause/resume sessions, read paged evidence, link existing direct corrections
or real missing transactions, explicitly check sourced evidence and finish fully
or partially. It must never invent a source, value, explanation or confirmation.
See [account reconciliation](account-reconciliation.md). Private receipts remain
separate from user-facing completed reconciliation evidence.

## Skill version policy

`skills/nestworth/VERSION` is the only skill version source. Packaging copies
that file verbatim; the installer reads it and records/reports it in installation
metadata and `--status`. Do not duplicate it in frontmatter or agent metadata.
The App's version/build and the installer's `--version` release selector are
independent. A new App release can ship an unchanged skill version only when
all distributable skill inputs are unchanged.

Bump in the same PR whenever any distributable input changes: every file under
`skills/nestworth/` (entry instructions, agent metadata and all references),
`tools/install-nestworth-skill.sh` and this guide (bundled as `INSTALL.md`).
Reference-only edits, examples, typo fixes, safety/authorization guidance,
financial meaning, paging, schema/tool arguments and compatibility guidance all
count. Changes to implementation contracts require reviewing the skill and
updating/bumping it when its guidance changes. Unrelated App code, development
docs and tests alone do not require a skill bump.

Use stable SemVer `MAJOR.MINOR.PATCH`: major for incompatible supported tool,
argument, workflow or interpretation changes; minor for compatible new
capabilities or expanded contract guidance; patch for compatible corrections,
clarifications, metadata or installer fixes. Classify by effect on consumers,
not file type. Never reuse or decrease a version. Before choosing it, fetch main
and tags, inspect VERSION history and published release assets (including draft
releases), then choose above both main and any already allocated version. Update
this guide's current repository section in the same PR; preserve historical
published versions. Recheck after rebasing or concurrent releases. Never replace
published assets with different content under the same skill version.

Stage added payload files so Git includes them in the local comparison.
Before committing, run:

```bash
git fetch origin main --tags
python3 tools/check-nestworth-skill.py --base origin/main
python3 -m unittest discover -s tools/tests -v
python3 tools/package-nestworth-skill.py
(cd dist/skills && shasum -a 256 -c nestworth-skill.tar.gz.sha256)
go test ./internal/mcpserver -run Skill
```

Inspect the archive's VERSION and compare all payload files with their sources.
Installer tests use temporary destinations exclusively; do not update a real
installed skill or ledger to validate a release.

The native CI tooling lane checks the complete branch change against its unique
merge base with `origin/main` on every origin branch push/manual run. The bump
must also exceed current main's skill version when the branch changes bundled
inputs, so a concurrent main release cannot silently allocate the same version.
A follow-up unrelated push cannot hide a missing bump; further edits in the same
unmerged PR can retain its already-increased version. Application-only branches
need no bump. Rebase/review versions when main advances.

On main pushes, the baseline is the actual previous main tip, which must exist
and be an ancestor of the exact event head. Manual main runs check the latest
integration against its first parent. Both also compare against main's latest
first-parent VERSION-change commit: that boundary must itself be a valid numeric
increase over its immediate first parent's VERSION. Whitespace edits and
version decreases cannot reset the drift check. Search is bounded to 256
first-parent commits (independent of the number of VERSION edits); absent,
invalid, parentless or older boundaries fail and require reviewed history or
bootstrap handling. No search skips invalid boundaries looking for a pass.
Content drift after an integrated version must not be hidden by a later
unrelated main push. Full checkout history is required; shallow
repositories, missing refs/commits, ambiguous/no merge bases, initial main commits
with no previous tip, unrelated main force-pushes and mismatched event heads fail
explicitly. No tag or release-history fallback silently passes the guard.
Initial adoption without historical VERSION requires a reviewed bootstrap;
`--base` supports explicit local historical review, not a CI bypass.

The native workflow continues to accept only origin branch pushes/manual runs;
PR and tag events are rejected by the automatic guard. PR heads receive checks
through their origin branch push, without enabling execution of fork PR code on
the personal runner. Tag publication does not replace branch validation or
release asset/version allocation review.

The content checker also verifies the guide's current repository version.
The guard compares Git changes for all bundled inputs and requires a strictly
higher stable version, including for version-only edits. No maintained hashes
are needed. Without `--base`/`--ci-base`, the checker validates current content
only (for source bundles/checkouts without history). The guard does not decide
SemVer impact, detect stale guidance after implementation-only changes, or query
release registries for version reuse: maintainers still review those points.
