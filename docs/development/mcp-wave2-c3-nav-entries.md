# MCP Wave 2 C3: account snapshot and account valuations

Conclusion: `get_account_snapshot` is the single-account projection of the same
current-valuation capture as `get_account_valuations`. Do not merge the tools.
Clarify the descriptions only. Endpoint movement and investment return stay
different questions, answered by other tools.

## What the code does

Both handlers call `Service.pagedAccountValuations`. That function reads
`application.Service.AccountValuations`, which calls
`ValuationService.ValueAccountCompositions`. The capture kind is
`account_valuations`. Compact rows, component, composition and missing-input
sections, and `get_read_item` section `accounts` are built in that one function.

`get_account_snapshot` parses one required `id` and calls that function with
`accountIds` set to that id and every account filter left at its zero value
(`includeArchived` false, no member, institution, group, type or ownership
scope, no explicit limit). If the accounts page total is 0, the handler
returns `not_found`. `TestAccountSnapshotIsSingleAccountValuationCapture`
compares that call with `get_account_valuations` given the same single id: the
`mcp-read/1` summary and the account row match.

`get_account_valuations` accepts optional `accountIds`, the account filter
fields, and `limit`. An id that parses but matches nothing is a successful
empty page, not `not_found`. The same test locks that difference.

An omitted limit on either tool is `readLimit(0)`, which is the fixed page size
25. This path does not use the overview auto-inline budget.

## What is not the same capture

`get_overview` calls `AccountValuations`'s neighbor `Overview`, which forces
`IncludeArchived` and uses `ValueAccounts` without the composition and display
projection. `get_financial_context` is the minimal-disclosure package. A2's
net-worth task called that tool, not these two. `get_historical_overview` is
endpoint movement. `analyze_period` is period return. Those boundaries stay.

`ValueAccountCompositions` is `ValueAccounts` plus composition and display
fields for the account-page read. Snapshot and valuations share that extra
projection. Overview does not.

## Why not merge

A merge would have to choose one input and one empty result. Today those
differ:

| | `get_account_snapshot` | `get_account_valuations` |
| --- | --- | --- |
| Input | required `id` | optional `accountIds`, account filters, `limit` |
| Empty match | `not_found` for an unknown or archived id | successful empty page |
| Caller | built-in account-page read passes one known id | filtered or multi-account rows |

The built-in assistant allowlist exposes `get_account_snapshot` and not
`get_account_valuations`. Its account page calls snapshot with the selected
account id. Deleting either name is a breaking tool change, not a description
edit.

A2 did not show a model choosing between the two. The measurement script
followed the skill and used snapshot while recording a buy and reconciling.
That is not evidence that one name is unused.

## What this pass changed

Tool descriptions and the skill's account and analysis references now state
the shared capture, the `not_found` versus empty-page difference, and that
neither result is a return or an endpoint move. No handler, schema, or
valuation formula changed. The discovery-contract hashes exclude description
prose. Measured `tools/list` growth is the prose itself: +325 bytes in every
permission mode, tool counts unchanged. See the Wave 2 measurement note in
[mcp.md](mcp.md#wave-2-account-entry-descriptions-and-toolgroups-removal).
