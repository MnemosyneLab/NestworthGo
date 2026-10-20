# Daily investment review

Status: V2 daily rules are the active investment-review feature. The earlier
opt-in V1 panel, preferences API, MCP tools and conversion flow were retired
because they had not been used. This document describes the current contract.

## Scope and user interface

Investment review is a rule-based reminder to inspect data or an investment;
it never recommends selling or records financial facts. Investment review is the second tab of Tasks & checks, after Upcoming actions
and before Reconciliation. It groups active alerts by investment, shows rule
conditions and daily evidence, and supports filters, acknowledgement,
suppressions, rule history and explicit refresh. Overview shows one Tasks &
checks summary; the review count opens this tab. There is not a second review
summary on the homepage.

Rules are named and target selected instruments or all current holdings. Each
rule has a closed predicate and a separate trigger policy: one daily hit,
consecutive hits, or a minimum number of hits in a calendar window. Evaluation
uses closed household civil dates in a fixed IANA timezone. Missing or unsafe
evidence is unknown and does not count as a hit. Today is preview-only. A rule
can be paused, edited, or archived; archival retains evidence and history.

## Evidence and calculations

The application layer evaluates persisted local financial facts and published
quotes. It owns eligibility, exact decimal calculations, currency conversion,
missing-data handling, quote admission, source fingerprints, correction replay,
episode identity and publication. The frontend displays returned results and
does not recalculate financial values. Reads show the last complete publication;
refresh is explicit and does not fetch provider quotes.

Daily outcomes are true, false or unknown. Unknown days never become hits. The
trigger policy determines when true days open an episode; supported false evidence
can resolve it. An acknowledged episode remains historical and does not suppress
a later, distinct episode. Evidence and events retain the original condition
version so edits do not reinterpret prior history.

## Acknowledgement and suppression

Acknowledgement applies to one episode. A suppression can apply to one instrument
for all rules, or to one rule, and can be permanent, temporary for supported
calendar days, or end on an explicit civil date. These controls are distinct:
suppression does not acknowledge an alert, and acknowledgement does not suppress
future rules. Writes use revision checks and durable operation receipts. On an
unknown outcome, inspect the operation and current state before making a new
change.

## Persistence and historical data

Schema 19 tables hold daily rule definitions, immutable condition versions,
evaluation inputs/results, episodes, events, suppressions and mutation receipts.
Historical migration files and schema verification remain intact. The v18-to-v19
upgrade preserves its existing behavior: it stores the original V1 preference
JSON and hash in archival migration state, keeps malformed/orphaned keys in
quarantine, and creates a migration-only `legacy_v1` compatibility rule for
valid preferences. Active V2 queries select daily rules only; the old settings
reader remains read-only for recovery. The verified recovery copy remains part
of the upgrade. No schema or ledger data is changed by this cleanup.

## Validation references

Use [daily rollout details](../development/investment-review-daily-rollout.md),
[real browser acceptance](../development/real-browser-acceptance.md), and the
[dated synthetic acceptance evidence](../testing/investment-review-daily-acceptance-2026-10-16.md).
The dated report records earlier implementation-time checks and is historical
evidence, not a claim that retired V1 interfaces remain available.
