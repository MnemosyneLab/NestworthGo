-- Sanitized v0.1.2 schema-3 fixture for v0.1.3 compatibility tests.
-- All identifiers are deterministic test UUIDs; no production data is present.
PRAGMA foreign_keys = ON;

CREATE TABLE households (
    id TEXT PRIMARY KEY NOT NULL,
    singleton_key INTEGER NOT NULL DEFAULT 1 UNIQUE CHECK(singleton_key = 1),
    name TEXT NOT NULL,
    base_currency TEXT NOT NULL CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE media_assets (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    data BLOB NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
);
CREATE TABLE members (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    name TEXT NOT NULL,
    avatar_asset_id TEXT,
    note TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(avatar_asset_id) REFERENCES media_assets(id) ON DELETE SET NULL
);
CREATE INDEX idx_members_household ON members(household_id);
CREATE TABLE institutions (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    name TEXT NOT NULL,
    institution_type TEXT,
    country_code TEXT,
    website TEXT,
    note TEXT,
    logo_asset_id TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(logo_asset_id) REFERENCES media_assets(id) ON DELETE SET NULL
);
CREATE INDEX idx_institutions_household ON institutions(household_id);
CREATE TABLE account_groups (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    name TEXT NOT NULL,
    icon_key TEXT,
    color TEXT,
    logo_asset_id TEXT,
    description TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(logo_asset_id) REFERENCES media_assets(id) ON DELETE SET NULL
);
CREATE INDEX idx_groups_household ON account_groups(household_id);
CREATE TABLE accounts (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    institution_id TEXT,
    group_id TEXT,
    name TEXT NOT NULL,
    primary_category TEXT NOT NULL CHECK(primary_category IN ('cash_equivalent','investment','property','receivable','liability')),
    secondary_category TEXT NOT NULL,
    tracking_mode TEXT NOT NULL CHECK(tracking_mode IN ('balance','manual_value','holdings')),
    default_currency TEXT NOT NULL CHECK(default_currency GLOB '[A-Z][A-Z][A-Z]'),
    note TEXT,
    logo_asset_id TEXT,
    include_in_net_worth INTEGER NOT NULL DEFAULT 1 CHECK(include_in_net_worth IN (0,1)),
    include_in_investment INTEGER NOT NULL DEFAULT 0 CHECK(include_in_investment IN (0,1)),
    include_in_liquid_assets INTEGER NOT NULL DEFAULT 0 CHECK(include_in_liquid_assets IN (0,1)),
    opened_on TEXT,
    closed_on TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(institution_id) REFERENCES institutions(id) ON DELETE SET NULL,
    FOREIGN KEY(group_id) REFERENCES account_groups(id) ON DELETE SET NULL,
    FOREIGN KEY(logo_asset_id) REFERENCES media_assets(id) ON DELETE SET NULL
);
CREATE INDEX idx_accounts_household ON accounts(household_id);
CREATE INDEX idx_accounts_institution ON accounts(institution_id);
CREATE INDEX idx_accounts_group ON accounts(group_id);
CREATE INDEX idx_accounts_category ON accounts(primary_category);
CREATE TABLE account_ownership (
    account_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    share_bps INTEGER NOT NULL CHECK(share_bps > 0 AND share_bps <= 10000),
    PRIMARY KEY(account_id, member_id),
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE RESTRICT
);
CREATE INDEX idx_ownership_member ON account_ownership(member_id);
CREATE TABLE account_values (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT NOT NULL,
    value_kind TEXT NOT NULL CHECK(value_kind IN ('balance','manual_value')),
    amount TEXT NOT NULL,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    effective_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE INDEX idx_account_values_latest ON account_values(account_id, effective_at DESC, created_at DESC, id DESC);
PRAGMA user_version = 1;

ALTER TABLE institutions ADD COLUMN icon_key TEXT;
ALTER TABLE accounts ADD COLUMN icon_key TEXT;

CREATE TABLE instruments (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    name TEXT NOT NULL,
    instrument_type TEXT NOT NULL CHECK(instrument_type IN ('stock','etf','mutual_fund','crypto','bond','precious_metal','bank_investment_product','other')),
    quote_currency TEXT NOT NULL CHECK(quote_currency GLOB '[A-Z][A-Z][A-Z]'),
    symbol TEXT,
    market_code TEXT,
    country_code TEXT CHECK(country_code IS NULL OR country_code GLOB '[A-Z][A-Z]'),
    isin TEXT,
    note TEXT,
    logo_asset_id TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    quote_source TEXT NOT NULL DEFAULT 'manual' CHECK(quote_source IN ('manual','provider')),
    provider_key TEXT,
    provider_symbol TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    CHECK(quote_source = 'manual' OR (provider_key IS NOT NULL AND provider_symbol IS NOT NULL)),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE,
    FOREIGN KEY(logo_asset_id) REFERENCES media_assets(id) ON DELETE SET NULL
);
CREATE INDEX idx_instruments_household ON instruments(household_id);
CREATE INDEX idx_instruments_quote_source ON instruments(household_id, quote_source, archived_at);
CREATE UNIQUE INDEX ux_instruments_active_provider_binding ON instruments(household_id, provider_key, provider_symbol) WHERE archived_at IS NULL AND provider_key IS NOT NULL AND provider_symbol IS NOT NULL;
CREATE TABLE holdings (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT NOT NULL,
    instrument_id TEXT NOT NULL,
    quantity TEXT NOT NULL,
    note TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT
);
CREATE INDEX idx_holdings_account ON holdings(account_id, archived_at, sort_order, id);
CREATE INDEX idx_holdings_instrument ON holdings(instrument_id, archived_at);
CREATE UNIQUE INDEX ux_holdings_active_account_instrument ON holdings(account_id, instrument_id) WHERE archived_at IS NULL;
CREATE TABLE account_cash_values (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT NOT NULL,
    amount TEXT NOT NULL,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    effective_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE INDEX idx_account_cash_latest ON account_cash_values(account_id, currency, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE instrument_quotes (
    id TEXT PRIMARY KEY NOT NULL,
    instrument_id TEXT NOT NULL,
    unit_price TEXT NOT NULL,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    source_key TEXT NOT NULL,
    quoted_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    delayed INTEGER NOT NULL DEFAULT 0 CHECK(delayed IN (0,1)),
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE CASCADE
);
CREATE INDEX idx_instrument_quotes_latest ON instrument_quotes(instrument_id, source_kind, currency, quoted_at DESC, created_at DESC, id DESC);
CREATE TABLE fx_quotes (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    base_currency TEXT NOT NULL CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]'),
    quote_currency TEXT NOT NULL CHECK(quote_currency GLOB '[A-Z][A-Z][A-Z]'),
    rate TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    source_key TEXT NOT NULL,
    quoted_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    delayed INTEGER NOT NULL DEFAULT 0 CHECK(delayed IN (0,1)),
    CHECK(base_currency <> quote_currency),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
);
CREATE INDEX idx_fx_quotes_latest ON fx_quotes(household_id, base_currency, quote_currency, source_kind, quoted_at DESC, created_at DESC, id DESC);
CREATE TABLE fx_preferences (
    household_id TEXT NOT NULL,
    currency_a TEXT NOT NULL CHECK(currency_a GLOB '[A-Z][A-Z][A-Z]'),
    currency_b TEXT NOT NULL CHECK(currency_b GLOB '[A-Z][A-Z][A-Z]'),
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(household_id, currency_a, currency_b),
    CHECK(currency_a < currency_b),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
);
CREATE INDEX idx_fx_preferences_household ON fx_preferences(household_id, currency_a, currency_b);

INSERT INTO households(id, singleton_key, name, base_currency, created_at, updated_at)
VALUES('00000000-0000-4000-8000-000000000001', 1, 'Sample Household', 'CNY', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO members(id, household_id, name, sort_order, created_at, updated_at, archived_at)
VALUES
('00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', 'Member A', 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000001', 'Member B', 1, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000004', '00000000-0000-4000-8000-000000000001', 'Archived Member', 2, '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z');
INSERT INTO institutions(id, household_id, name, institution_type, country_code, created_at, updated_at)
VALUES('00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001', 'Sample Bank', 'bank', 'SG', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO account_groups(id, household_id, name, sort_order, created_at, updated_at)
VALUES('00000000-0000-4000-8000-000000000011', '00000000-0000-4000-8000-000000000001', 'Sample Group', 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO accounts(id, household_id, institution_id, group_id, name, primary_category, secondary_category, tracking_mode, default_currency, include_in_net_worth, include_in_investment, include_in_liquid_assets, created_at, updated_at, archived_at)
VALUES
('00000000-0000-4000-8000-000000000020', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000011', 'Family Cash', 'cash_equivalent', 'bank_account', 'balance', 'CNY', 1, 0, 1, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000011', 'Portfolio', 'investment', 'brokerage_account', 'holdings', 'USD', 1, 1, 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL),
('00000000-0000-4000-8000-000000000022', '00000000-0000-4000-8000-000000000001', NULL, NULL, 'Archived Account', 'cash_equivalent', 'cash', 'balance', 'CNY', 1, 0, 0, '2026-01-01T00:00:00.000Z', '2026-01-03T00:00:00.000Z', '2026-01-03T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000023', '00000000-0000-4000-8000-000000000001', NULL, NULL, 'Manual Value', 'property', 'real_estate', 'manual_value', 'CNY', 1, 0, 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL);
INSERT INTO account_ownership(account_id, member_id, share_bps)
VALUES
('00000000-0000-4000-8000-000000000020', '00000000-0000-4000-8000-000000000002', 7000),
('00000000-0000-4000-8000-000000000020', '00000000-0000-4000-8000-000000000004', 3000),
('00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000002', 10000),
('00000000-0000-4000-8000-000000000023', '00000000-0000-4000-8000-000000000003', 10000);
INSERT INTO account_values(id, account_id, value_kind, amount, currency, effective_at, created_at)
VALUES
('00000000-0000-4000-8000-000000000030', '00000000-0000-4000-8000-000000000020', 'balance', '0', 'CNY', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000031', '00000000-0000-4000-8000-000000000022', 'balance', '0', 'CNY', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000032', '00000000-0000-4000-8000-000000000023', 'manual_value', '0', 'CNY', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO instruments(id, household_id, name, instrument_type, quote_currency, symbol, country_code, quote_source, provider_key, provider_symbol, created_at, updated_at)
VALUES
('00000000-0000-4000-8000-000000000040', '00000000-0000-4000-8000-000000000001', 'Sample ETF', 'etf', 'USD', 'QQQ', 'US', 'manual', NULL, NULL, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000041', '00000000-0000-4000-8000-000000000001', 'Sample Stock', 'stock', 'SGD', 'ES3', 'SG', 'provider', 'fixture', 'ES3', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000042', '00000000-0000-4000-8000-000000000001', 'Zero Position', 'stock', 'CNY', 'ZERO', 'CN', 'manual', NULL, NULL, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO holdings(id, account_id, instrument_id, quantity, sort_order, created_at, updated_at)
VALUES
('00000000-0000-4000-8000-000000000050', '00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000040', '3', 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000051', '00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000041', '1000', 1, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000052', '00000000-0000-4000-8000-000000000021', '00000000-0000-4000-8000-000000000042', '0', 2, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO account_cash_values(id, account_id, amount, currency, effective_at, created_at)
VALUES
('00000000-0000-4000-8000-000000000060', '00000000-0000-4000-8000-000000000021', '5000', 'SGD', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000061', '00000000-0000-4000-8000-000000000021', '0', 'USD', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed)
VALUES
('00000000-0000-4000-8000-000000000070', '00000000-0000-4000-8000-000000000040', '700', 'USD', 'manual', 'manual', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 0),
('00000000-0000-4000-8000-000000000071', '00000000-0000-4000-8000-000000000040', '701', 'USD', 'provider', 'fixture', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 0),
('00000000-0000-4000-8000-000000000072', '00000000-0000-4000-8000-000000000041', '4', 'SGD', 'provider', 'fixture', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 0);
INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed)
VALUES
('00000000-0000-4000-8000-000000000080', '00000000-0000-4000-8000-000000000001', 'SGD', 'CNY', '5.3', 'manual', 'manual', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 0),
('00000000-0000-4000-8000-000000000081', '00000000-0000-4000-8000-000000000001', 'USD', 'CNY', '6.9', 'manual', 'manual', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 0);
INSERT INTO fx_preferences(household_id, currency_a, currency_b, source_kind, created_at, updated_at)
VALUES
('00000000-0000-4000-8000-000000000001', 'CNY', 'SGD', 'manual', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
('00000000-0000-4000-8000-000000000001', 'CNY', 'USD', 'manual', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');

PRAGMA user_version = 3;
