# Reconcile accounts

Open **Reconciliation**, select the accounts you want to check, and preview the
new session. Active accounts are selected by default. The session keeps its
selected scope and your progress when paused or after restarting the App. Open
an account to see its last successful reconciliation and links to saved records.

Check the bank/broker statement against each native cash balance, ordinary
holding and managed product. Also check completeness: matching the known
positions does not establish that an instrument or currency is missing. Prices,
NAV and FX can have different publication times; those valuation differences do
not by themselves invalidate a cash or quantity check.

For a difference, choose what actually happened:

- **Correct current balance or holding** records a visible audited adjustment.
  An added quantity needs the added units' unit cost or the total cost of the
  entire remaining position. These are different inputs; do not combine them.
- **Record missing income, expense, buy or sell** opens the existing ledger form.
  Record the real event and its actual date/time. This does not place an order.
- Managed products use their product terms, actual value or receipt/lifecycle
  operations. Maturity does not automatically mean money was received.

Preview exact results before committing. Changes update Nestworth only. The
correction is linked to the session and returns you to the same check. Review
the refreshed evidence and explicitly confirm it; a saved correction never
confirms a point automatically.

Each confirmation needs a statement/source reference and its as-of date. Supply
a source time only when known; it uses the ledger timezone. The App separately
records when you actually checked the point. You can skip or leave an unresolved
difference with an explanation, or keep it pending.

Finish fully when all required points and completeness have been checked. To
finish with outstanding work, explicitly select partial completion and explain
why. The record shows confirmed, pending, skipped and unresolved counts and
per-account outcomes. Only fully confirmed accounts advance their last-success
time. Completed evidence is immutable. Later financial edits show a recheck
indicator beside the original record and never delete its historical date.

If a submission's outcome is unknown, keep the review open and retry the same
submission. Do not create a second correction to compensate for an uncertain
response. MCP users can follow the maintained Skill's session workflow with the
same evidence, revision and receipt rules.


Choose **Continue session** for unfinished work. When selected accounts overlap
an unfinished session, the start panel explains that a new session is a separate
record; it does not lock an account globally. **Back to session list** returns to
the current account's filtered list. **View last successful record** opens that
historical completed record.

**Anything missing?** appears first and lets you record omissions immediately.
Holding transactions start with a trade for that holding; cash starts with cash
flow. Archived positions with units remain visible with a warning. Removed
items retain their evidence and can be given a disposition, with correction
actions hidden.

Use **Compare: same units, currency and current as-of basis** only when those
conditions hold. A differing numeric reference is shown as a mismatch. Correct
the ledger, leave the check unresolved, or explain the difference before
confirming. Older statements and text can stay as evidence without comparison.
There is no automatic confirmation or mandatory copying of statement numbers.

Opening notes and closing explanations are saved separately. A blank finish
does not erase the opening note. Paused sessions can finish directly. The
completed record keeps its original interval; later bookkeeping is listed
separately and does not erase the historical success timestamp.

On a known revision conflict, current evidence refreshes. Close the old review,
read the changed figures and preview again. A completed record is read-only.
Unknown outcomes keep the same submission identity and require explicit retry.
Desktop reconciliation previews now keep private recovery material across
restart. Unavailable older plans can still remain unknown; do not compensate by
creating another financial write. JSON export includes business revisions and
change logs, and excludes private plans and receipts. It is not a restorable backup.
