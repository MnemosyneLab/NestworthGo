CREATE TABLE investment_review_control (
 household_id TEXT PRIMARY KEY NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
 config_revision INTEGER NOT NULL DEFAULT 1 CHECK(config_revision >= 1),
 input_generation INTEGER NOT NULL DEFAULT 0 CHECK(input_generation >= 0),
 review_state_revision INTEGER NOT NULL DEFAULT 0 CHECK(review_state_revision >= 0),
 projection_status TEXT NOT NULL DEFAULT 'pending' CHECK(projection_status IN ('pending','complete','incomplete','needs_repair')),
 published_on TEXT, published_input_fingerprint TEXT, evaluator_version TEXT NOT NULL DEFAULT 'daily_predicate_v1'
);
CREATE TABLE investment_review_rules (
 id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 120),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), severity TEXT NOT NULL CHECK(severity IN ('notice','warning')),
 evaluator TEXT NOT NULL CHECK(evaluator IN ('daily_predicate_v1','legacy_v1')),
 row_revision INTEGER NOT NULL CHECK(row_revision >= 1), current_condition_version INTEGER NOT NULL CHECK(current_condition_version >= 1),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT
);
CREATE INDEX idx_investment_review_rules_household ON investment_review_rules(household_id, archived_at, id);
CREATE TABLE investment_review_rule_versions (
 rule_id TEXT NOT NULL REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 condition_version INTEGER NOT NULL CHECK(condition_version >= 1), spec_version INTEGER NOT NULL,
 canonical_spec_json TEXT NOT NULL CHECK(json_valid(canonical_spec_json)), semantic_hash TEXT NOT NULL,
 created_at TEXT NOT NULL, PRIMARY KEY(rule_id, condition_version)
);
CREATE TABLE investment_review_rule_targets (
 rule_id TEXT NOT NULL REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 PRIMARY KEY(rule_id, instrument_id)
);
CREATE TABLE investment_review_suppressions (
 id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 rule_id TEXT REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 mode TEXT NOT NULL CHECK(mode IN ('permanent','until_date')),
 until_on TEXT, timezone TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision >= 1), created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 CHECK((mode='permanent' AND until_on IS NULL) OR (mode='until_date' AND until_on IS NOT NULL))
);
CREATE UNIQUE INDEX ux_investment_review_suppressions_all ON investment_review_suppressions(household_id,instrument_id) WHERE rule_id IS NULL;
CREATE UNIQUE INDEX ux_investment_review_suppressions_rule ON investment_review_suppressions(household_id,instrument_id,rule_id) WHERE rule_id IS NOT NULL;
CREATE TABLE investment_review_position_epochs (
 id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 opened_at TEXT, opening_activity_id TEXT REFERENCES activities(id) ON DELETE RESTRICT,
 closed_at TEXT, closing_activity_id TEXT REFERENCES activities(id) ON DELETE RESTRICT,
 start_unknown INTEGER NOT NULL CHECK(start_unknown IN (0,1)), source_fingerprint TEXT NOT NULL,
 CHECK(closed_at IS NULL OR opened_at IS NULL OR closed_at >= opened_at)
);
CREATE INDEX idx_investment_review_epochs_instrument ON investment_review_position_epochs(household_id,instrument_id,opened_at,id);
CREATE TABLE investment_review_daily_inputs (
 household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 timezone_version TEXT NOT NULL, on_date TEXT NOT NULL, input_policy TEXT NOT NULL,
 quantity TEXT, native_currency TEXT, instrument_type TEXT, amount_exact TEXT, cost_exact TEXT,
 evidence_json TEXT NOT NULL CHECK(json_valid(evidence_json)), coverage_json TEXT NOT NULL CHECK(json_valid(coverage_json)),
 input_fingerprint TEXT NOT NULL, computed_at TEXT NOT NULL,
 PRIMARY KEY(household_id,instrument_id,timezone_version,on_date,input_policy)
);
CREATE INDEX idx_investment_review_inputs_instrument_day ON investment_review_daily_inputs(instrument_id,on_date);
CREATE TABLE investment_review_daily_results (
 rule_id TEXT NOT NULL, condition_version INTEGER NOT NULL,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 timezone_version TEXT NOT NULL, on_date TEXT NOT NULL,
 predicate_truth TEXT NOT NULL CHECK(predicate_truth IN ('true','false','unknown')),
 reason TEXT, evidence_json TEXT NOT NULL CHECK(json_valid(evidence_json)), input_fingerprint TEXT NOT NULL,
 evaluated_at TEXT NOT NULL, revised_at TEXT,
 PRIMARY KEY(rule_id,condition_version,instrument_id,timezone_version,on_date),
 FOREIGN KEY(rule_id,condition_version) REFERENCES investment_review_rule_versions(rule_id,condition_version) ON DELETE RESTRICT
);
CREATE INDEX idx_investment_review_results_instrument_day ON investment_review_daily_results(instrument_id,on_date);
CREATE TABLE investment_review_episodes (
 id TEXT PRIMARY KEY NOT NULL, rule_id TEXT NOT NULL REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 position_epoch_id TEXT NOT NULL REFERENCES investment_review_position_epochs(id) ON DELETE RESTRICT,
 opened_condition_version INTEGER NOT NULL, first_hit_on TEXT NOT NULL, triggered_on TEXT NOT NULL,
 first_detected_at TEXT NOT NULL, window_start TEXT NOT NULL, window_end TEXT NOT NULL, latest_closed_on TEXT NOT NULL,
 hit_count INTEGER NOT NULL CHECK(hit_count >= 0), false_count INTEGER NOT NULL CHECK(false_count >= 0),
 unknown_count INTEGER NOT NULL CHECK(unknown_count >= 0), consecutive_count INTEGER NOT NULL CHECK(consecutive_count >= 0),
 evidence_json TEXT NOT NULL CHECK(json_valid(evidence_json)), evidence_fingerprint TEXT NOT NULL,
 resolved_at TEXT, resolved_reason TEXT CHECK(resolved_reason IN ('recovered','position_closed','rule_archived','rule_replaced','evidence_corrected')),
 acknowledged_at TEXT, revision INTEGER NOT NULL CHECK(revision >= 1),
 FOREIGN KEY(rule_id,opened_condition_version) REFERENCES investment_review_rule_versions(rule_id,condition_version) ON DELETE RESTRICT,
 CHECK((resolved_at IS NULL) = (resolved_reason IS NULL))
);
CREATE UNIQUE INDEX ux_investment_review_open_episode ON investment_review_episodes(rule_id,instrument_id,position_epoch_id) WHERE resolved_at IS NULL;
CREATE INDEX idx_investment_review_episodes_recent ON investment_review_episodes(rule_id,triggered_on DESC,id DESC);
CREATE TABLE investment_review_evaluation_state (
 rule_id TEXT NOT NULL REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 position_epoch_id TEXT NOT NULL REFERENCES investment_review_position_epochs(id) ON DELETE RESTRICT,
 latest_closed_on TEXT NOT NULL, policy_state TEXT NOT NULL CHECK(policy_state IN ('not_applicable','healthy','confirming','triggered','unknown')),
 window_start TEXT NOT NULL, window_end TEXT NOT NULL,
 hit_count INTEGER NOT NULL CHECK(hit_count >= 0), false_count INTEGER NOT NULL CHECK(false_count >= 0),
 unknown_count INTEGER NOT NULL CHECK(unknown_count >= 0), consecutive_count INTEGER NOT NULL CHECK(consecutive_count >= 0),
 episode_id TEXT REFERENCES investment_review_episodes(id) ON DELETE RESTRICT,
 dirty_from TEXT, coverage_json TEXT NOT NULL CHECK(json_valid(coverage_json)), publication_revision INTEGER NOT NULL CHECK(publication_revision >= 1),
 PRIMARY KEY(rule_id,instrument_id,position_epoch_id)
);
CREATE TABLE investment_review_episode_events (
 id TEXT PRIMARY KEY NOT NULL, episode_id TEXT NOT NULL REFERENCES investment_review_episodes(id) ON DELETE RESTRICT,
 event_key TEXT NOT NULL UNIQUE, kind TEXT NOT NULL, payload_json TEXT NOT NULL CHECK(json_valid(payload_json)), created_at TEXT NOT NULL
);
CREATE TABLE investment_review_legacy_state (
 household_id TEXT PRIMARY KEY NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 source_key TEXT NOT NULL UNIQUE, raw_json TEXT NOT NULL, raw_sha256 TEXT NOT NULL,
 migration_version INTEGER NOT NULL CHECK(migration_version >= 1),
 status TEXT NOT NULL CHECK(status IN ('compatible','needs_repair')),
 error TEXT, imported_at TEXT NOT NULL
);
CREATE TABLE investment_review_mutation_receipts (
 household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 mutation_id TEXT NOT NULL, request_hash TEXT NOT NULL, result_json TEXT NOT NULL CHECK(json_valid(result_json)),
 created_at TEXT NOT NULL, PRIMARY KEY(household_id,mutation_id)
);
PRAGMA user_version=19;
