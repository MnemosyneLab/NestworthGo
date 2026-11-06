# Nestworth documentation

This tree describes the checked-out source. The [published releases](https://github.com/MnemosyneLab/NestworthGo/releases)
describe distributed artifacts and their acceptance limits; they may differ from
`main`. App metadata lives in `build/config.yml`; the independently versioned
end-user skill lives in [`skills/nestworth/VERSION`](../skills/nestworth/VERSION).

## Start here

| Need | Entry point |
| --- | --- |
| Product purpose and future direction | [Vision](product/product-vision.md), [roadmap](product/roadmap.md) |
| User workflows | [Reconciliation](user/account-reconciliation.md), [cash adjustments and Repeat](user/cash-adjustments-and-repeat.md), [wealth history](user/historical-overview.md), [upcoming actions](user/upcoming-actions.md) |
| Install and connect the optional skill | [Nestworth skill guide](user/nestworth-skill.md) |
| Runtime and financial boundaries | [System overview](architecture/system-overview.md), [domain model](architecture/domain-model.md), [data and IPC contracts](architecture/data-and-ipc-contracts.md) |
| Built-in assistant | [Architecture and authority](architecture/builtin-ai.md), [desktop read-only runtime](architecture/builtin-ai-runtime.md), [page guide](architecture/builtin-ai-page-guide.md), [voice input](architecture/ai-voice-input.md) |
| Screens and interaction contracts | [Design index](design/README.md) |
| Development, build and recovery procedures | [Development index](development/README.md) |
| Regression methods and acceptance limits | [Testing index](testing/README.md), [QA evidence](qa/README.md) |
| Published compatibility and release evidence | [Release index](releases/README.md), [changelog](../CHANGELOG.md) |

## Authority and maintenance

Current code, manifests, schema and tests define implemented behavior.
Architecture owns technical and financial contracts; design owns user-visible
flows. Roadmap proposals do not imply implementation. Release records and dated
QA reports establish evidence only for their named source and environment.
Passing Linux, browser or jsdom checks does not establish native macOS acceptance.

Keep one owning document per contract and link to it. Prefer durable decisions,
repeatable commands and known limitations over branch diaries, task assignments,
intermediate test counts or copied implementation plans. Before deleting an old
record, preserve unique compatibility, recovery, safety and acceptance evidence
in its owning document. Historical source remains available in Git history.
Do not turn an unverified old finding into either a current defect or a claimed fix.

[`AGENTS.md`](../AGENTS.md) and [`frontend/AGENTS.md`](../frontend/AGENTS.md)
are contributor instructions. `skills/nestworth` is a distributable end-user
ledger skill, not a repository development workflow. Asset licenses/notices and
machine-consumed contracts retain their own authority.

Maintained engineering contracts use English; existing proposal and test-plan
material may retain its established language. Mark future work as planned or
deferred and name unverified acceptance gates explicitly.
