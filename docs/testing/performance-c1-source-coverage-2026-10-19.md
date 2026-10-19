# C1 financial source tracking coverage (2026-10-19)

Contract: [performance plan §5](../design/performance-optimization-plan.zh-CN.md#5-事务版本和变更日志pr-c1c2c3依赖-a). This is PR-C1 tracking only. No review reader consumes these tables. Full-source fingerprints, capture, rechecks, publication and correctness budgets remain authoritative.

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

## Migration, retention and rollback

C1 allocates schema **20** from verified schema19, with the same flushed, verified, read-only recovery-copy discipline as v18→19. Verified v15–18 continue through existing additive steps. The new migration creates revision0/floor0 per existing household and an empty journal, then marks review pending without enabling it or clearing needs_repair. This is an unknown pre-C1 baseline, **not** a historical change log or no-op certificate. There is no startup financial-history backfill. Startup verifies retained journal continuity (work proportional to retained metadata); C1 makes no startup speed claim.

Each changed row increments revision once, adds ordinal0, and commits/rolls back with its business transaction; revision is not a transaction count. No secondary journal index is added. `PruneFinancialSourceChanges` persists a monotone floor and deletes the covered prefix in the same transaction. `ResetFinancialSourceTracking` rotates epoch and clears metadata atomically at restore/import boundaries. Neither explicit primitive has a UI/MCP endpoint. Automatic retention now runs in a verified journal AFTER INSERT trigger: at each multiple of256 source revisions above4096, atomically advance the floor to at least revision−4096 and delete its covered PK prefix. Retain4096 rows at a prune boundary and at most4351 between boundaries. Verification counts at most4352 rows per household before rejecting a gap/overflow; it does not add an unbounded retained-journal scan. This is a row bound, not a byte or elapsed-time SLA. A consumer below floor must fully recapture; no consumer is enabled here. See the [review-fix/retention report](performance-c1-insert-fix-2026-10-19.md) for justification, transactional failure/restore coverage and real batch costs.

Rollback cannot point an old v19 binary at v20. Stop writers, retain the current file, then restore the verified `original-v19.sqlite` recovery artifact to a new live path using a schema19-capable binary that includes PR112's 28-place FX snapshot fix (main `1ee80acea6659b75ee994d9c675d52eaefdb0e45` or newer compatible v19 build). This restores pre-migration data, so post-migration writes require an explicit recovery decision. Never edit `user_version`, drop triggers to make a live database look old, or treat the benchmark's isolated trigger-disabled clone as supported rollback. No real ledger/backup was accessed for this work.

`app_configuration` can contain provider credentials. Its journal source ID is the fixed string `configuration`, and each scope is only `{"configuration":1}`. No key, value, JSON path, hash, URL or credential is copied. Configuration changes conservatively invalidate the household. F4/F15 secret workflows are unchanged and excluded. No journal is exported in public JSON or diagnostic responses.

C2/C3 remain blocked on shadow token/full-fingerprint agreement at a fixed cutoff, transaction/CAS concurrency proof, complete temporal frontier auditing and clock rollback tests. An unchanged revision/closed-day vector is demonstrably insufficient at a future effective boundary. No temporal certificate, token replacement, no-op shortcut or planner activation is introduced here. D dirty suffix/full lifecycle and later GUI/MCP scope parity retain their independent gates.
