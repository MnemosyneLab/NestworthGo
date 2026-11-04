# MCP audit follow-ups

## Published baseline and scope

Baseline: main `a46d38bbbf84f9908506463dae1e27d28f62b163`.

- #212: bounded read-only import comparison, compact classification and same-capture detail.
- #215: frozen contribution list and drilldown, signed continuation and revocation.
- #216: reconciliation pages fit the complete 65,536-byte JSON-RPC response, including both SDK representations; write/recovery receipts bypass read interception.
- #217: compact identity directories and lean frozen financial continuation pages; skill 4.0.0.
- #220 remains separate: account/Agent SQL target/date pushdown and scoped counts. This follow-up does not modify its account/Agent/application/repository files.

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
