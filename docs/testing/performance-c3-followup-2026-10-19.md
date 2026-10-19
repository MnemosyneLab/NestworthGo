# C3 follow-up: implementation boundaries and trigger integrity

PR129 remains an intermediate, default-off delivery. This follow-up stays on
`perf/c3-runtime-window-oracle` and the same PR. No race rerun, timeout increase,
CI change, workflow dispatch, activation, merge to main or release is performed.
The earlier global application race timeout remains an incomplete gate.

## Actual stage-two boundary

[Design section 5.3](../design/performance-optimization-plan.zh-CN.md#53-分阶段接入与-cas)
limits initial C3 acceleration to same-day no-op and publication recheck. A full
financial capture on clock-only calls is permitted until D/E. C3 does not
require a new dirty/suffix planner, shared financial fact storage or complete
clock-only incremental computation. Existing fixed-window correctness remains
relevant; optional full/bounded runtime diagnostics must not be mistaken for a
mandatory C3 exit requirement. Default activation and native p95/RSS acceptance
are separate gates from implementing and reviewing the C3 path.

Remaining cloud work is to remove unnecessary C3 verification/recomputation
cost, keep source/temporal/atomic proof intact, and provide a non-laundering
bootstrap for the supported existing/restarted state space. States whose frozen
history cannot be independently authenticated must retain explicit full fallback.

## Reproduced integrity finding and fix

The review's two trigger scenarios were reproduced using ordinary tests on an
immutable archive of exact head `0aa192f4a292eee6d17c8781ad615795369ebb12`, plus the
new regression fixture. No existing branch was edited for reproduction.

* An unknown permanent trigger installed before verified Open corrupted an old
  daily JSON row during a control self-update. Both the first and second no-op
  were accepted by the old code, retaining integrity authority.
* A TEMP trigger on review control updates changed a retained **resolved**
  episode's opening event. Legitimate ack, rule and suppression commands each
  re-signed the corruption. Dropping the TEMP trigger before the next capture
  did not remove the laundered authority. The permanent-trigger variants also
  reproduced. Full preparation does not rewrite this retained event, so the
  fixture cannot hide the defect by repairing a current daily row.

The certified verifier now checks an exact immutable contract for **all**
permanent triggers: financial source and reconciliation trigger names/bodies,
with no unrecognized trigger allowed. A schema cookie recorded by Open is not
sufficient. The compiled contract copies the existing source DDL map rather
than mutating that shared registry. TEMP object rejection remains.

Both before/after handling hooks verify schema, source tracking and the complete
writer contract before preserving/reissuing authority. If those assumptions
are unknown, the legitimate handling mutation may proceed, but cannot preserve
certified authority. A changed protected seal then prevents recertification,
even after the fault trigger has been dropped.

[Before-fix reproduction](artifacts/c3-followup-2026-10-19/trigger-before-0aa192f.log)
fails with both accepted no-ops and all six laundering cases.
[Focused after-fix tests](artifacts/c3-followup-2026-10-19/trigger-focused-after.log)
pass. The ordinary [certified suite](artifacts/c3-followup-2026-10-19/certified-unit-after-trigger-fix.log)
also passes (application 14.839s, SQLite 0.114s). No race was run for this change.
These are synthetic fixtures with a synthetic provider key and no provider
network or real ledger/credential access.
