# MCP import comparison QA — 2026-11-04

Task: [#211](https://github.com/MnemosyneLab/NestworthGo/issues/211).
Environment: isolated managed cloud Linux amd64, Go 1.27.3, synthetic temporary
SQLite and loopback MCP HTTP/SDK. No real household/CSV was opened or uploaded,
and no external market data was requested. Git identity: Walter Wang
`wangweichen1994@gmail.com`.

Branch `feat/mcp-import-comparison` started at main `85435e9` and was rebased
to latest main `51f498f` after #206/#207/#209 landed. At inspection,
[#210](https://github.com/MnemosyneLab/NestworthGo/pull/210) was an unmerged draft
at `4aff374eb925151c9ce87277e7e49a73fce61989`. This implementation does not
require its date-window or unified-history changes and targets main directly.
No stacked base or merge order is required. A later check found #210 still
unmerged at `89f5ef7`; its scope remains independent. #210's design-only comparison
proposal should defer to the [implemented API contract](../design/mcp-import-comparison-api.md)
if both changes land. This work creates a draft PR only; merging is not authorized.

## Evidence

The implemented endpoint compares 1–500 ordered rows / 100 distinct targets /
128 KiB against one read transaction, including inactive Agent quote history and
directly linked audit operations. Results retain exact input, client ID, ordinal,
batch duplicates/conflicts and decimal relations. Separate shared sections store
observations, audit records and current target policy/selection once. Hashes are
snapshot identities, not write CAS or import authorization.

Targeted synthetic application, SQLite and real SDK/HTTP regressions cover:

- Same value across Agent/provider/manual sources, provenance matching independent
  of value, competing equal and different active facts, NAV versus raw close,
  related unknown/converted/undated evidence and scoped missing versus unknown target.
- Instrument zero, tiny and maximum values; 8-place price and 12-place FX tails;
  rejected FX zero, overprecision, float-artifact lexemes, negative/exponent/NaN/Inf;
  format-only equality and exact nonzero display tails remaining conflicts.
- Original FX direction and a rounded reciprocal that cannot establish equality;
  selected current source/direction and conservative unknown simulation conditions.
- Market day versus UTC date; spring/fall DST, both repeated-hour offsets,
  missing-origin UTC fallback, invalid configured timezone, future/invalid dates,
  actual latest instants and explicit unavailable for unsupported submillisecond
  timestamps. Legacy RFC3339 forms remain discoverable within the queried second.
- Cross-date correction and later withdrawal, inactive state and original/target/
  replacement links, historical-only evidence instead of false missing.
- Reversed client order, same-key repeats with distinct IDs, exact-format repeats,
  input conflicts even with persisted absence, repeated-ID rejection, max rows,
  101-target rejection, UTF-8 ID byte bounds and request byte failure.
- Frozen evidence retry after a later authoritative synthetic write, cursor
  tampering/wrong-section rejection and capture expiry; ordinary generic cache
  tests retain cross-capture, eviction and context/permission revocation coverage.
- SQLite query_only and unchanged total_changes/portfolio facts/preferences;
  provider fake with zero calls; unauthenticated HTTP failure before disclosure;
  read permission matrix and unchanged pre-existing external discovery bytes.
- Atomic paired concurrent writer generations across two targets: captured
  current/history facts cannot mix generations. Query-only application comparison
  succeeds; no operation receipt, financial write, refresh or snapshot maintenance
  path is used.
- SQL failure, 20,001-row expansion, source-byte budget and JSON escape expansion
  exceeding the final 4 MiB budget fail closed. MCP's existing final 64 KiB
  enforcement and explicit oversized-page-item behavior remain in force.

Skill 2.2.0 is a compatible minor addition above main/allocated 2.1.0. Main,
tags, VERSION history and all GitHub releases (including draft visibility) were
checked; published v0.3.10 still carries skill 2.0.0. The installation guide keeps
published history separate. No release asset or installed user skill was changed.

The production writer's time/value/provenance parsers were extracted into shared
pure functions; no write entry point is invoked by comparison. Invalid synthetic
candidates are also checked against authoritative writer validation to retain
parity. Earlier failures were fixed: the added capability required additive
local/SDK discovery counts, the SQL row fixture needed valid UUIDs, and legacy
RFC3339 instant lookup needed a bounded whole-second prefilter before exact
parsed-time comparison. No assertion was removed to bless a broken contract.

## Workload measurement

The deterministic fixture has 35 instruments and 233 stored Agent NAV facts,
with 233 ordered proposed rows. Setup uses three authoritative 100-item import
batches only to populate temporary test data. Setup calls are excluded from the
read measurement. Every returned section is traversed completely; continuation
limits match the initial call's explicit/default limit.

| Read path | Calls | Final JSON-RPC bytes, total | Maximum actual response | Maximum reserved response | Frozen bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Comparison, default 25 | 40 | 893,198 | 64,216 | 65,096 | 385,888 |
| Comparison, explicit 100 | 16 | 851,162 | 64,521 | 65,400 | 385,888 |
| Existing history, explicit 100 | 73 | 567,548 | 63,455 | — | — |

The comparison reads all 233 results, 233 distinct matches, 233 original evidence
rows, 233 audit rows and 35 current-target rows. The existing history path reads
233 observations, selected chart points and target metadata, but does not compute
import classifications or prove absence of withdrawn facts. It is not equivalent
functionality, and comparison's richer evidence uses more total bytes. Request
IDs/cursors/UUIDs can slightly change byte sizes. Responses stay under 65,536
bytes and captures under 4 MiB. No latency, p95, global throughput or one-response
claim is made. The writer still caps each later authorized import at 100 items.

## Check scope and remaining gates

Passed on the final implementation after rebasing to `51f498f`:

- `go test ./internal/...`: all packages passed, including application 193.749s,
  SQLite 39.756s and MCP 83.719s under this shared cloud run. These are observed
  test durations, not product performance measurements.
- `go vet ./internal/...`, gofmt and `git diff --check`.
- 136 Python tooling tests, 70 skill JSON/reference examples, the `2.1.0 -> 2.2.0`
  distribution version guard, Go skill workflows and packaged checksum.
- The archive's VERSION and all 16 payload files equal their repository sources.
  Packaging used only the ignored local `dist/skills` directory; nothing was
  published or installed into a user skill directory.

Final exact-head GitHub Actions links/status are recorded in the draft PR. A full `go test ./...`, `go vet ./...`
and `go build ./cmd/nestworth` are attempted. The desktop target cannot compile
locally because GTK4/WebKitGTK6/libsoup and glib/gio development packages are
missing. This is an environment build limitation, not a passing desktop check.
No frontend implementation or generated bindings are changed. Full native Mac
CI, GUI interaction, packaging, signing and notarization are separate evidence;
none is inferred from these Linux checks. CI monitoring belongs to the coordinator;
hand off the exact head/run links without keeping this environment solely waiting.

Missing means no comparable retained local fact in this capture's scope, never
that a target had no historical price anywhere. Non-price closures and unlimited
history are outside scope. Source equality is not receipt idempotency. Selection
impact models a single current append only; historical/batch impacts, generated-ID
ties, absent FX preferences and converted dependencies are explicitly limited or
unknown. Separate captures have distinct snapshot times. No comparison result
permits automatic import, overwrite, priority change or provider request.
