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
