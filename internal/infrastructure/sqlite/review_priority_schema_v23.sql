CREATE TABLE investment_review_rule_priorities(
 rule_id TEXT PRIMARY KEY REFERENCES investment_review_rules(id) ON DELETE CASCADE,
 priority TEXT NOT NULL CHECK(priority IN ('p0','p1','p2'))
);
PRAGMA user_version=23;
