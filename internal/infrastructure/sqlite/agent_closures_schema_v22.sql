CREATE TABLE agent_instrument_closures (
 id TEXT PRIMARY KEY NOT NULL,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 effective_date TEXT NOT NULL,
 effective_at TEXT NOT NULL,
 timezone TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX idx_agent_instrument_closures_date ON agent_instrument_closures(instrument_id,effective_date);
PRAGMA user_version=22;
