CREATE TABLE financial_source_revision (
 household_id TEXT PRIMARY KEY NOT NULL REFERENCES households(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL DEFAULT 0 CHECK(typeof(revision)='integer' AND revision>=0),
 journal_floor INTEGER NOT NULL DEFAULT 0 CHECK(journal_floor>=0 AND journal_floor<=revision),
 tracking_version TEXT NOT NULL,
 tracking_epoch TEXT NOT NULL CHECK(length(tracking_epoch)=32 AND tracking_epoch NOT GLOB '*[^0-9a-f]*')
);
CREATE TABLE financial_source_changes (
 household_id TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL CHECK(revision>0),
 change_ordinal INTEGER NOT NULL CHECK(change_ordinal=0),
 source_table TEXT NOT NULL,
 source_id TEXT NOT NULL,
 operation TEXT NOT NULL CHECK(operation IN ('insert','update','delete')),
 old_scope_json TEXT CHECK(old_scope_json IS NULL OR (json_valid(old_scope_json) AND json_type(old_scope_json)='object')),
 new_scope_json TEXT CHECK(new_scope_json IS NULL OR (json_valid(new_scope_json) AND json_type(new_scope_json)='object')),
 old_effective_at TEXT,
 new_effective_at TEXT,
 dependency_kind TEXT NOT NULL CHECK(dependency_kind='household_all'),
 PRIMARY KEY(household_id,revision,change_ordinal)
);
