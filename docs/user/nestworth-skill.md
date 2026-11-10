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

### Current repository skill: 3.0.15; published skill: 2.0.0

Repository skill 3.0.15 shares compact business recipes with built-in tool descriptions, documents the complete daily review rule format, and distinguishes direct mutations from preview/commit workflows. It does not load installed skills in the App or change permissions/recovery.

Repository skill 3.0.14 documents the two MCP modes, `read_only` and
`read_write`. The read-write mode includes directory, ledger and
investment-review management operations. Unsupported saved modes fall back to
read only.

Repository skill 3.0.12 uses a compact overview with opt-in detail, one
managed-product read, one ledger preview for 1–100 commands, and a reconciliation
header with the first item page. Financial context and comparison calls select
initial sections and continue with capture identities and opaque cursors. The
MCP implementation has its own version.

Repository skill 3.0.11 documents the shared mcp-read/2 protocol, unified
account valuations, direct holdings selection and focused evidence reads.

Repository skill 3.0.8 removes `get_contribution_item`, `get_return_day` and
`get_asset_driver_detail`. Open those details with `get_read_item` on the
parent capture: contribution rows use section=`rows`; an `analyze_period`
return day uses section=`returnDays` and id=YYYY-MM-DD; an asset-change
bucket uses section=`assetDrivers` and the bucket key.

Repository skill 3.0.7 tells agents that investment-review management tools are
hidden unless Settings enables that switch on a `ledger_write` connection.
Review reads stay available. No tool was added or removed from the App; default
MCP discovery omits the seven management tools until the switch is on.

Repository skill 3.0.6 splits the long product, financial-context and paging
references into one file per task. Each file keeps that task's steps, failure
recovery and the same schema-checked examples. No tool was added or removed.
`commit_plan` may still omit `operationId`. The server derives one from `planId`.

Repository skill 3.0.5 is the coordinator PATCH for the Wave 1 tool-surface
changes already in this tree and for this follow-on. Wave 1 removed
`archive_member`, `archive_institution`, `archive_group`,
`get_historical_overview_series` and `get_market_data_batch`. Member,
institution and group archive is `archived` on the matching `update_*` tool.
Historical overview and market data each keep one tool with two input shapes.
This pass removes the optional `tools/list` `_meta.nestworth.toolGroups`
filter. Clients do not send it, and a missing group never changed `tools/call`
permission. Default discovery stays the full list for the permission mode.
`commit_plan` may omit `operationId`; the server derives one from `planId`.
Other writes still require `operationId`, and an explicit id remains valid.
Success results still carry both `structuredContent` and JSON text.
The current protocol unifies current-account reads as `get_account_valuations`,
with one or many IDs, summary/holdings detail and empty unmatched results. It is
not a return or an endpoint movement. The version
stays a PATCH step because the policy below does not raise MAJOR or MINOR for
tool changes alone, and actual discovery is the authority when connecting an
older App to this skill.

Repository skill 3.0.4 clarifies partial institution updates and adds a
schema-checked icon-only example. It includes the unified business continuation entry
`get_read_page({cursor, limit?})`, default 25/max 100, including market-import
comparison guidance. Live and version-bound reads return items/page; review
inputs use optional limit. Descriptors retain live/version_bound/frozen semantics,
fixed capture/hash/TTL, disclosure, stale/restart rules and the complete 64 KiB
JSON-RPC budget. Initial financial content and focused-item descriptors keep
their immutable shape without duplicate rows.

The same skill includes compact identity lists and same-capture details with
metadataVersion guards; frozen contribution rows/drill-down; progress-only repair
polling with separate frozen evidence; task-directed discovery and reads; inclusive
batch-history windows; read-only import comparison; and asset-change guidance.
Complete-list/absence/audit claims still require traversing all relevant pages.
Embedded and standalone consumers use the same source and schema-checked examples.
The current MCP workflows include breaking changes from the published skill;
use actual discovery and schemas when connecting to an older App.

Skill 3.0.3 adds update-convention guidance (how `update_account` clears fields and
which product-terms field is optional) and follows two tool consolidations. First,
one `commit_plan` replaces the ten per-kind commit tools: it takes the same
`{operationId, input:{planId}}`, reads the plan kind from the stored plan, and keeps
each kind's receipt, replay and unknown-outcome rules. Second, `update_member`,
`update_institution` and `update_group` take `name`, `iconKey` or both, and the
three `set_*_icon` tools are gone. A 3.0.2 or older skill names tools this App no
longer registers; the version stays a PATCH step because the policy below does not
raise MAJOR or MINOR for tool changes alone, and actual discovery is the authority
when connecting an older App to this skill. Skill 3.0.2 was a slimming change with no new MCP contract: the entry file no
longer asks the agent to inspect every tool before a task, connection guidance
drops client-specific configuration (moved to the developer MCP guide), and the
financial-context, two-state comparison and attribution workflows moved from
`analysis.md` to `financial-context.md` so period-return questions load less
text. The two explicit-scope financial page aliases were removed from the App;
`get_read_page({cursor, limit?})` is the only continuation entry.

This is an explicitly requested version normalization from the unreleased 6.0.1
tree at `94302de02e6b3894fe3ba5f0d7931210c396ef5a`, not a content rollback or
publication. Earlier repository labels 3.0.1, 4.0.0, 5.0.0, 6.0.0 and 6.0.1
in Git history and dated QA records identify those original tested trees only.
They are not aliases for this tree and were not published release assets.
Repository skill 3.0.15 is not published: `--version latest` still selects App v0.3.10
and its skill 2.0.0. No published assets or installed user skills are replaced.


The App and skill are separate components: installing a newer skill does not
add tools to an older App. Actual discovery and returned schemas remain the
authority. Examples remain synthetic and do not establish external Codex
discovery, client reasoning or live Mac acceptance.

Existing clients do **not** update their installed skill automatically when the
App or repository changes. Update explicitly, check `--status` for the intended
skill version and reopen the chat if discovery is stale. A locally built archive
is not a published release asset unless uploaded as part of a release.

For Inspector CLI 2.10.0, add `"type": "http"` to the App-copied entry and
preserve the endpoint and bearer header; see the
[Inspector configuration notes](../development/mcp.md#inspector-cli-configuration).
Other clients keep the App's copied configuration unchanged.

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
`mcp-read/2` and `historical-read/2`) are separate contracts. Installed clients require
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
`tools/install-nestworth-skill.sh`, this guide (bundled as `INSTALL.md`), and
`internal/agentguidance/recipes.json`, the source of generated business blocks.
Reference-only edits, examples, typo fixes, safety/authorization guidance,
financial meaning, paging, schema/tool arguments and compatibility guidance all
count. Changes to implementation contracts require reviewing the skill and
updating/bumping it when its guidance changes. Unrelated App code, development
docs and tests alone do not require a skill bump.

Use stable `MAJOR.MINOR.PATCH`. Whenever distributable skill content changes,
**default to increasing PATCH (the third component) by exactly one**, for example
`3.0.1` → `3.0.2`. This includes ordinary additions, reference/example edits,
corrections, clarifications, metadata and installer changes. Do not automatically
increase MAJOR or MINOR because a tool/schema changed or guidance grew.
Adjust MAJOR or MINOR only for a substantial change or an explicit user request;
document the scope and reason in the PR. For a substantial change, use MAJOR for
incompatible supported workflows/interpretation and MINOR for significant
compatible capabilities. Review consumer compatibility independently of the number.

Before choosing a version, fetch main and tags and inspect VERSION history and
published/draft release assets. Normally choose above main and every allocated
version; never replace published assets with different content under the same
version. Update this guide's current repository section and recheck after rebasing
or concurrent releases. Preserve historical published versions and dated evidence.

The sole approved historical exception is the explicitly requested normalization
of unreleased 6.0.1 at `94302de02e6b3894fe3ba5f0d7931210c396ef5a` to 3.0.1.
The guard accepts only that exact source tip/version and target version, including
its main integration boundary; there is no flag or general downgrade exemption.
After integration the normal sequence resumes at 3.0.2. Unchanged/whitespace-only
versions, other decreases and later content drift still fail. Historical
unreleased 4.x–6.x labels do not set the future allocation floor after this
normalization; published versions remain immutable.

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
inputs (except the exact normalization above), so a concurrent main release cannot silently allocate the same version.
A follow-up unrelated push cannot hide a missing bump; further edits in the same
unmerged PR can retain its already-increased version. Application-only branches
need no bump. Rebase/review versions when main advances.

On main pushes, the baseline is the actual previous main tip, which must exist
and be an ancestor of the exact event head. Manual main runs check the latest
integration against its first parent. Both also compare against main's latest
first-parent VERSION-change commit: that boundary must itself be a valid numeric
increase over its immediate first parent's VERSION, or the exact approved
normalization above. Whitespace edits and other
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
higher stable version, including for version-only edits, apart from that one
normalization. No maintained payload hashes are needed. Without `--base`/`--ci-base`, the checker validates current content
only (for source bundles/checkouts without history). The guard does not decide
SemVer impact, detect stale guidance after implementation-only changes, or query
release registries for version reuse: maintainers still review those points.

## Shared business recipe maintenance

Compact business interpretation and workflow recipes have one canonical source:
`internal/agentguidance/recipes.json`. Change that source, then run
`python3 tools/check-nestworth-skill.py --sync-recipes` to refresh marked blocks in
the relevant references. Ordinary validation rejects drift. Keep per-client
connection, permission and receipt-recovery rules outside those blocks. The App
embeds the neutral source into optional built-in tool descriptions; it does not
read this installed skill. Update VERSION in the same PR; this remains an
independent portable skill distribution, not an App activation mechanism.
