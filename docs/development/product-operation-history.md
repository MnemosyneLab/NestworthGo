# Managed product operation history

Product detail opens one cursor page of 25 rows. **Load more** requests one next
page explicitly; no eager draining, infinite scroll, totals or page numbers are
used. Rows retain the existing descending creation-time/UUID keyset order,
independent of effective time. Each row's `reversedByOperationId` resolves the
inverse across the complete scoped history.

The page includes `latestRecordedOperationId` and `undo` with operation identity,
kind, effective/creation times, allowed and a nullable typed reason. The global
candidate is independent of the cursor: reversals and reversed originals are
excluded, but valuations remain candidates and block older lifecycle undo. An
empty page may still have a global candidate. Missing or foreign products fail
with not-found rather than proving existence through an empty page.

A read-only SQLite transaction materializes the page and complete candidate
facts coherently. The shared in-memory undo planner evaluates one candidate,
including linked-product order, revision/restoration receipts, exact inverse
cash/quantities, reservation revisions and release times, and archived account,
holding and instrument restrictions. Dependencies are batched by entity type;
complete linked histories are neither capped nor returned to the caller. No
schema, persisted cache, analytics materialization, provider call, preview token
or financial rule change is introduced.

Each mounted sheet owns a cancellable query session. Explicit refresh, relevant
invalidation, product switch and close/reopen discard the page chain and request
one first page. A different head on a next page restarts from the first page.
Successful pages use the newest undo summary because non-history edits also
change eligibility. Initial loading, empty and failed reads are distinct; next
page failure retains rows and retries the same cursor. Refresh/error disables
undo and invalidates the reviewed command. A changed candidate never silently
retargets an undo draft. Late results cannot affect another sheet session.

Load more and retry use canonical localized controls. Pending and completion
are announced. Additional rows retain focus on Load more; when its final page
removes that focused trigger, focus moves to the adjacent completion status only
if the user has not moved elsewhere. Existing civil dates, timezone-formatted
history timestamps, names, currencies, exact amounts and confirmation UI remain.

Availability is current GUI information, not write authorization. Preview and
commit independently rebuild and validate the operation; reviewed hashes,
coordinator locks, durable mutation receipts, reservation CAS, unique inverse
links, archived-target SQL guards and atomic bundle commit remain mandatory.
MCP reads share the DTO and purity contract but keep their stricter permissions
and reservation restrictions. See [MCP products](mcp-products.md).

Automated synthetic tests cover long/off-page restoration, refusal parity,
keyset ties, fixed statement counts, separate-connection snapshot interleaving,
read failure/cancellation, stale commit rejection, MCP parity/purity and frontend
session/error/focus behavior. Server-tag Linux and jsdom/cloud browser checks do
not establish native macOS, VoiceOver or packaged-app acceptance.
