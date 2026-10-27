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

Review priority uses P0 (highest), P1 and P2. The API's `severity` field accepts
`p0`, `p1`, `p2`, plus historical `warning` (P1) and `notice` (P2) aliases.
Schema23 adds a priority table without rewriting the old severity column or
condition versions. An absent priority row uses the historical mapping. Priority
edits retain mutation receipts, CAS and projection invalidation. The complete
schema allowlist and certified row seal include priority records. Alert groups
and rules sort by handling status, then priority, then stable identity; cursors
include the complete order key and remain bound to the publication and filter.

Why-alert summaries and daily history attach the immutable condition version's
predicate at read time. The UI displays its AND/OR tree, numbered conditions,
lookback, comparison/threshold, actual evidence and each leaf's own truth.
The whole-rule result is separate. Stored votes and historical receipts are
never rewritten to add display context.

Simple annualized price/NAV change supports bank investment products, mutual
funds and ETFs. Review validation, daily evaluation and descriptive returns use
the same explicit type eligibility and decimal projection: interval percent ×
365 / actual civil days. Every annualized branch needs an inclusive type gate
containing only supported types in an AND ancestor; OR siblings and exclusion
gates cannot scope it. Descriptive history ends at the latest actual daily quote;
review additionally applies its existing evaluation-day carry, freshness and
baseline-gap policies. These are historical instrument returns, excluding personal
trades, fees and transfers, and are neither personal holding-period returns nor
quoted product yields. Raw-close ETF changes exclude dividends; NAV histories
with distribution evidence and unsafe split/basis/currency/binding histories
remain unavailable. Missing or invalid evidence is not an invented zero return.
Other priced types retain period price changes without this annualized field.

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
Supported schemas 21/22 may contain original V1 preference JSON and hashes in
archival state, malformed/orphaned keys in quarantine, and `legacy_v1` rules.
These immutable records remain valid supported data. Active V2 queries select
daily rules only; the old settings reader remains read-only for recovery.
Pre21 SQLite migrations are no longer supported.

## Validation references

Use [daily rollout details](../development/investment-review-daily-rollout.md),
[real browser acceptance](../development/real-browser-acceptance.md), and the
[dated synthetic acceptance evidence](../testing/investment-review-daily-acceptance-2026-10-16.md).
The dated report records earlier implementation-time checks and is historical
evidence, not a claim that retired V1 interfaces remain available.


## Financial source tracking maintenance

The compiled source manifest and schema/trigger verification tests are the
executable authority for this dependency inventory. Financial read-closure
changes must update both the inventory and those tests.

## Read closure and conservative invalidation

The audit follows `review_historical_batch.go:loadReviewHistoricalBatchQuery` through `historical_batch_repository.go:loadHistoricalSnapshotBatchQueryMode`, the review portfolio loader, origin data, observations/preferences/binding revisions, activity/effect/correction loaders, complete quote/FX histories, coverage/slots/day status, and `gain_snapshot.go:readReviewGainCosts` and `cost_basis_repository.go:costBasisEventsSelect`. `ReviewHistoricalFingerprint` marshals the complete returned batch and deterministically adds private exact decimal scalars. It includes `history_snapshot_state.input_generation` and resolver policy, plus current portfolio latest account/cash values. It excludes reconciliation/review state.

Two easy-to-miss dependencies are included: latest account/cash queries **do read replay rows**, and quote/FX queries anti-join **agent_quote_records** to exclude withdrawals. Therefore account/cash replay DML is conservatively tracked; publication writes neither. Holding quantity replay rows are absent from this read closure and are excluded, but source↔replay transitions are tracked. Changing these decisions requires a separate read-closure proof, not renaming a derived row. Exact financial values remain solely in source tables.

Schema enforces one household (`singleton_key = 1`, UNIQUE). Every relevant DML invalidates **all currently existing households**, with `dependency_kind=household_all`. OLD and NEW metadata are retained independently, including ownership/identity/time moves; neither side is inferred from the post-edit database. This safely over-invalidates catalog/configuration/product changes and shared dependencies. NULL effective time means unknown/full-origin fallback, never “no effect”. Created timestamps alone are not economic times. During parent deletion, a child lookup may return NULL; full-household invalidation still applies. Household removal removes its own metadata by FK; a later household gets a new random epoch. Invalid referenced households are rejected by FK/schema verification.

## Per-table / per-field / writer matrix

All listed columns participate in insert/delete and null-safe update comparisons, **except** `history_snapshot_state`: only `household_id,input_generation,resolver_policy_version` participate in updates. Its dirty/completion/timestamp fields are derived. Identical SQL updates produce no journal entry. `holding_quantity_values` rows with `projection_kind=replay` on both sides are excluded. This table is a conservative superset of the read closure: product/liquidity source metadata and configuration are included even where current review does not directly read their payload.

Writer files below are under `internal/infrastructure/sqlite/`. They were audited against all non-test SQL DML call sites, including UPSERTs, and dynamic table helpers. The table/column manifest is checked against actual schema with `TestFinancialSourceManifestMatchesActualSchema`; every known application table must have an explicit source/excluded classification. An unrelated extension table remains compatible with existing database policy. Adding a financial read dependency requires updating this audit and classification before any future fast path is enabled.

### `households`

- Fields: `id`, `singleton_key`, `name`, `base_currency`, `created_at`, `updated_at`.
- Write owners: `history_repository.go`, `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'base_currency',OLD.base_currency,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `members`

- Fields: `id`, `household_id`, `name`, `icon_key`, `note`, `sort_order`, `created_at`, `updated_at`, `archived_at`.
- Write owners: `history_repository.go`, `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `institutions`

- Fields: `id`, `household_id`, `name`, `institution_type`, `country_code`, `website`, `note`, `sort_order`, `created_at`, `updated_at`, `archived_at`, `icon_key`.
- Write owners: `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `account_groups`

- Fields: `id`, `household_id`, `name`, `icon_key`, `color`, `description`, `sort_order`, `created_at`, `updated_at`, `archived_at`.
- Write owners: `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `accounts`

- Fields: `id`, `household_id`, `institution_id`, `group_id`, `name`, `account_type`, `balance_sheet_role`, `tracking_mode`, `default_currency`, `note`, `include_in_net_worth`, `include_in_portfolio`, `include_in_liquid_assets`, `opened_on`, `closed_on`, `sort_order`, `created_at`, `updated_at`, `archived_at`, `icon_key`.
- Write owners: `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'institution_id',OLD.institution_id,'group_id',OLD.group_id,'opened_on',OLD.opened_on,'closed_on',OLD.closed_on,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `account_ownership`

- Fields: `account_id`, `member_id`, `share_bps`.
- Write owners: `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('account_id',OLD.account_id,'member_id',OLD.member_id)`.
- OLD/NEW effective time expression: `NULL`.

### `account_values`

- Fields: `id`, `account_id`, `value_kind`, `amount`, `currency`, `effective_at`, `created_at`, `activity_effect_id`, `projection_kind`.
- Write owners: `change_repository.go`, `derived_data_rebuild.go`, `repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'account_id',OLD.account_id,'currency',OLD.currency,'effective_at',OLD.effective_at,'created_at',OLD.created_at,'activity_effect_id',OLD.activity_effect_id,'projection_kind',OLD.projection_kind)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `instruments`

- Fields: `id`, `household_id`, `name`, `instrument_type`, `quote_currency`, `symbol`, `market_code`, `country_code`, `isin`, `note`, `icon_key`, `sort_order`, `quote_source`, `provider_key`, `provider_symbol`, `created_at`, `updated_at`, `archived_at`, `metal_template`, `quantity_unit`, `short_name`.
- Write owners: `instrument_metadata_batch.go`, `portfolio_repository.go`, `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'quote_currency',OLD.quote_currency,'provider_key',OLD.provider_key,'provider_symbol',OLD.provider_symbol,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `holdings`

- Fields: `id`, `account_id`, `instrument_id`, `quantity`, `note`, `sort_order`, `created_at`, `updated_at`, `archived_at`.
- Write owners: `change_repository.go`, `derived_data_rebuild.go`, `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'account_id',OLD.account_id,'instrument_id',OLD.instrument_id,'created_at',OLD.created_at,'archived_at',OLD.archived_at)`.
- OLD/NEW effective time expression: `NULL`.

### `account_cash_values`

- Fields: `id`, `account_id`, `amount`, `currency`, `effective_at`, `created_at`, `activity_effect_id`, `projection_kind`.
- Write owners: `change_repository.go`, `derived_data_rebuild.go`, `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'account_id',OLD.account_id,'currency',OLD.currency,'effective_at',OLD.effective_at,'created_at',OLD.created_at,'activity_effect_id',OLD.activity_effect_id,'projection_kind',OLD.projection_kind)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `instrument_quotes`

- Fields: `id`, `instrument_id`, `unit_price`, `currency`, `source_kind`, `source_key`, `quoted_at`, `created_at`, `delayed`, `observation_kind`, `effective_date`, `provider_timestamp`, `fetched_at`, `value_effective_at`, `binding_revision`, `source_policy_version`, `price_basis`, `timestamp_basis`, `revision`, `supersedes_quote_id`, `split_factor`, `dividend_cash`, `conversion_json`.
- Write owners: `agent_quotes.go`, `converted_quotes.go`, `market_history_repository.go`, `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'instrument_id',OLD.instrument_id,'currency',OLD.currency,'source_kind',OLD.source_kind,'quoted_at',OLD.quoted_at,'created_at',OLD.created_at,'observation_kind',OLD.observation_kind,'effective_date',OLD.effective_date,'provider_timestamp',OLD.provider_timestamp,'value_effective_at',OLD.value_effective_at,'binding_revision',OLD.binding_revision,'source_policy_version',OLD.source_policy_version,'price_basis',OLD.price_basis,'timestamp_basis',OLD.timestamp_basis,'supersedes_quote_id',OLD.supersedes_quote_id)`.
- OLD/NEW effective time expression: `COALESCE(OLD.value_effective_at,OLD.effective_date,OLD.quoted_at)`.

### `fx_quotes`

- Fields: `id`, `household_id`, `base_currency`, `quote_currency`, `rate`, `source_kind`, `source_key`, `quoted_at`, `created_at`, `delayed`, `observation_kind`, `effective_date`, `fetched_at`, `value_effective_at`, `source_policy_version`, `timestamp_basis`, `revision`, `supersedes_quote_id`.
- Write owners: `agent_quotes.go`, `market_history_repository.go`, `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'base_currency',OLD.base_currency,'quote_currency',OLD.quote_currency,'source_kind',OLD.source_kind,'quoted_at',OLD.quoted_at,'created_at',OLD.created_at,'observation_kind',OLD.observation_kind,'effective_date',OLD.effective_date,'value_effective_at',OLD.value_effective_at,'source_policy_version',OLD.source_policy_version,'timestamp_basis',OLD.timestamp_basis,'supersedes_quote_id',OLD.supersedes_quote_id)`.
- OLD/NEW effective time expression: `COALESCE(OLD.value_effective_at,OLD.effective_date,OLD.quoted_at)`.

### `fx_preferences`

- Fields: `household_id`, `currency_a`, `currency_b`, `source_kind`, `created_at`, `updated_at`.
- Write owners: `agent_quotes.go`, `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('household_id',OLD.household_id,'currency_a',OLD.currency_a,'currency_b',OLD.currency_b,'source_kind',OLD.source_kind,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `history_origins`

- Fields: `id`, `household_id`, `timezone`, `started_at`, `created_at`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'timezone',OLD.timezone,'started_at',OLD.started_at,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.started_at`.

### `history_origin_components`

- Fields: `id`, `origin_id`, `component_kind`, `account_id`, `holding_id`, `instrument_id`, `amount`, `currency`, `quantity`, `created_at`, `unit_cost`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'origin_id',OLD.origin_id,'account_id',OLD.account_id,'holding_id',OLD.holding_id,'instrument_id',OLD.instrument_id,'currency',OLD.currency,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `(SELECT started_at FROM history_origins WHERE id=OLD.origin_id)`.

### `history_origin_account_states`

- Fields: `origin_id`, `account_id`, `archived_at`, `include_in_net_worth`, `include_in_portfolio`, `include_in_liquid_assets`, `created_at`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('origin_id',OLD.origin_id,'account_id',OLD.account_id,'archived_at',OLD.archived_at,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `(SELECT started_at FROM history_origins WHERE id=OLD.origin_id)`.

### `history_origin_ownership`

- Fields: `origin_id`, `account_id`, `member_id`, `share_bps`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('origin_id',OLD.origin_id,'account_id',OLD.account_id,'member_id',OLD.member_id)`.
- OLD/NEW effective time expression: `(SELECT started_at FROM history_origins WHERE id=OLD.origin_id)`.

### `history_origin_instrument_preferences`

- Fields: `origin_id`, `instrument_id`, `source_kind`, `created_at`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('origin_id',OLD.origin_id,'instrument_id',OLD.instrument_id,'source_kind',OLD.source_kind,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `(SELECT started_at FROM history_origins WHERE id=OLD.origin_id)`.

### `history_origin_fx_preferences`

- Fields: `origin_id`, `currency_a`, `currency_b`, `source_kind`, `created_at`.
- Write owners: `history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('origin_id',OLD.origin_id,'currency_a',OLD.currency_a,'currency_b',OLD.currency_b,'source_kind',OLD.source_kind,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `(SELECT started_at FROM history_origins WHERE id=OLD.origin_id)`.

### `activities`

- Fields: `id`, `household_id`, `kind`, `reason`, `effective_at`, `effective_local_date`, `created_at`, `note`, `reverses_activity_id`, `correction_group_id`, `transaction_fx_rate`.
- Write owners: `change_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'kind',OLD.kind,'effective_at',OLD.effective_at,'effective_local_date',OLD.effective_local_date,'created_at',OLD.created_at,'reverses_activity_id',OLD.reverses_activity_id,'correction_group_id',OLD.correction_group_id)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `activity_effects`

- Fields: `id`, `activity_id`, `sequence`, `role`, `direction`, `target`, `classification`, `account_id`, `holding_id`, `instrument_id`, `amount`, `currency`, `quantity`, `cost_unit_price`.
- Write owners: `change_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'activity_id',OLD.activity_id,'account_id',OLD.account_id,'holding_id',OLD.holding_id,'instrument_id',OLD.instrument_id,'currency',OLD.currency)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM activities WHERE id=OLD.activity_id)`.

### `activity_trade_details`

- Fields: `activity_id`, `side`, `instrument_id`, `holding_id`, `quantity`, `gross_amount`, `gross_currency`, `unit_price`, `fee_amount`, `fee_currency`.
- Write owners: `change_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('activity_id',OLD.activity_id,'instrument_id',OLD.instrument_id,'holding_id',OLD.holding_id)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM activities WHERE id=OLD.activity_id)`.

### `activity_dividend_details`

- Fields: `activity_id`, `holding_id`, `instrument_id`, `amount`, `currency`.
- Write owners: `change_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('activity_id',OLD.activity_id,'holding_id',OLD.holding_id,'instrument_id',OLD.instrument_id,'currency',OLD.currency)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM activities WHERE id=OLD.activity_id)`.

### `activity_correction_groups`

- Fields: `id`, `household_id`, `original_activity_id`, `replacement_activity_id`, `created_at`.
- Write owners: `change_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'original_activity_id',OLD.original_activity_id,'replacement_activity_id',OLD.replacement_activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `holding_quantity_values`

- Fields: `id`, `holding_id`, `quantity`, `effective_at`, `created_at`, `activity_effect_id`, `projection_kind`.
- Write owners: `change_repository.go`, `derived_data_rebuild.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'holding_id',OLD.holding_id,'effective_at',OLD.effective_at,'created_at',OLD.created_at,'activity_effect_id',OLD.activity_effect_id,'projection_kind',OLD.projection_kind)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `account_state_observations`

- Fields: `id`, `account_id`, `effective_at`, `archived_at`, `include_in_net_worth`, `include_in_portfolio`, `include_in_liquid_assets`, `activity_id`, `created_at`.
- Write owners: `observation_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'account_id',OLD.account_id,'effective_at',OLD.effective_at,'archived_at',OLD.archived_at,'activity_id',OLD.activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `account_state_ownership`

- Fields: `observation_id`, `member_id`, `share_bps`.
- Write owners: `observation_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('observation_id',OLD.observation_id,'member_id',OLD.member_id)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM account_state_observations WHERE id=OLD.observation_id)`.

### `instrument_preference_observations`

- Fields: `id`, `instrument_id`, `source_kind`, `effective_at`, `activity_id`, `created_at`.
- Write owners: `observation_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'instrument_id',OLD.instrument_id,'source_kind',OLD.source_kind,'effective_at',OLD.effective_at,'activity_id',OLD.activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `fx_preference_observations`

- Fields: `id`, `household_id`, `currency_a`, `currency_b`, `source_kind`, `effective_at`, `activity_id`, `created_at`.
- Write owners: `observation_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'currency_a',OLD.currency_a,'currency_b',OLD.currency_b,'source_kind',OLD.source_kind,'effective_at',OLD.effective_at,'activity_id',OLD.activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `history_snapshot_state`

- Fields: `household_id`, `dirty_from`, `last_completed_closed_on`, `updated_at`, `dirty_to`, `input_generation`, `resolver_policy_version`.
- Write owners: `change_repository.go`, `derived_data_rebuild.go`, `historical_policy.go`, `history_repository.go`, `market_history_repository.go`, `snapshot_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('household_id',OLD.household_id)`.
- OLD/NEW effective time expression: `NULL`.

### `instrument_state_observations`

- Fields: `id`, `instrument_id`, `effective_at`, `archived_at`, `activity_id`, `created_at`.
- Write owners: `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'instrument_id',OLD.instrument_id,'effective_at',OLD.effective_at,'archived_at',OLD.archived_at,'activity_id',OLD.activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `holding_state_observations`

- Fields: `id`, `holding_id`, `effective_at`, `archived_at`, `activity_id`, `created_at`.
- Write owners: `portfolio_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'holding_id',OLD.holding_id,'effective_at',OLD.effective_at,'archived_at',OLD.archived_at,'activity_id',OLD.activity_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `instrument_provider_bindings`

- Fields: `instrument_id`, `provider_key`, `provider_symbol`, `market`, `currency`, `enabled`, `created_at`, `updated_at`, `binding_revision`, `effective_from`.
- Write owners: `bindings.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('instrument_id',OLD.instrument_id,'provider_key',OLD.provider_key,'provider_symbol',OLD.provider_symbol,'currency',OLD.currency,'created_at',OLD.created_at,'binding_revision',OLD.binding_revision,'effective_from',OLD.effective_from)`.
- OLD/NEW effective time expression: `OLD.effective_from`.

### `instrument_provider_binding_revisions`

- Fields: `instrument_id`, `provider_key`, `binding_revision`, `provider_symbol`, `market`, `currency`, `enabled`, `effective_from`, `created_at`.
- Write owners: `bindings.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('instrument_id',OLD.instrument_id,'provider_key',OLD.provider_key,'binding_revision',OLD.binding_revision,'provider_symbol',OLD.provider_symbol,'currency',OLD.currency,'effective_from',OLD.effective_from,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `OLD.effective_from`.

### `instrument_observation_slots`

- Fields: `instrument_id`, `provider_key`, `binding_revision`, `source_policy_version`, `market_date`, `observation_kind`, `quote_id`, `updated_at`.
- Write owners: `market_history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('instrument_id',OLD.instrument_id,'provider_key',OLD.provider_key,'binding_revision',OLD.binding_revision,'source_policy_version',OLD.source_policy_version,'market_date',OLD.market_date,'observation_kind',OLD.observation_kind,'quote_id',OLD.quote_id)`.
- OLD/NEW effective time expression: `OLD.market_date`.

### `fx_observation_slots`

- Fields: `household_id`, `base_currency`, `quote_currency`, `provider_key`, `source_policy_version`, `market_date`, `observation_kind`, `quote_id`, `updated_at`.
- Write owners: `market_history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('household_id',OLD.household_id,'base_currency',OLD.base_currency,'quote_currency',OLD.quote_currency,'provider_key',OLD.provider_key,'source_policy_version',OLD.source_policy_version,'market_date',OLD.market_date,'observation_kind',OLD.observation_kind,'quote_id',OLD.quote_id)`.
- OLD/NEW effective time expression: `OLD.market_date`.

### `market_data_day_status`

- Fields: `target_type`, `target_id`, `provider_key`, `household_id`, `binding_revision`, `source_policy_version`, `effective_date`, `status`, `reason`, `checked_at`, `next_check_at`, `expires_at`.
- Write owners: `market_history_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('target_type',OLD.target_type,'target_id',OLD.target_id,'provider_key',OLD.provider_key,'household_id',OLD.household_id,'binding_revision',OLD.binding_revision,'source_policy_version',OLD.source_policy_version,'effective_date',OLD.effective_date,'next_check_at',OLD.next_check_at,'expires_at',OLD.expires_at)`.
- OLD/NEW effective time expression: `OLD.effective_date`.

### `product_operations`

- Fields: `id`, `household_id`, `kind`, `payload_sha256`, `request_version`, `request_json`, `result_json`, `effective_at`, `created_at`, `reverses_operation_id`.
- Write owners: `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'kind',OLD.kind,'effective_at',OLD.effective_at,'created_at',OLD.created_at,'reverses_operation_id',OLD.reverses_operation_id)`.
- OLD/NEW effective time expression: `OLD.effective_at`.

### `product_contracts`

- Fields: `id`, `household_id`, `account_id`, `holding_id`, `instrument_id`, `kind`, `name`, `note`, `currency`, `principal`, `start_on`, `maturity_on`, `interest_mode`, `annual_rate`, `maturity_interest`, `interest_paid_through_on`, `renewed_from_id`, `state`, `opened_operation_id`, `closed_operation_id`, `revision`, `created_at`, `updated_at`.
- Write owners: `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'account_id',OLD.account_id,'holding_id',OLD.holding_id,'instrument_id',OLD.instrument_id,'kind',OLD.kind,'currency',OLD.currency,'renewed_from_id',OLD.renewed_from_id,'opened_operation_id',OLD.opened_operation_id,'closed_operation_id',OLD.closed_operation_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `liquidity_policies`

- Fields: `id`, `household_id`, `source_kind`, `account_id`, `holding_id`, `currency`, `access_kind`, `unlock_on`, `settlement_days`, `day_basis`, `receipt_on_override`, `accessible_amount_cap`, `normal_exit_fee`, `early_kind`, `early_settlement_days`, `early_day_basis`, `early_fee`, `early_amount_mode`, `early_gross_amount`, `confirmed_at`, `note`, `revision`, `created_at`, `updated_at`.
- Write owners: `liquidity_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'source_kind',OLD.source_kind,'account_id',OLD.account_id,'holding_id',OLD.holding_id,'currency',OLD.currency,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `liquidity_reservations`

- Fields: `id`, `household_id`, `source_kind`, `account_id`, `holding_id`, `currency`, `label`, `amount`, `revision`, `created_at`, `updated_at`, `released_at`.
- Write owners: `liquidity_repository.go`, `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'source_kind',OLD.source_kind,'account_id',OLD.account_id,'holding_id',OLD.holding_id,'currency',OLD.currency,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `product_operation_products`

- Fields: `operation_id`, `product_id`, `role`.
- Write owners: `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('operation_id',OLD.operation_id,'product_id',OLD.product_id,'role',OLD.role)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM product_operations WHERE id=OLD.operation_id)`.

### `product_operation_activities`

- Fields: `operation_id`, `activity_id`, `sequence`, `purpose`, `product_id`.
- Write owners: `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('operation_id',OLD.operation_id,'activity_id',OLD.activity_id,'product_id',OLD.product_id)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM product_operations WHERE id=OLD.operation_id)`.

### `product_operation_reservations`

- Fields: `operation_id`, `reservation_id`, `previous_released_at`, `resulting_released_at`, `resulting_revision`.
- Write owners: `product_repository.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('operation_id',OLD.operation_id,'reservation_id',OLD.reservation_id)`.
- OLD/NEW effective time expression: `(SELECT effective_at FROM product_operations WHERE id=OLD.operation_id)`.

### `agent_quote_records`

- Fields: `id`, `request_key`, `household_id`, `target_type`, `operation`, `quote_id`, `target_quote_id`, `source_title`, `source_url`, `created_at`.
- Write owners: `agent_quotes.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('id',OLD.id,'household_id',OLD.household_id,'target_type',OLD.target_type,'quote_id',OLD.quote_id,'target_quote_id',OLD.target_quote_id,'created_at',OLD.created_at)`.
- OLD/NEW effective time expression: `NULL`.

### `app_configuration`

- Fields: `key`, `value`.
- Write owners: `configuration.go`, `product_terms_mutation.go`, `reconciliation_recovery.go`.
- OLD/NEW scope expression (substitute NEW independently): `json_object('configuration',1)`.
- OLD/NEW effective time expression: `NULL`.

## Explicit exclusions and external boundaries

| Tables | Reason / independent authority |
| --- | --- |
| `daily_valuation_snapshots`, `daily_valuation_snapshot_items` | Derived persisted valuation outputs; review captures authoritative history, not these rows. |
| `activity_mutation_keys`, `change_batch_mutation_keys`, `agent_quote_batches` | Idempotency/batch receipts; actual activities, quotes and withdrawal records are tracked. Deleting a batch with records is FK-restricted. |
| `reconciliation_sessions`, `reconciliation_accounts`, `reconciliation_items`, `reconciliation_receipts`, `reconciliation_links`, `account_financial_revisions`, `reconciliation_financial_changes` | Reconciliation workflow/output and its separate revision/journal; none read by this financial fingerprint. |
| All fourteen `investment_review_*` v19 tables | Review configuration/handling/projection/receipts. Existing rule/control/state revisions and CAS remain independently authoritative. Never replace them with the financial revision. |
| `financial_source_revision`, `financial_source_changes` | Derived invalidation metadata; direct maintenance reset/prune occurs transactionally and never generates recursive source events. |

Dynamic write owners also covered: `repository.go:setIconReference` (members/institutions/groups/accounts/instruments), `setArchive` (institutions/groups), and `derived_data_rebuild.go:replaceCurrentProjectionsTx` (account/cash/quantity replay deletes). Explicit revision writes are generated by the new trigger builder, not by these application helpers. Existing reconciliation triggers may write their own excluded output tables.

All source INSERT triggers run AFTER INSERT so rejected/ignored attempts and the UPDATE arm of UPSERT cannot publish phantom inserts. UPDATE/DELETE retain BEFORE timing for OLD and parent capture. Insert/update publication guards abort source statements if metadata is ignored. All relevant source tables have INSERT/UPDATE/DELETE trigger coverage; production financial writes currently use explicit DML or UPSERT, not `INSERT OR REPLACE`. The reduced all-column harness exercises every source column/operation and identical-value updates; it does **not** substitute for real financial constraints. Separate real-schema tests cover scalar corrections, concurrent connections, rollback, PK movement and FK cascade, full application publication, latest replay values, withdrawal/removal, and T−1ns/T future activation with an unchanged source revision/closed day. Existing application/domain oracle and lifecycle tests are retained.

Managed connections set `foreign_keys=1`, `recursive_triggers=1`, existing immediate transaction mode and busy timeout. Verification checks recursive triggers, exact trigger bodies and table definitions, metadata/version/epoch, and contiguous retained revision range. General schema verification retains FK/integrity/financial validations. Missing/tampered triggers block opening/writes through the normal verified DB factory. A raw unverified external SQLite connection can deliberately bypass enforcement; C1 does not establish a security boundary. With recursive triggers OFF a REPLACE insertion still produces a full-household invalidation, even if SQLite omits its implicit DELETE callback; managed connections use ON. Tests cover both fallback and normal DML. No narrow dependency consumer may use the missing OLD side as proof.

Full SQLite snapshots preserve revision, epoch, floor and journal consistently. Restore reconciliation opens/verifies the installed live copy, transactionally rotates epoch, advances floor to revision, clears its journal and marks review pending (preserving needs_repair) before declaring the restore verified. A crash before that marker safely repeats rotation. Completed restore/ordinary restart preserves the new epoch; backup/safety artifacts retain the old one. JSON v2 export intentionally excludes this metadata, including credentials/configuration; it is read-only. There is currently **no JSON importer**. The finance-only import storage test initializes a fresh DB with equal financial IDs and a distinct epoch. Any future in-place finance import must reset tracking and invalidate projections before readers resume; C1 does not claim an end-to-end importer test.


## Temporal publication maintenance

## Publication interval and logical time

A preexisting quantity event can activate at T without another financial write. At T−1ns and T the source revision and every rule's closed day can agree while current eligibility and position epochs differ. Rechecking the fingerprint at the old capture cutoff cannot validate that staged position for a later instant.

Capture builds a finite `ReviewTemporalCertificate` in the same transaction as all financial/review inputs: UTC `[EvaluatedAt, NextTransitionAt)` at nanosecond precision, temporal policy/scope, source checkpoint, config/state/rule revisions, and DB session/clock generation. Certificates establish reuse eligibility only together with the authenticated source, projection, provider and connection proofs.

Publication retains the existing immediate write reservation, full old-cutoff recapture/fingerprint, source checkpoint and review revision checks. It also repeats the uncut future-source fence there. The injected clock is sampled **after** those checks and before projection writes. Both captured/current intervals must cover that instant, clock generation must match, and existing per-rule closed-day checks must pass. Publication saves their intersection: its end is the earlier captured/rechecked boundary, never an extension of either proof. The caller's captured certificate remains unchanged. A conflict discards the entire stage and uses the existing three-attempt recapture limit.

The clock-aware publication port is required by the shared service. An adapter lacking that capability is unavailable; it cannot fall back to a time sampled before the recheck. The explicit-time repository method remains for deterministic direct callers/tests.

That final sample is the logical as-of, saved atomically with projections/lifecycle/checkpoint. Later wall time or commit cannot extend validity. Alert pages/detail groups and coverage reads retain old data and return `projectionStatus: stale` outside the interval or after session/source invalidation. Reads never reconcile. Existing page status UI is reused. The certificate digest participates in report/cursor revisions even when daily rows are unchanged.

## Complete temporal dependency audit

Scope is the singleton household, including closed/archived holdings, transfer sources and historical routes. Early invalidation is accepted. Missing/unknown namespace or checkpoint cannot prove reuse. Invalid instants/calendar intervals produce errors rather than infinity.

| Sources/read path | Time predicate and treatment |
| --- | --- |
| `history_origins`, all `history_origin_*` | Current position admits origin through the exclusive `now+1ns` cutoff; future `StartedAt` is fenced. Other baseline membership/ownership/cost/preferences use recorded origin and fixed daily cutoffs. |
| `holdings` | All holdings are captured. Current unknown-creation guard uses `CreatedAt < now+1ns`; every future creation is fenced, including archived holdings. Origin holding membership compares with fixed origin. |
| `activities`, `activity_effects`, `activity_correction_groups` | Scan all raw household activity effective instants without capture cutoff, including transfers, reversals and original/replacement chains. Replacements inherit their chain's original instant; immutable original rows remain in the scan. Earliest future raw instant conservatively bounds activation. Atomic quantity effects drive current epochs. |
| Trade/dividend details and holding cost effects | Complete cost events are read independently, then selected at fixed closed-day cutoffs. They do not activate solely when current time passes T within an unchanged closed-day vector. Quantity-changing parent events are fenced above. |
| Households, members, institutions, groups, accounts, account ownership, instruments; current FX preferences | Current directory/latest projections have no moving-now filter. Historical creation/archive/ownership is selected at fixed replay cutoffs. Rule candidates include all captured holdings; current eligibility comes from quantity replay. Source DML still uses C1/full fingerprint checks. |
| Account values/cash, holding quantity values | Latest portfolio selection follows stored row ordering, not wall time. Explicit zero baselines use fixed origin/creation predicates. Historical amounts/quantities replay at fixed day cutoffs. Current review quantities use origin/economic activities. Existing C1 replay-row classification is unchanged. |
| Account/instrument/holding state observations and state ownership | Complete histories are captured; effective/archive instants are selected at fixed rule-day cutoffs. Passing a future state instant within today cannot change a closed day's scope. |
| Instrument/FX preference observations, provider bindings/revisions | Complete histories are captured. Effective routing uses fixed replay/market cutoffs and historical route closure. Configuration edits remain source/config revisions. Credentials never enter this metadata. |
| Instrument/FX quotes | Latest main already returns **uncut history**: both `listAllInstrumentQuotesQuery` and `listAllFXQuotesQuery` explicitly ignore cutoff. A future quote edit changes the retained full fingerprint. This stage's `quoted_at` scans are additional conservative early fences, not a claim that quote capture activates at current time. |
| Quote effective dates, value-effective/provider/fetched timestamps, basis; slots and market-day coverage/status | Market selection, FX economic-date/age admission and coverage use fixed per-rule day cutoffs/market labels. Status check/expiry instants are captured data; refresh scheduling is separate. No production wall-clock call occurs in this read closure. Future metadata fixtures compare complete financial/coverage/lifecycle output at T−1ns/T. |
| Rules, suppressions, calendars | Each rule and each suppression contributes its own next local date boundary and next timezone offset transition. Origin calendar is also bounded. Household-only midnight is insufficient. Existing config/rule/state/episode checks remain. |
| Products/operations, liquidity policies/reservations, agent records, snapshot state, configuration | C1 retains conservative household source invalidation. These add no independent moving-now eligibility predicate in review. Agent withdrawal exclusions are stored facts. Resolver/evaluator/metric versions remain in the token. No settings key/value/hash or provider credential is copied. |

The roster remains the [C1 all-writer/field contract](#financial-source-tracking-maintenance). Fixed-day exclusions are tested, not inferred from an empty future-event result. A changed read closure requires policy review/versioning before changing certified reuse.

## Exact timestamps, calendars and rollback

The audit found that SQL text comparison of valid RFC3339 values is insufficient: whole-second timestamps, optional fractional digits and UTC offsets lack one lexicographic instant order. Review quantity capture now filters parsed instants exactly, retaining the existing quantity/correction-chain scope and hydrating only admitted activities. Parsed ordering preserves the CTE's original correction-chain tie key. Other activity readers keep their existing behavior. No stored financial timestamp is rounded or rewritten.

Tests cover whole-second UTC, nanosecond UTC and a nonzero UTC offset at T−1ns/T/T+1ns. A zero→positive→zero pair with reversed timestamp text order checks full daily replay as well as current quantity.

Review calendar loads pin immutable `time.Location` objects for the process. Each newly opened DB handle has a fresh session identity; persisted certificates never assume calendar/tzdata equivalence across restart or another handle. Legacy `Local` origins pin an initialized location copy; review schedules retain fixed-IANA validation. Hot OS tzdata changes take effect with a new process/session and full recomputation. Coarse restart invalidation is deliberate; no inferred OS version is used.

All clock observations made by review commands/read models, including preview/handling, update mutex-protected observation state. Actual observed time is stored, never `max(now,last)`. Backwards observation advances generation, even above published as-of; outstanding stages conflict. Restart, missing/incompatible checkpoint, rollback, source epoch/policy change or consumed revision below floor removes reusable evidence and takes the full oracle path. Normal forward day advancement still captures all sources and may use the existing independently checked daily-window planner; it is not a no-op.

Observations strip Go's monotonic clock component before comparison, so a still-increasing monotonic reading cannot hide wall-clock rollback. The stored representation and generation transition have their own race-tested regression.

## Schema21 and recovery

Both supported schemas contain `review_temporal_checkpoints`. Startup verifies
its exact definition and checkpoint structure. Unknown nonempty policy versions
cannot establish reuse and are replaced by full publication. Schema21→22 adds
closure facts without replaying financial history and retains a verified
read-only recovery copy before mutation.

Complete SQLite snapshots preserve certificates consistently. Restore retains diagnostic metadata while rotating the live source epoch before resume, invalidating its source binding. Checkpoint writes are derived/excluded from C1 and cannot feed back into source revision. JSON v2 export excludes them. Algorithm rollback retains schema21/full recomputation; an older binary requires the verified pre-upgrade copy, which lacks later financial writes and must not automatically overwrite live data.

### Source journal retention and recovery

Both supported schemas retain the financial source journal. An initial empty
journal is not a historical completeness proof. Each changed source
row advances its revision and journal atomically with its business transaction.
At multiples of 256 revisions above 4096, retention advances the floor to at
least revision minus 4096 and deletes the covered prefix atomically. It retains
4096 rows at the boundary and at most 4351 between boundaries; verification
examines at most 4352 before rejecting overflow/gaps. Consumers below the floor
must fully recapture. Restore/import boundaries rotate source epoch and
invalidate projections before resuming reads. Never edit user_version or drop
triggers to open a live database with an older binary. An older recovery copy
lacks later writes and requires an explicit recovery decision.
