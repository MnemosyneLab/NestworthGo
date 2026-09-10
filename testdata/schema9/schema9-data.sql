-- Compact schema-9 business rows for 9→10 migration tests.
-- Covers manual quotes, unverifiable provider quotes, mixed FX, archived
-- provider bindings, history origin, dirty state, and one legacy snapshot.
PRAGMA foreign_keys = ON;

INSERT INTO households(id, singleton_key, name, base_currency, created_at, updated_at)
VALUES('00000000-0000-4000-8000-000000000001', 1, 'Schema9 Household', 'SGD', '2026-09-01T00:00:00.000Z', '2026-09-01T00:00:00.000Z');

INSERT INTO members(id, household_id, name, icon_key, note, sort_order, created_at, updated_at, archived_at)
VALUES('00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', 'Owner', 'person', NULL, 0, '2026-09-01T00:00:00.000Z', '2026-09-01T00:00:00.000Z', NULL);

INSERT INTO instruments(id, household_id, name, instrument_type, quote_currency, symbol, market_code, country_code, isin, note, icon_key, sort_order, quote_source, provider_key, provider_symbol, created_at, updated_at, archived_at)
VALUES
('00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001', 'Manual Fund', 'mutual_fund', 'SGD', 'MANUAL', NULL, 'SG', NULL, NULL, 'chart', 0, 'manual', NULL, NULL, '2026-09-01T00:00:00.000Z', '2026-09-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000011', '00000000-0000-4000-8000-000000000001', 'Apple', 'stock', 'USD', 'AAPL', 'XNAS', 'US', NULL, NULL, 'chart', 1, 'provider', 'yahoo_finance', 'AAPL', '2026-09-01T00:00:00.000Z', '2026-09-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000012', '00000000-0000-4000-8000-000000000001', 'Archived ETF', 'etf', 'USD', 'OLD', 'XNAS', 'US', NULL, NULL, 'chart', 2, 'provider', 'yahoo_finance', 'OLD', '2026-09-01T00:00:00.000Z', '2026-09-02T00:00:00.000Z', '2026-09-02T00:00:00.000Z');

INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed)
VALUES
('00000000-0000-4000-8000-000000000020', '00000000-0000-4000-8000-000000000010', '1.25', 'SGD', 'manual', 'manual', '2026-09-04T08:00:00.000Z', '2026-09-04T08:00:00.000Z', 0),
('00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000011', '185.25', 'USD', 'provider', 'yahoo_finance', '2026-09-04T20:00:00.000Z', '2026-09-04T20:05:00.000Z', 1),
('00000000-0000-4000-8000-000000000022', '00000000-0000-4000-8000-000000000012', '40', 'USD', 'provider', 'yahoo_finance', '2026-09-01T20:00:00.000Z', '2026-09-01T20:05:00.000Z', 0);

INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed)
VALUES
('00000000-0000-4000-8000-000000000030', '00000000-0000-4000-8000-000000000001', 'USD', 'SGD', '1.35', 'manual', 'manual', '2026-09-04T16:00:00.000Z', '2026-09-04T16:00:00.000Z', 0),
('00000000-0000-4000-8000-000000000031', '00000000-0000-4000-8000-000000000001', 'EUR', 'SGD', '1.50', 'provider', 'frankfurter', '2026-09-04T16:00:00.000Z', '2026-09-04T16:00:00.000Z', 0);

INSERT INTO history_origins(id, household_id, timezone, started_at, created_at)
VALUES('00000000-0000-4000-8000-000000000040', '00000000-0000-4000-8000-000000000001', 'Asia/Singapore', '2026-09-01T00:00:00.000+08:00', '2026-09-01T00:00:00.000Z');

INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at)
VALUES('00000000-0000-4000-8000-000000000001', NULL, '2026-09-04', '2026-09-05T00:00:00.000Z');

INSERT INTO daily_valuation_snapshots(id, household_id, local_date, cutoff_at, revision, supersedes_id, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at)
VALUES('00000000-0000-4000-8000-000000000050', '00000000-0000-4000-8000-000000000001', '2026-09-04', '2026-09-04T15:59:59.999+08:00', 1, NULL, 'legacy-snapshot-v9', '1000', '0', '1000', 'SGD', 1, 0, 0, 'legacy', '2026-09-05T00:00:00.000Z');
