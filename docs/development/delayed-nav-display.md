# Delayed NAV display

Display quality is derived in `application/valuation_quality.go` from the same
selected components, immutable quote IDs and exact decimal amounts used by
strict valuation. Current, trend, panorama, series, level analytics and MCP
summary projections share that vocabulary. Existing strict selectors,
completeness, source preferences, snapshot revisions and return guards remain.

Snapshot trends hydrate evidence with one existing bounded capture, using its
1,000-leaf/100,000-fact/32 MiB input budget; changed input generation rejects the
read for retry. Unknown legacy evidence permits a labeled subtotal but does not
prove full availability. No schema migration or provider calendar is added.
Additive quality increases serialized response size. The existing MCP wire
budget is unchanged: captures exceeding it fail with deterministic `too_large`
instead of truncated dates. Narrow the requested range; pagination remains owned
by the independent MCP scalability task.

A carried 10-09 NAV of 2 values 100 units on 10-12 at 200 provisionally. Quantity
150 values at 300 from its reconstructed quantity, rather than copying 200.
A 10-12 NAV of 2.1 retrieved on 10-13 revises 10-12 to 210. Existing quote-import
invalidation revises dependent snapshots/analysis; no download-day gain or
synthetic 10-12 observation is created. A 10-13 economic quote cannot value
10-12. These examples and all verification households are synthetic.

`valuation_quality_test.go` exercises availability, distinct counts, carried
price/FX economic dates, missing liability, null versus zero, quantity changes,
no lookahead, rollover, read-only captures, late revision, conservative legacy
evidence, strict return isolation and final-requested-date level bucketing.
Frontend quality/clock tests cover three locales, gap styling and resume.
Aggregate tests retain existing source/binding/retraction, exact financial,
query cancellation, DST/skipped-date and read-budget regression coverage.

Linux server-profile builds and synthetic browser checks do not establish
native macOS/Wails, VoiceOver, packaging, signing or release acceptance.
