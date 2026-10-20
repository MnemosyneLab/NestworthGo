# E2a: bounded authoritative shared inputs

E2a implements a real mixed-format production reader/writer, **disabled by
default** because the two small observations below show higher total storage
and publication work. It is reached by the internal repository copy constructor
`WithReviewSharedInputs(true)`, not by normal desktop initialization. This is
implemented and synthetically exercised, not deployed or a production speedup.
The prior design-only assessment in this PR is superseded by this contract.

## Supported subset and admission

Only a fully prepared publication with no suffix, reusable window inputs or
certified full-oracle alternative is admitted. At most **64 emitted rule-days**
are considered; over-budget publications use inline inputs in full. Within that
publication, only groups with two or more different rules and exactly identical
instrument, semantic key, source provenance, complete payload and coverage share.
Threshold/operator differences share; financial requirements, currencies, FX
modes, lookbacks, provider, calendar session and policy differences do not.
Unsupported and nonexistent dates remain inline. A repeated output key rejects
admission. Neither an E1 sample nor an old reference grants authority.

Admission uses the existing conservative 256 KiB estimated new-column budget,
4,096 lifetime facts, 16,384 lifetime refs and 8 MiB encoded-column bounds,
including E1 diagnostic rows. These are column/admission bounds, not a database
file-size guarantee. Exhaustion falls back to inline publication without
removing evidence. No historical migration, startup backfill, GC, checkpoints,
B activation or PR143/144 changes are included.

## Representation and complete membership

The existing daily-input primary key remains the complete per-date membership
and **current selector**. Its `evidence_json` is either the legacy complete input
or a canonical `{"SharedFactV1":"<immutable-fact-id>"}` object. Every date still
has its normal input row, exact scalar columns, coverage and fingerprint. Daily
results and all handling/lifecycle records retain their existing representation.
Alert amount/cost readers therefore continue to read current scalar columns.
No date universe is inferred from sparse diagnostic refs or min/max ranges.

A separate semantic namespace, `e2a-complete-input-v1`, stores one **complete**
financial input in an immutable `base_v1` fact. E1's `base_v1` and `metrics_v1`
representations remain under their separate `e1-v1` namespace. This deliberate
simplification avoids independently selecting incompatible base/metric versions.
The complete-input codec preserves private exact FX scalars. Only
`internal/jsoncodec` is used.

Each selector also has an exact `review_result_fact_refs` record binding rule,
condition, instrument, timezone, date, fact ID and input fingerprint. Existing
restrictive foreign keys retain rule/instrument/fact existence; the resolver
additionally verifies household ownership, instrument, kind, namespace/date/
timezone, coverage and the complete reference key. Missing or malformed refs
fail; they never become empty financial inputs. Input decoders retain exact
identity checks. No new table or mutable fact union is introduced.

The selector is the authoritative current version; older append-only refs are
history, never alternate current candidates. SQL row membership plus the exact
selector/ref/fact relation replaces the separate manifest/selector tables
considered in the initial assessment. This applies to each admitted date, and
permits explicit inline/normalized dates in the same scope.

## Publication, proofs and lifecycle

Publication retains every existing source/rule/handling/provider/time/suffix
fence. In the same transaction it compares all semantic/source/payload/coverage
bytes, writes each distinct immutable complete fact once, writes exact refs,
resolves persisted bytes, compares them with the complete staged input, and
compares the unchanged rule evaluation with its staged result. It then writes
selectors, scalars, results, lifecycle, control and temporal state atomically.
Failed/late/cancelled publication rolls back all these writes.

The normal admitted representation replaces the old full input JSON with the
selector; it does **not** keep another full per-rule input payload. Scalars and
coverage still repeat. Rule results and episodes intentionally remain separate.
Correction appends versions and replaces current selectors; no old version or
ref is updated/deleted. Same-condition opening evidence may change while episode
identity and acknowledgement survive, matching the baseline.

The complete row seal now includes exact shared fact and ref rows for the
household, including historical versions. Certificate completeness moves to
`certified-full-review-v4` and row-byte sealing to `certified-row-bytes-v3`.
The existing input row counts continue to count every date, including selectors;
missing refs are additionally covered by the sealed relation and strict resolver.
No new checksum can bless old data. Source MAC/CAS, provider lease, connection
checks and full source fallback remain intact.

Both the baseline window reader (B remains disabled) and suffix reader resolve
selectors inside their existing coherent transaction. Suffix attestation compares
the exact resolved complete input and the original full raw SQL selector/scalar/
coverage/result image. It does not replace exact comparisons with hashes. Private
prefix memo entries contain owned complete bytes bound to the new complete seal.
Each decode owns its maps; financial values do not alias between rules. The
publication equality reader retains raw images and compares against the actual
planned inline/selector SQL parameters.

## Restart, corruption, rollback and compatibility

Restart/restore retains readable storage but loses same-handle certification.
Full preparation from authoritative sources remains the fallback and does not
acquire authority merely by checksumming persisted facts. Corrupt shared bytes
or refs invalidate an existing sealed chain. A direct window reader reports
invalid refs rather than silently omitting dates; forced full preparation does
not read them and can restore inline authority. Damaged fact rows remain retained
for diagnosis; enabling writes again can reject disagreeing stored versions.

The practical rollback is `NESTWORTH_REVIEW_FULL_RECOMPUTE=1` before launch, or
`WithFullReviewRecompute()` internally. It disables normalized writes/reuse and
reconstructs inline inputs from authoritative sources in the existing atomic
publication. The input UPSERT explicitly replaces a selector even when the input
fingerprint is unchanged. Current scalars, result/lifecycle behavior and ack are
preserved. Simply disabling shared writes keeps mixed-format reading available;
it does not promise immediate conversion of retained suffixes. No old table,
shared version or handling record is deleted.

Schema **24** is a format fence even though table definitions are unchanged.
A schema-23 binary must not open a ledger containing this new input format.
Verified v23 databases get a verified, flushed, read-only `original-v23.sqlite`
recovery copy before the version upgrade; no financial rows are replayed.
Older supported schemas still upgrade sequentially. Read-only inspection accepts
v23 without upgrading. Current snapshots/reopen preserve selectors and versions.
Algorithm rollback stays on schema24. An old binary requires a separately restored
verified pre-upgrade copy, with later financial writes accounted for explicitly.
Peak disk includes old rows, new facts/refs, indexes, WAL and recovery copies.

## Two small paired observations

Linux amd64, Go1.26.0, `GOMAXPROCS=5`, synthetic temporary SQLite databases only.
Each arm opens its own snapshot of the same unpublished fixture. One instrument,
eight civil days, two or eight threshold rules; one publication and one subsequent
C2 capture/window read per arm. This is reopened storage, not a warm prefix memo.
The observation was rerun once after the selector UPSERT predicate was corrected;
the table reports the final code only. No repeated suites, race run, global/p95/RSS
or native Mac claim.

| Rules | Representation | Input/fact/ref rows | Input/fact/ref column bytes | Total bytes | Publication | Capture + window |
| --- | --- | --- | --- | --- | --- | --- |
| 2 | Inline | 16 / 0 / 0 | 8,510 / 0 / 0 | 8,510 | 19.35 ms | 48.52 ms |
| 2 | Shared | 16 / 8 / 16 | 5,276 / 16,982 / 3,248 | 25,506 | 21.33 ms | 6.05 ms |
| 8 | Inline | 64 / 0 / 0 | 34,040 / 0 / 0 | 34,040 | 12.49 ms | 7.40 ms |
| 8 | Shared | 64 / 8 / 64 | 21,104 / 16,982 / 12,992 | 51,078 | 15.46 ms | 12.57 ms |

Results/lifecycle rows are unchanged and excluded from both totals; all input,
fact and reference columns are counted. SQLite record/index/page/WAL bytes are
excluded. The first arm's slow capture illustrates why one sample is not evidence
of a reader speedup. The consistent storage increase alone blocks normal activation.

Shared publication adds three aggregate admission queries, three statements per
new fact (lookup/insert/readback), and two per ref (insert/exact resolver readback):
59 extra statements for two rules and 155 for eight, in addition to unchanged
input/result writes and existing guards. It also evaluates persisted reconstructed
inputs during admission. The subsequent one-day window reads 2 or 8 inputs;
shared rows add one fact/ref lookup each and still decode once per rule-day.
There is no decoding reduction in this slice. Whole-row sealing includes the
new historical rows when certification is used, adding further work. The paired
observations use direct C2 publication, so do not measure that certified cost.

Normal activation remains off: complete source/key provenance and reference
metadata outweigh saved payload for these inputs. A later improvement must
reduce this measured overhead or establish a demonstrably beneficial admitted
payload without weakening complete proofs. This is not a request to enlarge the
admission budget, migrate history, remove exact comparisons or ship double writes.

## Focused validation

Ordinary new application checks cover complete inline/shared equality, thresholds
and differing lookbacks, exact FX, correction/opening/identity/ack, transactional
late failure and cancellation, restart, actual certified suffix read/attestation,
shared-payload damage, missing refs, budget fallback, forced-inline rollback and
Apia's nonexistent date. Migration checks cover v23 flush failure, read-only
recovery, no backfill and snapshot/reopen. The tiny paired observations are behind
`perf`; they do not expand ordinary PR checks. Existing affected SQLite and
selected application regression checks are recorded in the PR.

Native macOS, desktop packaging/launch, race and broad performance validation
remain unexecuted. CI elapsed-time review belongs to the coordinator; no polling
or workflow changes are part of this PR.
