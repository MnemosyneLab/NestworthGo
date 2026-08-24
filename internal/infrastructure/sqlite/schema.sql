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
    icon_key TEXT,
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
    icon_key TEXT,
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
    created_at TEXT NOT NULL, activity_effect_id TEXT, projection_kind TEXT NOT NULL DEFAULT 'baseline' CHECK(projection_kind IN ('baseline','event','replay')),
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE INDEX idx_account_values_latest ON account_values(account_id, effective_at DESC, created_at DESC, id DESC);
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
    created_at TEXT NOT NULL, activity_effect_id TEXT, projection_kind TEXT NOT NULL DEFAULT 'baseline' CHECK(projection_kind IN ('baseline','event','replay')),
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
CREATE TABLE history_origins (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL UNIQUE,
    timezone TEXT NOT NULL CHECK(length(trim(timezone)) > 0),
    started_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE CASCADE
);
CREATE INDEX idx_history_origins_household ON history_origins(household_id);
CREATE TABLE history_origin_components (
    id TEXT PRIMARY KEY NOT NULL,
    origin_id TEXT NOT NULL,
    component_kind TEXT NOT NULL CHECK(component_kind IN ('account_value','account_cash','holding_quantity')),
    account_id TEXT,
    holding_id TEXT,
    instrument_id TEXT,
    amount TEXT,
    currency TEXT,
    quantity TEXT,
    created_at TEXT NOT NULL, unit_cost TEXT,
    FOREIGN KEY(origin_id) REFERENCES history_origins(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    CHECK((component_kind = 'account_value' AND account_id IS NOT NULL AND amount IS NOT NULL AND currency IS NOT NULL AND holding_id IS NULL AND instrument_id IS NULL AND quantity IS NULL) OR
          (component_kind = 'account_cash' AND account_id IS NOT NULL AND amount IS NOT NULL AND currency IS NOT NULL AND holding_id IS NULL AND instrument_id IS NULL AND quantity IS NULL) OR
          (component_kind = 'holding_quantity' AND account_id IS NOT NULL AND holding_id IS NOT NULL AND instrument_id IS NOT NULL AND quantity IS NOT NULL AND amount IS NULL AND currency IS NULL))
);
CREATE INDEX idx_history_origin_components_origin ON history_origin_components(origin_id, component_kind, id);
CREATE INDEX idx_history_origin_components_account ON history_origin_components(account_id, component_kind, id);
CREATE TABLE history_origin_account_states (
    origin_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    archived_at TEXT,
    include_in_net_worth INTEGER NOT NULL CHECK(include_in_net_worth IN (0,1)),
    include_in_investment INTEGER NOT NULL CHECK(include_in_investment IN (0,1)),
    include_in_liquid_assets INTEGER NOT NULL CHECK(include_in_liquid_assets IN (0,1)),
    created_at TEXT NOT NULL,
    PRIMARY KEY(origin_id, account_id),
    FOREIGN KEY(origin_id) REFERENCES history_origins(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT
);
CREATE TABLE history_origin_ownership (
    origin_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    share_bps INTEGER NOT NULL CHECK(share_bps > 0 AND share_bps <= 10000),
    PRIMARY KEY(origin_id, account_id, member_id),
    FOREIGN KEY(origin_id) REFERENCES history_origins(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE RESTRICT
);
CREATE TABLE history_origin_instrument_preferences (
    origin_id TEXT NOT NULL,
    instrument_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    created_at TEXT NOT NULL,
    PRIMARY KEY(origin_id, instrument_id),
    FOREIGN KEY(origin_id) REFERENCES history_origins(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT
);
CREATE TABLE history_origin_fx_preferences (
    origin_id TEXT NOT NULL,
    currency_a TEXT NOT NULL CHECK(currency_a GLOB '[A-Z][A-Z][A-Z]'),
    currency_b TEXT NOT NULL CHECK(currency_b GLOB '[A-Z][A-Z][A-Z]'),
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    created_at TEXT NOT NULL,
    PRIMARY KEY(origin_id, currency_a, currency_b),
    FOREIGN KEY(origin_id) REFERENCES history_origins(id) ON DELETE RESTRICT
);
CREATE TABLE activities (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('cash_in','cash_out','cash_transfer','fx_conversion','position_transfer','buy','sell','value_update','debt_draw','debt_payment','reversal')),
    reason TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    effective_local_date TEXT NOT NULL,
    created_at TEXT NOT NULL,
    note TEXT,
    reverses_activity_id TEXT UNIQUE,
    correction_group_id TEXT,
    transaction_fx_rate TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(reverses_activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE INDEX idx_activities_timeline ON activities(household_id, effective_at DESC, created_at DESC, id DESC);
CREATE INDEX idx_activities_local_date ON activities(household_id, effective_local_date, id);
CREATE TABLE activity_effects (
    id TEXT PRIMARY KEY NOT NULL,
    activity_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK(sequence > 0),
    role TEXT NOT NULL,
    direction TEXT NOT NULL CHECK(direction IN ('added','removed')),
    target TEXT NOT NULL CHECK(target IN ('account_value','account_cash','holding_quantity')),
    classification TEXT NOT NULL,
    account_id TEXT,
    holding_id TEXT,
    instrument_id TEXT,
    amount TEXT,
    currency TEXT,
    quantity TEXT, cost_unit_price TEXT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    UNIQUE(activity_id, sequence),
    CHECK((target IN ('account_value','account_cash') AND account_id IS NOT NULL AND amount IS NOT NULL AND currency IS NOT NULL AND quantity IS NULL) OR
          (target = 'holding_quantity' AND holding_id IS NOT NULL AND instrument_id IS NOT NULL AND quantity IS NOT NULL AND amount IS NULL AND currency IS NULL))
);
CREATE INDEX idx_activity_effects_activity ON activity_effects(activity_id, sequence);
CREATE INDEX idx_activity_effects_account ON activity_effects(account_id, activity_id, sequence);
CREATE INDEX idx_activity_effects_holding ON activity_effects(holding_id, activity_id, sequence);
CREATE TABLE activity_trade_details (
    activity_id TEXT PRIMARY KEY NOT NULL,
    side TEXT NOT NULL CHECK(side IN ('buy','sell')),
    instrument_id TEXT NOT NULL,
    holding_id TEXT NOT NULL,
    quantity TEXT NOT NULL,
    gross_amount TEXT NOT NULL,
    gross_currency TEXT NOT NULL CHECK(gross_currency GLOB '[A-Z][A-Z][A-Z]'),
    unit_price TEXT NOT NULL,
    fee_amount TEXT,
    fee_currency TEXT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT
);
CREATE TABLE activity_correction_groups (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    original_activity_id TEXT NOT NULL UNIQUE,
    replacement_activity_id TEXT UNIQUE,
    created_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(original_activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    FOREIGN KEY(replacement_activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE TABLE holding_quantity_values (
    id TEXT PRIMARY KEY NOT NULL,
    holding_id TEXT NOT NULL,
    quantity TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    activity_effect_id TEXT,
    projection_kind TEXT NOT NULL DEFAULT 'baseline' CHECK(projection_kind IN ('baseline','event','replay')),
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_effect_id) REFERENCES activity_effects(id) ON DELETE RESTRICT
);
CREATE INDEX idx_holding_quantity_latest ON holding_quantity_values(holding_id, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE account_state_observations (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    archived_at TEXT,
    include_in_net_worth INTEGER NOT NULL CHECK(include_in_net_worth IN (0,1)),
    include_in_investment INTEGER NOT NULL CHECK(include_in_investment IN (0,1)),
    include_in_liquid_assets INTEGER NOT NULL CHECK(include_in_liquid_assets IN (0,1)),
    activity_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE INDEX idx_account_state_observations_effective ON account_state_observations(account_id, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE account_state_ownership (
    observation_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    share_bps INTEGER NOT NULL CHECK(share_bps > 0 AND share_bps <= 10000),
    PRIMARY KEY(observation_id, member_id),
    FOREIGN KEY(observation_id) REFERENCES account_state_observations(id) ON DELETE RESTRICT,
    FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE RESTRICT
);
CREATE TABLE instrument_preference_observations (
    id TEXT PRIMARY KEY NOT NULL,
    instrument_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    effective_at TEXT NOT NULL,
    activity_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE INDEX idx_instrument_preference_observations_effective ON instrument_preference_observations(instrument_id, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE fx_preference_observations (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    currency_a TEXT NOT NULL CHECK(currency_a GLOB '[A-Z][A-Z][A-Z]'),
    currency_b TEXT NOT NULL CHECK(currency_b GLOB '[A-Z][A-Z][A-Z]'),
    source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','provider')),
    effective_at TEXT NOT NULL,
    activity_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    CHECK(currency_a < currency_b)
);
CREATE INDEX idx_fx_preference_observations_effective ON fx_preference_observations(household_id, currency_a, currency_b, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE daily_valuation_snapshots (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    local_date TEXT NOT NULL,
    cutoff_at TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK(revision > 0),
    supersedes_id TEXT,
    content_hash TEXT NOT NULL,
    assets_amount TEXT,
    liabilities_amount TEXT,
    net_worth_amount TEXT,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    complete INTEGER NOT NULL CHECK(complete IN (0,1)),
    component_count INTEGER NOT NULL CHECK(component_count >= 0),
    missing_count INTEGER NOT NULL CHECK(missing_count >= 0),
    generation_reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(supersedes_id) REFERENCES daily_valuation_snapshots(id) ON DELETE RESTRICT,
    UNIQUE(household_id, local_date, revision)
);
CREATE INDEX idx_daily_valuation_latest ON daily_valuation_snapshots(household_id, local_date, revision DESC, created_at DESC);
CREATE TABLE daily_valuation_snapshot_items (
    id TEXT PRIMARY KEY NOT NULL,
    snapshot_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    holding_id TEXT,
    instrument_id TEXT,
    native_amount TEXT,
    native_currency TEXT,
    base_amount TEXT,
    base_currency TEXT NOT NULL CHECK(base_currency GLOB '[A-Z][A-Z][A-Z]'),
    quote_id TEXT,
    fx_quote_id TEXT,
    state_observation_id TEXT,
    preference_observation_id TEXT,
    complete INTEGER NOT NULL CHECK(complete IN (0,1)),
    missing_reason TEXT, fx_preference_observation_id TEXT REFERENCES fx_preference_observations(id) ON DELETE RESTRICT,
    FOREIGN KEY(snapshot_id) REFERENCES daily_valuation_snapshots(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT
);
CREATE INDEX idx_daily_valuation_items_snapshot ON daily_valuation_snapshot_items(snapshot_id, account_id, id);
CREATE TABLE history_snapshot_state (
    household_id TEXT PRIMARY KEY NOT NULL,
    dirty_from TEXT,
    last_completed_closed_on TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT
);
CREATE TABLE instrument_state_observations (
    id TEXT PRIMARY KEY NOT NULL,
    instrument_id TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    archived_at TEXT,
    activity_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE INDEX idx_instrument_state_observations_effective ON instrument_state_observations(instrument_id, effective_at DESC, created_at DESC, id DESC);
CREATE TABLE holding_state_observations (
    id TEXT PRIMARY KEY NOT NULL,
    holding_id TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    archived_at TEXT,
    activity_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT
);
CREATE INDEX idx_holding_state_observations_effective ON holding_state_observations(holding_id, effective_at DESC, created_at DESC, id DESC);
CREATE INDEX idx_daily_valuation_items_fx_preference ON daily_valuation_snapshot_items(fx_preference_observation_id);
PRAGMA user_version = 6;
