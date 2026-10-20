CREATE TABLE review_financial_daily_facts (
 id TEXT PRIMARY KEY NOT NULL,
 household_id TEXT NOT NULL REFERENCES households(id) ON DELETE RESTRICT,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 semantic_key TEXT NOT NULL,
 fact_kind TEXT NOT NULL CHECK(fact_kind IN ('base_v1','metrics_v1')),
 source_version TEXT NOT NULL,
 payload_json TEXT NOT NULL,
 coverage_json TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX idx_review_financial_fact_lookup ON review_financial_daily_facts(household_id,instrument_id,semantic_key,fact_kind);
CREATE TABLE review_result_fact_refs (
 rule_id TEXT NOT NULL REFERENCES investment_review_rules(id) ON DELETE RESTRICT,
 condition_version INTEGER NOT NULL,
 instrument_id TEXT NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
 timezone_version TEXT NOT NULL,
 on_date TEXT NOT NULL,
 fact_id TEXT NOT NULL REFERENCES review_financial_daily_facts(id) ON DELETE RESTRICT,
 input_fingerprint TEXT NOT NULL,
 PRIMARY KEY(rule_id,condition_version,instrument_id,timezone_version,on_date,fact_id,input_fingerprint)
);
PRAGMA user_version=23;
