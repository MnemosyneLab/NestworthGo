-- v0.1.3 schema 4: append-only history facts and replaceable snapshot cache.
-- This migration creates structure only. It deliberately inserts no business
-- rows, Activities, or Starting point.
ALTER TABLE account_values ADD COLUMN activity_effect_id TEXT;
ALTER TABLE account_values ADD COLUMN projection_kind TEXT NOT NULL DEFAULT 'legacy' CHECK(projection_kind IN ('legacy','event','replay'));
ALTER TABLE account_cash_values ADD COLUMN activity_effect_id TEXT;
ALTER TABLE account_cash_values ADD COLUMN projection_kind TEXT NOT NULL DEFAULT 'legacy' CHECK(projection_kind IN ('legacy','event','replay'));

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
    created_at TEXT NOT NULL,
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
    kind TEXT NOT NULL CHECK(kind IN ('cash_in','cash_out','cash_transfer','position_transfer','buy','sell','value_update','debt_draw','debt_payment','reversal')),
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
    quantity TEXT,
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
    projection_kind TEXT NOT NULL DEFAULT 'legacy' CHECK(projection_kind IN ('legacy','event','replay')),
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
    missing_reason TEXT,
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
