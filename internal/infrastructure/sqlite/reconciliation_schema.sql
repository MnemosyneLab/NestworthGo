CREATE TABLE reconciliation_sessions (
 id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id),
 status TEXT NOT NULL CHECK(status IN ('in_progress','paused','completed')),
 revision INTEGER NOT NULL CHECK(revision >= 1), started_at TEXT NOT NULL, completed_at TEXT,
 note TEXT NOT NULL, partial INTEGER NOT NULL CHECK(partial IN (0,1)), closing_note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_reconciliation_sessions ON reconciliation_sessions(household_id, started_at DESC, id DESC);
CREATE TABLE reconciliation_accounts (
 session_id TEXT NOT NULL REFERENCES reconciliation_sessions(id), account_id TEXT NOT NULL REFERENCES accounts(id),
 evidence TEXT NOT NULL CHECK(json_valid(evidence)), PRIMARY KEY(session_id, account_id)
);
CREATE INDEX idx_reconciliation_accounts ON reconciliation_accounts(account_id, session_id);
CREATE TABLE reconciliation_items (
 session_id TEXT NOT NULL, account_id TEXT NOT NULL, id TEXT NOT NULL, evidence TEXT NOT NULL CHECK(json_valid(evidence)),
 PRIMARY KEY(session_id, account_id, id), FOREIGN KEY(session_id, account_id) REFERENCES reconciliation_accounts(session_id, account_id)
);
CREATE TABLE reconciliation_receipts (
 operation_id TEXT PRIMARY KEY NOT NULL, household_id TEXT NOT NULL REFERENCES households(id),
 payload_hash TEXT NOT NULL, result TEXT NOT NULL CHECK(json_valid(result))
);
CREATE TABLE reconciliation_links (
 session_id TEXT NOT NULL, account_id TEXT NOT NULL, item_id TEXT NOT NULL,
 operation_id TEXT NOT NULL, family TEXT NOT NULL, evidence TEXT NOT NULL CHECK(json_valid(evidence)),
 PRIMARY KEY(session_id, account_id, operation_id),
 FOREIGN KEY(session_id, account_id, item_id) REFERENCES reconciliation_items(session_id, account_id, id)
);
CREATE TABLE account_financial_revisions (
 account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id), revision INTEGER NOT NULL CHECK(revision >= 1)
);
INSERT INTO account_financial_revisions SELECT id, 1 FROM accounts;
CREATE TABLE reconciliation_financial_changes (
 account_id TEXT NOT NULL REFERENCES accounts(id), revision INTEGER NOT NULL,
 family TEXT NOT NULL, fact_id TEXT NOT NULL, recorded_at TEXT NOT NULL, effective_at TEXT NOT NULL,
 evidence TEXT NOT NULL CHECK(json_valid(evidence)), PRIMARY KEY(account_id, revision)
);
CREATE TRIGGER reconciliation_accounts_insert AFTER INSERT ON accounts BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.id,revision,'accounts',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.id)) FROM account_financial_revisions WHERE account_id=NEW.id;
END;
CREATE TRIGGER reconciliation_accounts_update AFTER UPDATE ON accounts WHEN OLD.archived_at IS NOT NEW.archived_at OR OLD.tracking_mode IS NOT NEW.tracking_mode OR OLD.default_currency IS NOT NEW.default_currency BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.id,revision,'accounts',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.id)) FROM account_financial_revisions WHERE account_id=NEW.id;
END;
CREATE TRIGGER reconciliation_account_values_insert AFTER INSERT ON account_values WHEN NEW.projection_kind <> 'replay' BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'account_values',NEW.id,NEW.created_at,NEW.effective_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'amount',NEW.amount,'currency',NEW.currency) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_account_cash_values_insert AFTER INSERT ON account_cash_values WHEN NEW.projection_kind <> 'replay' BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'account_cash_values',NEW.id,NEW.created_at,NEW.effective_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'amount',NEW.amount,'currency',NEW.currency) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_holdings_insert AFTER INSERT ON holdings BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'holdings',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'quantity',NEW.quantity,'holdingId',NEW.id) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_holdings_update AFTER UPDATE ON holdings WHEN OLD.quantity IS NOT NEW.quantity OR OLD.archived_at IS NOT NEW.archived_at OR OLD.instrument_id IS NOT NEW.instrument_id OR OLD.account_id IS NOT NEW.account_id BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'holdings',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'quantity',NEW.quantity,'holdingId',NEW.id) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_activity_effects_insert AFTER INSERT ON activity_effects BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(COALESCE(NEW.account_id, (SELECT account_id FROM holdings WHERE id=NEW.holding_id)), 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT COALESCE(NEW.account_id, (SELECT account_id FROM holdings WHERE id=NEW.holding_id)),revision,'activity_effects',NEW.id,(SELECT created_at FROM activities WHERE id=NEW.activity_id),(SELECT effective_at FROM activities WHERE id=NEW.activity_id),json_object('accountName',(SELECT name FROM accounts WHERE id=COALESCE(NEW.account_id, (SELECT account_id FROM holdings WHERE id=NEW.holding_id))),'kind',(SELECT kind FROM activities WHERE id=NEW.activity_id),'activityId',NEW.activity_id,'amount',NEW.amount,'currency',NEW.currency,'quantity',NEW.quantity,'unitCost',NEW.cost_unit_price) FROM account_financial_revisions WHERE account_id=COALESCE(NEW.account_id, (SELECT account_id FROM holdings WHERE id=NEW.holding_id));
END;
CREATE TRIGGER reconciliation_product_contracts_insert AFTER INSERT ON product_contracts BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'product_contracts',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'productName',NEW.name,'principal',NEW.principal,'state',NEW.state,'maturityOn',NEW.maturity_on) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_product_contracts_update AFTER UPDATE ON product_contracts WHEN OLD.household_id IS NOT NEW.household_id OR OLD.account_id IS NOT NEW.account_id OR OLD.holding_id IS NOT NEW.holding_id OR OLD.instrument_id IS NOT NEW.instrument_id OR OLD.kind IS NOT NEW.kind OR OLD.currency IS NOT NEW.currency OR OLD.principal IS NOT NEW.principal OR OLD.start_on IS NOT NEW.start_on OR OLD.maturity_on IS NOT NEW.maturity_on OR OLD.interest_mode IS NOT NEW.interest_mode OR OLD.annual_rate IS NOT NEW.annual_rate OR OLD.maturity_interest IS NOT NEW.maturity_interest OR OLD.interest_paid_through_on IS NOT NEW.interest_paid_through_on OR OLD.renewed_from_id IS NOT NEW.renewed_from_id OR OLD.state IS NOT NEW.state OR OLD.opened_operation_id IS NOT NEW.opened_operation_id OR OLD.closed_operation_id IS NOT NEW.closed_operation_id BEGIN
 INSERT INTO account_financial_revisions(account_id, revision) VALUES(NEW.account_id, 1)
 ON CONFLICT(account_id) DO UPDATE SET revision=revision+1;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'product_contracts',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'productName',NEW.name,'principal',NEW.principal,'state',NEW.state,'maturityOn',NEW.maturity_on) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_instrument_scope AFTER UPDATE OF archived_at, quote_currency ON instruments
 WHEN OLD.archived_at IS NOT NEW.archived_at OR OLD.quote_currency IS NOT NEW.quote_currency BEGIN
 UPDATE account_financial_revisions SET revision=revision+1 WHERE account_id IN (SELECT account_id FROM holdings WHERE instrument_id=NEW.id);
 INSERT INTO reconciliation_financial_changes SELECT r.account_id,r.revision,'instrument_scope',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',a.name,'instrumentName',NEW.name,'archivedAt',NEW.archived_at,'currency',NEW.quote_currency) FROM account_financial_revisions r JOIN accounts a ON a.id=r.account_id WHERE r.account_id IN (SELECT account_id FROM holdings WHERE instrument_id=NEW.id);
END;
CREATE TRIGGER reconciliation_product_facts AFTER INSERT ON product_operation_products
 WHEN NOT (NEW.role='valued' AND (SELECT kind FROM product_operations WHERE id=NEW.operation_id)='value_observation') BEGIN
 UPDATE account_financial_revisions SET revision=revision+1 WHERE account_id=(SELECT account_id FROM product_contracts WHERE id=NEW.product_id);
 INSERT INTO reconciliation_financial_changes SELECT p.account_id,r.revision,'product_operation',NEW.operation_id,o.created_at,o.effective_at,json_object('accountName',a.name,'productName',p.name,'kind',o.kind,'result',json(o.result_json)) FROM product_contracts p JOIN accounts a ON a.id=p.account_id JOIN account_financial_revisions r ON r.account_id=p.account_id JOIN product_operations o ON o.id=NEW.operation_id WHERE p.id=NEW.product_id;
END;
CREATE TRIGGER reconciliation_product_policy_insert AFTER INSERT ON liquidity_policies WHEN EXISTS(SELECT 1 FROM product_contracts WHERE holding_id=NEW.holding_id) BEGIN
 UPDATE account_financial_revisions SET revision=revision+1 WHERE account_id=NEW.account_id;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'product_policy',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'holdingId',NEW.holding_id,'accessKind',NEW.access_kind,'unlockOn',NEW.unlock_on,'normalExitFee',NEW.normal_exit_fee,'earlyFee',NEW.early_fee) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
CREATE TRIGGER reconciliation_product_policy_update AFTER UPDATE ON liquidity_policies WHEN EXISTS(SELECT 1 FROM product_contracts WHERE holding_id=NEW.holding_id) AND (OLD.source_kind IS NOT NEW.source_kind OR OLD.account_id IS NOT NEW.account_id OR OLD.holding_id IS NOT NEW.holding_id OR OLD.currency IS NOT NEW.currency OR OLD.access_kind IS NOT NEW.access_kind OR OLD.unlock_on IS NOT NEW.unlock_on OR OLD.settlement_days IS NOT NEW.settlement_days OR OLD.day_basis IS NOT NEW.day_basis OR OLD.receipt_on_override IS NOT NEW.receipt_on_override OR OLD.accessible_amount_cap IS NOT NEW.accessible_amount_cap OR OLD.normal_exit_fee IS NOT NEW.normal_exit_fee OR OLD.early_kind IS NOT NEW.early_kind OR OLD.early_settlement_days IS NOT NEW.early_settlement_days OR OLD.early_day_basis IS NOT NEW.early_day_basis OR OLD.early_fee IS NOT NEW.early_fee OR OLD.early_amount_mode IS NOT NEW.early_amount_mode OR OLD.early_gross_amount IS NOT NEW.early_gross_amount) BEGIN
 UPDATE account_financial_revisions SET revision=revision+1 WHERE account_id=NEW.account_id;
 INSERT INTO reconciliation_financial_changes SELECT NEW.account_id,revision,'product_policy',NEW.id,NEW.updated_at,NEW.updated_at,json_object('accountName',(SELECT name FROM accounts WHERE id=NEW.account_id),'holdingId',NEW.holding_id,'accessKind',NEW.access_kind,'unlockOn',NEW.unlock_on,'normalExitFee',NEW.normal_exit_fee,'earlyFee',NEW.early_fee) FROM account_financial_revisions WHERE account_id=NEW.account_id;
END;
PRAGMA user_version = 18;
