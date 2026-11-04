# MCP audit follow-ups

## Published baseline and scope

Baseline: main `a46d38bbbf84f9908506463dae1e27d28f62b163`.

- #212: bounded read-only import comparison, compact classification and same-capture detail.
- #215: frozen contribution list and drilldown, signed continuation and revocation.
- #216: reconciliation pages fit the complete 65,536-byte JSON-RPC response, including both SDK representations; write/recovery receipts bypass read interception.
- #217: compact identity directories and lean frozen financial continuation pages; skill 4.0.0.
- At inventory time #220 was separate: account/Agent SQL target/date pushdown and scoped counts. It is now integrated as main 82dd932; its filtering implementations remain unchanged.

This single draft PR collects remaining read wire/paging, discovery/schema,
repair polling, recovery guidance and documentation fixes in thematic commits.
It does not authorize merging or deployment. Existing financial calculations,
permissions, CAS, capture/revocation and unknown-write recovery remain mandatory.

## Initial findings from published source

- Review and activity reads are outside the final HTTP response interception.
  Their row limits alone cannot bound escaped or nested evidence bytes.
- Every repair-job poll performs a fresh health scan, then captures full job and
  health evidence even when only progress is needed.
- Server/skill entry guidance requires context and catalog for ordinary reads,
  despite the individual tools being usable without them.
- The reconciliation skill reference still advertises 96 KiB; #216 already
  changed the complete response limit to 64 KiB.

Validation results and remaining limits will be updated with implementation.

## Read wire and safe live pagination

Production response interception now derives from each registered capability's
read/analysis/preview effect; writes and get_operation recovery bypass it. This
also fixes the previous product-commit and metadata-batch overlap. The direct
shared read adapter checks successful payloads too. SDK validation errors and
JSON-RPC overhead still receive final HTTP enforcement.

Activity, review and product-operation pages retry a smaller authoritative page
under one application operation fence. They never cut rows off an already-made
cursor. Review publication cursors retain their original revision/filter guards;
activities and product history remain documented live reads. A single oversized
item or required header fails explicitly, with no dropped evidence.

Linux/amd64, Go 1.27.3 / SDK 1.7.0, synthetic temporary SQLite: the focused read
boundary, all-registered-effect receipt, single-item and complete activity union
regressions passed. An 18-row escaped/Unicode activity journey used 3 actual SDK
HTTP calls / 113,120 total response bytes, maximum 44,012 bytes. These are wire
bytes, not tokens or latency evidence. Every write effect's middleware path was
checked with a synthetic oversized body; that sweep does not execute business
writes. Existing persisted unknown-outcome tests remain a separate gate.

## Final implementation

The follow-up integrates main `82dd9321b8080523c3bae4640f913708292b1966`
(#220). Its account/Agent filtering, count scopes and repository implementations
are unchanged by this PR. The only application addition is a coherent read gate:
pure page-size retries preserve preview tokens; nested writes still invalidate
normally. Existing product/ledger-preview coordination regressions caught and
verified this distinction.

- Production response policy comes from registered capability effects, covering
  reads, analysis and previews, including SDK validation. Actual write and
  get_operation receipt bodies always bypass read interception. SDK write input
  errors are reduced before business dispatch; structured business failures and
  unknown outcomes retain their original codes. A synthetic executed oversized
  receipt and an unknown outcome are independently checked through the SDK.
- Activity, review and product-operation live pages resize through their owning
  readers under one read gate. No returned cursor is retained after cutting off
  rows. Complete groups/items are never split or skipped. Rules/suppressions
  continue every nonempty identity page until empty, including short pages.
- Discovery pages contain at most 16 tools, retain every input/output schema,
  and fit the complete 64 KiB HTTP limit. Repository SDK consumers now follow
  nextCursor. Common retry text and high-frequency descriptions are concise;
  structural schemas express activity cursor triplets, market target alternatives,
  paired dates/range exclusion, records/history dependence and ledger-kind inputs.
  Business decimal/date/identity/permission/CAS checks remain authoritative.
- Repair polling defaults to job sequence/phase/outcome/counters only. It does
  not scan health or allocate/evict captures. view=evidence freezes job details;
  neither view includes health. After completion, scan current health separately.
  Missing process-local jobs still leave completion unknown. Submission receipts
  retain their original durable recovery behavior.
- Rule previews preserve the original full policy, source token, window and
  exact daily results. The potentially oversized 365-day collection now uses a
  frozen days section; GUI parity reconstructs its complete union. There is no
  requirement to shorten a policy to make a read fit.
- Skill 5.0.0 teaches task-directed context/catalog reads, deferred detail and
  complete relevant evidence for list/absence/audit claims. It corrects the
  reconciliation reference from the obsolete 96 KiB inner-DTO limit to complete
  64 KiB responses. The guide and archive share the same version. Main is 4.0.0;
  the latest published v0.3.10 skill archive was inspected and contains 2.0.0.
  No published asset or installed skill was changed.

## Validation and interpretation

All local data is synthetic and temporary; loopback HTTP uses the production
MCP SDK 1.7.0 / JSON-RPC path with both text and structuredContent. Environment:
Linux/amd64, Go 1.27.3, server tag. Wire figures include actual response envelopes,
not model tokens. Opaque IDs/timestamps can change sizes slightly between runs.

Final-source focused measurements at `0033eb62af9be6cec317db772319514bddcd5aba`:

| Journey | Actual tool calls | Final response bytes | Maximum response |
| --- | ---: | ---: | ---: |
| 18 activities with Unicode/escaped notes, complete unique union | 3 | 113,120 | 44,012 |
| Known-account snapshot without context/catalog prerequisites | 1 | 2,960 | 2,960 |
| Three unchanged terminal repair progress polls | 3 | 3,066 | 1,022 |
| Complete 104-tool discovery after concise common retry guidance | 7 | 149,766 | 29,850 |
| Full 365-day rule preview, initial limit 1 then byte-aware evidence pages | 5 | 209,199 | 58,562 |

Discovery pagination adds requests; it bounds individual responses rather than
promising a proportional reduction in total schema bytes. Reuse schemas within
the connection. The known-account example demonstrates an executable one-call
route, not model obedience or a universal reduction for ambiguous-ID workflows.
The polling sample is a retained synthetic terminal job; it proves no capture
churn and compact repeated delivery, not provider performance or repair success.

An initial whole MCP run exposed unintended preview invalidation, missing common
safety/routing phrases, lower-permission SDK diagnostic handling and a prior test
assumption that write-schema errors had to pass through read response buffering.
The implementation fixes preserve preview safety, permission availability,
original unknown outcomes and all existing assertions; diagnostics now shrink
before execution without intercepting execution receipts. A subsequent complete
MCP run passed in 197.985s at source `5779cb0` before the final rule-preview change.
Final-source results follow below.

The full 136-test Python tooling attempt had one acquisition-fixture timeout
failure and one cancellation-fixture timeout error while Go compilation/checks
ran concurrently. No tooling code or thresholds were changed. The two matching
acquisition tests passed in an isolated 35.986s run; the cancellation test passed
in 0.286s. This is passing isolated rerun evidence, not a single green whole-suite
attempt. The other 134 tests passed on the original attempt.

Native desktop build/bindings/GUI and macOS packaging/signing/notarization are
unverified: this host lacks gtk4, webkitgtk-6.0 and libsoup-3.0 pkg-config entries.
No live providers, real household/CSV data, external assistant/model behavior,
model-token use, dense large rule sets, multi-year full-household analysis,
latency percentiles, allocation/RSS or concurrent restore stress are claimed.
The page fitter may requery a large live page up to eight candidate sizes; it
bounds delivery without claiming reduced database work. Oversized single
items/groups, required headers or captures retain explicit failures.


Final-code focused checks: all four persisted SQLite recovery tests passed
(8.936s): missing plan/receipt recovery, old unknown intent after a new plan
revision, product restart/unknown/restore, and in-process namespace restart with
unknown directory writes. This is separate from synthetic middleware-body tests.
The 365-day preview and existing funded-trade/no-change GUI parity tests passed;
three permission modes retain the immutable discovery baseline with explicit
reviewed metadata projections. MCP/application vet and targeted application gate/
preview/exclusive tests passed (application 2.138s). Skill checks validate 75
executable examples and 4.0.0 -> 5.0.0; checksum and 16 archive/source byte
comparisons passed. Gofmt and whitespace checks are clean.


Final implementation `0033eb62af9be6cec317db772319514bddcd5aba`:

- Complete MCP package passed with `-count=1` in **167.317s**, after the final
  preview paging change. All code and skill payloads in the later evidence/docs
  commit are identical to this tested implementation.
- Ordinary 20-account/8-instrument and perf-tagged 200-account/25-instrument
  payload tiers passed together in **19.227s**, each exercising 60 distinct tools.
  Actual business responses: 119 calls / 1,555,349 bytes / maximum 64,165 bytes at
  small scale; 270 calls / 8,752,739 bytes / maximum **64,632 bytes** at large scale.
  These aggregate all representative probes and complete journeys, not one user
  task. Both tiers independently traversed all seven discovery pages (149,766
  aggregate bytes). Larger repair-job polling is covered by its dedicated test,
  not claimed as part of the 200-account tier.
- Final focused measurements repeated the table above, including **0 new
  captures** for three repair polls, exact 365-day result union, explicit revoked
  preview cursor failure, complete unique activity union and one-call targeted
  account read. Permission/CAS/financial-quality tests remain in the full suite.
- Final MCP vet, perf selector validation, 75-example skill guard, packaging,
  checksum and all **16** archive/source byte comparisons passed. The maintained
  session architecture and testing documents now also state the complete 64 KiB
  budget; historical evidence describing the old 96 KiB implementation is labeled
  as historical rather than silently rewritten.

CI monitoring is separate from these local results. No CI completion, native UI
acceptance, merge or deployment is inferred from a branch push.

## Independent-review correction: preview kind discovery

Independent review identified two GUI-only DTO variants accidentally advertised
by the new preview_change schema: position_adjustment and value_update. They
are now removed from its conditional branches. The MCP handler's existing
allowlist is unchanged; target quantity/value adjustments still require their
dedicated reconciliation workflow. No permission or business capability expands.

A regression independently enumerates the ten supported MCP kinds, validates
each advertised required-field shape against both the schema and boundary
handler, and verifies that both layers reject fully populated unsupported
variants. The immutable discovery test projects only the reviewed schema delta.

## Independent-review follow-up: progress getter allocation

Review also identified that compact wire polling still called GetSyncJob, whose
Clone copied every retained item, blocker and prerequisite before the MCP
projection discarded them. Default progress now calls GetSyncJobProgress: one
lookup under the existing syncMu, scalar fields and len counts, plus a detached
NextEligibilityAt timestamp when present. No evidence arrays, Current pointer,
internal scope or job identity metadata are copied into this projection. Exact
job selection, nil/missing not_found behavior, sequence/outcome/counters and
wire shape are unchanged. view=evidence and GUI full snapshots still call the
original defensive Clone; their complete evidence remains available.

Application regression: an older requested job remains distinct from a newer
current job; 4,096 item / 2,000 blocker / 1,000 prerequisite counts match retained
state; every progress field matches the expected snapshot; returned timestamps
cannot mutate job state; full evidence snapshots remain independently mutable.
MCP progress/repair-retry/three-permission-mode health regressions passed in
2.593s. Three unchanged polls remain 3,066 actual response bytes with zero new
captures. The schema/handler/discovery regression group passed in 3.351s.

Opt-in getter-only microbenchmark: Linux/amd64, Go 1.27.3, 100 iterations per
case, retained synthetic jobs, non-null NextEligibilityAt. Each array has the
listed row count. No provider, SQLite, lock contention or MCP serialization is
included; no latency or percentile claim is made.

| Getter | Rows in each of three arrays | Allocated bytes/op | Allocations/op |
| --- | ---: | ---: | ---: |
| Full snapshot Clone | 1 | 264 | 4 |
| Progress projection | 1 | 24 | 1 |
| Full snapshot Clone | 4,096 | 983,074 | 4 |
| Progress projection | 4,096 | 24 | 1 |

The projection's work does not grow with retained evidence. The remaining one
allocation detaches the optional timestamp. This is a getter measurement, not
the total MCP response allocation; full evidence reads/GUI listeners retain their
existing O(n) snapshot copying. Focused application getter tests, MCP/application
vet, retained perf selector and the 75-example skill guard passed. This follow-up
uses focused regressions; it does not claim another full-package/race/stress run
or native macOS acceptance at the new head.

## Skill versus MCP contract audit

Reviewed every shipped reference against the current registered schemas, owning
handlers and response builders: connection/discovery/permission; account and
instrument identity; cash/debt/trade/batch writes; product lifecycle/terms/value/
policy; reconciliation; historical/current/period/financial reads; market import/
comparison/audit; repair; and recovery. Fixed contradictions and clarified scope:

- Repair defaults to progress, not mcp-read/1; evidence alone creates a capture.
- Initial captures contain summary/sections; get_read_page returns only the
  requested section/items/page with capture identity/hash/expiry.
- The obsolete “no import-comparison tool” sentence is removed.
- Read/preview wire budgets explicitly exclude executed/recovery receipts.
- Renewal without reservation release is supported by lifecycle preview/commit;
  reservation-affecting renewal remains an App workflow.
- Frozen preview days retain their TTL through source changes; live publication
  cursors bind revision/filter. Cache revocation, expiry, lost cache and auth
  failures are distinguished rather than promising one error for every trigger.
- Product detail/revision paths and instrument query versus filter.query are
  explicit. Comparison summaries fetch only needed evidence; complete-list,
  absence and audit claims still require complete relevant cursors.
- Connection references name all 104 registered tools. A per-family defaults,
  continuation, cache/wire budget and error table is included.

The audit also exposed a code/schema inconsistency: the Agent reader already
accepted closed observation filtering and returned closed facts, but its schema
enum excluded closed. Discovery now admits that existing read-only filter. The
real SDK regression uses the shipped closed-only filter example, excludes an
ordinary price record, then verifies withdrawal remains inactive. No filtering
pushdown, permission or financial selection authority changed.

Validation distinguishes three different layers:

1. Python reference/name/JSON guard: **76 parseable JSON examples**, not an
   execution claim. Its output now states this limitation explicitly.
2. New schema routing guard: **75 tool-argument examples** validate against the
   actual registered input schemas; **1 Inspector client configuration** is
   classified separately. Unmapped/new/missing examples fail. All 104 registered
   tools must appear by exact name in shipped references. This does not prove
   fixture identities, cursor authenticity, permission or business-state validity.
3. Actual SDK calls: the Skill workflow suites plus product lifecycle/terms/
   valuation/policy, reconciliation correction routes, repair progress/evidence
   and closed-day audit regressions passed in **10.936s**. They exercise the
   shipped arguments with real synthetic fixture IDs/plans/cursors and production
   typed handlers. Not every static example is claimed as a complete real-world
   journey. Shared-page defaults/lean shape and 16-tool discovery pages are also
   checked through the actual SDK; retained-data budgets are checked separately
   against the guide.

The 39 skill/installer/version Python tests passed in **19.447s**. Version 5.0.0
remains above main 4.0.0 within this same unmerged PR; no published asset or
installed skill was replaced. Packaging/checksum/source comparisons remain
required after the final reference changes. Real external assistant behavior,
model tokens and native macOS acceptance remain unverified.

## Remaining contract/discovery/recovery work

The follow-up implements the remaining safe client-boundary work rather than
claiming one consistency model for different authorities:

- Frozen captures and financial packages keep their fixed identity/hash/TTL and
  same-capture pages. Published review/reconciliation cursors retain revision
  fences; activity/product/rule/suppression identity pages remain live. Their
  differences are now explicit in the Skill matrix, not hidden as “all frozen”.
- **34** shared read/financial header/progress tools now publish stable success
  output envelopes validated by the registry and SDK. Nested summary/row DTOs
  retain exact existing fields; this does not claim exhaustive JSON schemas for
  every nested GUI field or every legacy direct-DTO tool. Write and recovery
  receipts acquire no new post-execution output gate.
- Safe external tool errors additionally expose **mcp-error/1** structured
  error/recovery data while preserving isError and original code/field/message
  text. Every hint has automaticRetry=false. Unknown/stale/integrity-sensitive
  writes preserve original intent and direct clients to operation/current-state
  inspection. Success and get_operation recovery receipts are unchanged.
- Optional tools/list `_meta.nestworth.toolGroups` provides request-scoped
  discovery. Full schemas/annotations match default discovery, shared evidence/
  recovery tools remain available, and signed cursors bind the normalized groups
  and running server. The mode-specific registered call allowlist remains the
  permission authority. Default discovery is still complete; a client that does
  not expose tools/list metadata is not claimed to use this optimization.

Actual SDK/HTTP measurements with final envelope schemas:

| Discovery request | Tools | HTTP calls | Response bytes |
| --- | ---: | ---: | ---: |
| Default full inventory | 104 | 7 | 190,499 |
| Explicit summary categories plus shared evidence/recovery | 9 | 1 | 17,897 |

The full inventory is **larger**, because output envelopes add 40,803 bytes.
The original 160 KiB non-output-envelope metadata guard remains after projecting
only that reviewed addition; measured other metadata is 149,696 bytes. A separate
192 KiB full-inventory ceiling, 24 KiB summary-subset ceiling and unchanged 64 KiB
per-response cap prevent unbounded growth. This is an explicit interface tradeoff,
not a claim that default discovery shrank or that all clients use subsets.

The typed SDK intentionally emits structuredContent and JSON text. An actual
account call delivered equal exact JSON in both representations: 1,301 bytes
each, 2,960 final HTTP bytes including escaping/envelope. The test checks semantic
identity with preserved numeric lexemes and counts both copies. No external
assistant/model-token consumption was measured. The built-in Eino capability
adapter passes one direct result string in its FunctionToolResult; that is a
separate path, established from its maintained source, not live model evidence.
Removing the SDK text fallback without consumer evidence could remove the copy
that a real client uses; both copies therefore remain. No token-saving claim is
made from these transport measurements.

Focused SDK checks cover subset size/schema equality, complete selected unions,
signed group/cursor mismatch rejection without echo, read-only write rejection,
unchanged calls outside a discovery filter, stable output identity/version,
structured expired-read/unknown-write handling and full oversized success receipts.
The shipped task-discovery protocol example itself is used by the SDK test.
Current static/schema coverage is **77** parseable JSON examples: **75** tool
arguments validated against actual registered schemas, one Inspector config and
one tools/list protocol-parameter example. This is not 77 end-to-end scenarios.

An initial full MCP attempt ran 231.206s and failed only the previous aggregate
160 KiB discovery-growth assertion; no functional test failed. The reviewed
output-schema cost is now separated while preserving that original guard for
other metadata. Both representative payload tiers subsequently passed:
20 accounts / 8 instruments, 5.233s, 119 calls / 1,555,875 bytes / maximum 64,165;
200 accounts / 25 instruments, 24.212s, 270 calls / 8,753,219 bytes / maximum
64,634. Each tier still probes 60 business tools. These are synthetic transport
measurements, not latency comparisons or loaded native CI timing claims.
