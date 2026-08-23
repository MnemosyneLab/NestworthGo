-- v0.1.3 schema 5: versioned archive intervals and complete snapshot evidence.
-- This migration is forward-only. Existing mutable archive values remain
-- readable; all subsequent archive/unarchive operations append state facts.

ALTER TABLE daily_valuation_snapshot_items
    ADD COLUMN fx_preference_observation_id TEXT REFERENCES fx_preference_observations(id) ON DELETE RESTRICT;

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
