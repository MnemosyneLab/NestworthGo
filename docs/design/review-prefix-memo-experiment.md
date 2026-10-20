# Same-handle prefix memo: bounded experiment contract

This opt-in experiment extends authenticated D2, not ordinary C2 routing or
persistent E/shared facts. Normal D2 enablement and full rollback remain intact.
It claims neither O(new-days) reconciliation nor native/performance acceptance.

Private representation: immutable strings encoding complete typed input via the
existing lossless exact-FX codec, coverage, and daily result, plus exact key and
fingerprint/value metadata. These are typed-evidence encodings, not SQL tuples.
Only entries whose22-column SQL image exactly matched canonical arguments may
enter the memo. On reuse, authenticate the private content digest and envelope
before decoding fresh owned public values. No caller map, slice, decimal storage
or scan destination is retained. Decode/copy remains O(history), as do complete
source capture/fingerprint, policy/position/lifecycle replay, final typed-prefix
digest/MAC, unique retained/emitted partition and full protected-row sealing.

Bind each snapshot to this DB handle/key, household, exact committed predecessor
certificate digest and row seal, complete source fingerprint and exact token,
provider selection/revision, clock/proof/codec generation, and exact key/scope
membership. Every scope covers a contiguous prefix from its captured origin.
Full source reads remain mandatory because future events can change membership
without a journal write. Provider/rule/handling/source/clock/reopen changes miss
conservatively. Miss/eviction never grants or destroys certification authority.

Seed private candidates while the coherent reader still owns decoded data,
before exposing the capture. Carry only an authenticated receipt in the capture.
Candidate data stays private. After successful certified commit, promote only the
matching private candidate/generation and rebind its unchanged attested content
to the exact new committed certificate/seal. The unique partition guarantees
those retained keys were not written. Newly emitted UPSERT rows are NOT cached:
the next capture reads/attests the uncovered tail with the existing comparator.
SQL success alone is not canonical equality. Serialization mismatches stop that
scope's cached coverage, retaining ordinary comparison.

One committed and one pending immutable snapshot per DB handle. At most65536
entries and32MiB encoded payload per snapshot; at most64MiB encoded payload plus
bounded entry/scope metadata across both. These are eviction limits, not routing
thresholds or an RSS bound. Snapshot replacement is mutex-protected; readers use
immutable snapshots. A private generation observed inside the SQL transaction fences its postcommit
hook; stale hooks and stale/evicted receipts cannot replace newer state. No
private mutex spans SQLite commit. Failed/cancelled/CAS-
rejected/rolled-back publication cannot advance or promote it. No promotion reads
caller-owned evidence. Source/proof failure uses existing fallback; no authority
repair. Default D2 remains available independently of memo availability.

Validation starts with deterministic cold/warm/noop/next-clock and independent
full/durable/time/FX oracles, invalidation, missing coverage/content, alias and
stale-promotion checks. Heavy matrices stay local opt-in. Start performance small,
separate cold fill from repeated warm, report actual SQL decode/argument encode
counts, allocation, resident memory and writer/end-to-end time. At most10 useful
matched repetitions. Reject expansion if ownership/hash/GC/memory erase savings.
No workflow changes/dispatch, race gate, release, merge or real-ledger operations.

Certified noop is a checkpoint-writing commit: rebind only the existing immutable
content digest after that successful CAS, requiring identical source checkpoint,
row seal, provider and clock. It seeds no entries. Decoder still visits all prefix
entries on warm use; actual SQL decode/argument-encode counts are measured
separately. Optional observation-only configuration disables storage/reuse.
