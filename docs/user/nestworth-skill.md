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
the GitHub release, not the skill's own `VERSION` (currently `1.6.0`). Each release
must include the two named assets. If the latest App release lacks them, use a
release that includes them or a local bundle; there is no fallback to unreleased
main-branch content. The published v0.3.7 release includes skill 1.6.0 and both
required skill assets.

### Current skill: 1.6.0

Skill 1.6.0 documents frozen one-state financial context and detail/item reads,
two-state comparisons and coherent comparison-to-attribution links, plus
managed product/liquidity reads and guarded lifecycle, terms and valuation
workflows. Its MCP examples are synthetic and executable by repository tests;
that does not prove external Codex discovery, client reasoning or live Mac
acceptance. The App and skill are separate components. An older App does not
gain tools by installing a newer skill. The published v0.3.7 release bundles
skill 1.6.0. Running the installer with `--version latest` or `--version
v0.3.7` selects that published bundle.

Existing clients do **not** update their installed skill automatically when the
App or repository changes. Rerun the trusted installer (or use the published
v0.3.7 bundle), check `--status` for `1.6.0`, and reopen the chat if discovery
is stale. A locally built archive is not a published release asset unless it
is uploaded as part of a release.

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
term deposits/locked products require the App for subscription, redemption,
reservation and maturity; the skill does not imitate their lifecycle with trades.
Setup, backups/restores, provider credentials and history resets also remain App
workflows. No brokerage orders are placed through the skill.

The App version, package content schema (`financial-context/1`) and item schema
(`financial-context-item/1`) are separate contracts. Installed clients require
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
