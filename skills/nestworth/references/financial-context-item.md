# Drill into one frozen financial-context item

When the question concerns a returned account, position or evidence alias, call
`get_financial_context_item` with its original `(contextId, ref)` pair from
[financial context](financial-context.md). This is the same captured projection,
not a fresh account snapshot. The item inherits the original package disclosure: minimal stays free of names/raw IDs; named preserves only the names and IDs
already disclosed there. This is not an additional disclosure choice and never
upgrades minimal to named or recaptures a package. Do not read the directory or
switch to identity-bearing tools to explain a minimal item.

<!-- example: financial-context-item -->
```json
{"contextId":"${contextId}","ref":"${ref}"}
```

The response carries the original contentHash, capturedAt, cacheExpiresAt, asOf
and basis. `position` is the selected account or position. `evidenceItem` is
selected evidence. `positions`, `gaps` and `evidence` contain only related frozen
rows. An account's `position` is a rollup and its `positions` are the cash and
holding components. Never sum the rollup again with those children. A position
has no child rows. Evidence lists only direct referring rows and gaps that name
it. Shared FX does not expand each user's account or other evidence. Evidence in
account and position results is deduplicated by ref. Preserve excluded, archived,
not_created, measured zero and missing amounts as returned.

The initial response has at most one row per related section. Inspect all three
page descriptors. Supply section to read a related section, even if empty, then
follow its nextCursor with `get_read_page({cursor, limit?})`. Its signed scope
retains contextId, ref and section. A zero-row initial section with hasMore is
deferred. Limits default to 25 and the maximum is 100. Whole-package page cursors
and another item's cursors are not interchangeable. Keep the three initial
descriptors independently and update only the requested section after each
section, items and page continuation. Retain ref and the initial target and
header. Continuations omit unrelated descriptors.

<!-- example: financial-context-item-page -->
```json
{"cursor":"${cursor}","limit":25}
```

Current and closed-day packages work identically. There is no transaction
history, new identity disclosure, period comparison or recomputation here. If
the projection lacks an answer, state that boundary. Do not invent a history.
The original five-minute expiry is fixed. Expired or revoked packages require a
new capture and a newly selected target. Never attach an old answer to a
same-spelled alias in the new package, even if the hash matches. Bare aliases
cannot reveal which package they originally came from. Keep their contextId
throughout. The same single-request, wire-budget and too_large rules as the
parent package apply.

## When the item fails

An item `too_large` error for a section row plus the required target names the
section (`positions`, `gaps` or `evidence`). If the initial item overview fails
this way, request other sections explicitly with the same contextId and ref,
omitting cursor to start each section. This can recover the other related data.
Report the failed section as unavailable and the item detail as incomplete.
Lowering limit cannot fix that oversized row. An oversized required target is
not section-specific and cannot be bypassed by selecting another section.
On `context_expired` or revocation, discard the item and its pages, recapture
with `get_financial_context`, and select the target again in the new package.
