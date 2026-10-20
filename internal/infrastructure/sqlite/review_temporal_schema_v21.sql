CREATE TABLE review_temporal_checkpoints (
 household_id TEXT PRIMARY KEY NOT NULL REFERENCES households(id) ON DELETE CASCADE,
 evaluated_at TEXT NOT NULL,
 next_transition_at TEXT NOT NULL,
 certificate_json TEXT NOT NULL CHECK(json_valid(certificate_json))
);
UPDATE investment_review_control SET projection_status='pending' WHERE projection_status='complete';
PRAGMA user_version=21;
