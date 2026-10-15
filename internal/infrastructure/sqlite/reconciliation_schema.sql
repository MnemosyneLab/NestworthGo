CREATE TABLE reconciliation_sessions (
 id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id),
 status TEXT NOT NULL CHECK(status IN ('in_progress','paused','completed')),
 revision INTEGER NOT NULL CHECK(revision >= 1), started_at TEXT NOT NULL, completed_at TEXT,
 note TEXT NOT NULL, partial INTEGER NOT NULL CHECK(partial IN (0,1))
);
CREATE INDEX idx_reconciliation_sessions ON reconciliation_sessions(household_id, started_at DESC, id DESC);
CREATE TABLE reconciliation_accounts (
 session_id TEXT NOT NULL REFERENCES reconciliation_sessions(id), account_id TEXT NOT NULL REFERENCES accounts(id),
 evidence TEXT NOT NULL, PRIMARY KEY(session_id, account_id)
);
CREATE INDEX idx_reconciliation_accounts ON reconciliation_accounts(account_id, session_id);
CREATE TABLE reconciliation_items (
 session_id TEXT NOT NULL, account_id TEXT NOT NULL, id TEXT NOT NULL, evidence TEXT NOT NULL,
 PRIMARY KEY(session_id, account_id, id), FOREIGN KEY(session_id, account_id) REFERENCES reconciliation_accounts(session_id, account_id)
);
CREATE TABLE reconciliation_receipts (
 operation_id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id),
 payload_hash TEXT NOT NULL, result TEXT NOT NULL
);
CREATE TABLE reconciliation_links (
 session_id TEXT NOT NULL, account_id TEXT NOT NULL, item_id TEXT NOT NULL,
 operation_id TEXT NOT NULL, family TEXT NOT NULL, evidence TEXT NOT NULL,
 PRIMARY KEY(session_id, account_id, operation_id),
 FOREIGN KEY(session_id, account_id, item_id) REFERENCES reconciliation_items(session_id, account_id, id)
);
CREATE TABLE account_financial_revisions (
 account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id), revision INTEGER NOT NULL CHECK(revision >= 1)
);
INSERT INTO account_financial_revisions SELECT id, 1 FROM accounts;
CREATE TRIGGER reconciliation_accounts_insert AFTER INSERT ON accounts BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_accounts_update AFTER UPDATE ON accounts WHEN OLD.archived_at IS NOT NEW.archived_at OR OLD.tracking_mode IS NOT NEW.tracking_mode OR OLD.default_currency IS NOT NEW.default_currency BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_account_values_insert AFTER INSERT ON account_values BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_account_cash_values_insert AFTER INSERT ON account_cash_values BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_holdings_insert AFTER INSERT ON holdings BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_holdings_update AFTER UPDATE ON holdings WHEN OLD.quantity IS NOT NEW.quantity OR OLD.archived_at IS NOT NEW.archived_at OR OLD.instrument_id IS NOT NEW.instrument_id OR OLD.account_id IS NOT NEW.account_id BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_activity_effects_insert AFTER INSERT ON activity_effects BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(COALESCE(NEW.account_id, (SELECT account_id FROM holdings WHERE id=NEW.holding_id)), 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_product_contracts_insert AFTER INSERT ON product_contracts BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_product_contracts_update AFTER UPDATE ON product_contracts BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER reconciliation_instrument_scope AFTER UPDATE OF archived_at, quote_currency ON instruments
 WHEN OLD.archived_at IS NOT NEW.archived_at OR OLD.quote_currency IS NOT NEW.quote_currency BEGIN
 UPDATE account_financial_revisions SET revision=revision+1 WHERE account_id IN (SELECT account_id FROM holdings WHERE instrument_id=NEW.id);
END;
PRAGMA user_version = 17;
