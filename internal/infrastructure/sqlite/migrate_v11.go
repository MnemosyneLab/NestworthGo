package sqlite

import (
	"context"
	"database/sql"
)

func migrateV11ToV12(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range v12MigrationDDL {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := verifySchema(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

var v12MigrationDDL = []string{
	`CREATE TABLE product_operations (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('open','record_existing','receive_interest','settle','renew','undo','value_observation')),
    payload_sha256 TEXT NOT NULL,
    request_version INTEGER NOT NULL CHECK(request_version = 1),
    request_json TEXT NOT NULL,
    result_json TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    reverses_operation_id TEXT,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(reverses_operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT
)`,
	`CREATE UNIQUE INDEX ux_product_operations_reverses ON product_operations(reverses_operation_id) WHERE reverses_operation_id IS NOT NULL`,
	`CREATE INDEX idx_product_operations_household ON product_operations(household_id, created_at, id)`,
	`CREATE TABLE product_contracts (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    holding_id TEXT NOT NULL,
    instrument_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('term_deposit','locked_product')),
    name TEXT NOT NULL,
    note TEXT,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    principal TEXT NOT NULL,
    start_on TEXT NOT NULL,
    maturity_on TEXT,
    interest_mode TEXT NOT NULL CHECK(interest_mode IN ('none','manual_maturity_amount','simple_act_365','simple_act_360')),
    annual_rate TEXT,
    maturity_interest TEXT,
    interest_paid_through_on TEXT,
    renewed_from_id TEXT,
    state TEXT NOT NULL CHECK(state IN ('open','settled','cancelled')),
    opened_operation_id TEXT NOT NULL,
    closed_operation_id TEXT,
    revision INTEGER NOT NULL CHECK(revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT,
    FOREIGN KEY(instrument_id) REFERENCES instruments(id) ON DELETE RESTRICT,
    FOREIGN KEY(renewed_from_id) REFERENCES product_contracts(id) ON DELETE RESTRICT,
    FOREIGN KEY(opened_operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT,
    FOREIGN KEY(closed_operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT
)`,
	`CREATE UNIQUE INDEX ux_product_contracts_holding ON product_contracts(holding_id)`,
	`CREATE UNIQUE INDEX ux_product_contracts_instrument ON product_contracts(instrument_id)`,
	`CREATE INDEX idx_product_contracts_account ON product_contracts(account_id, state)`,
	`CREATE INDEX idx_product_contracts_household ON product_contracts(household_id, state)`,
	`CREATE TABLE liquidity_policies (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('account_value','account_cash','holding')),
    account_id TEXT NOT NULL,
    holding_id TEXT,
    currency TEXT CHECK(currency IS NULL OR currency GLOB '[A-Z][A-Z][A-Z]'),
    access_kind TEXT NOT NULL CHECK(access_kind IN ('on_request','on_date','unknown','excluded')),
    unlock_on TEXT,
    settlement_days INTEGER CHECK(settlement_days IS NULL OR (settlement_days >= 0 AND settlement_days <= 365)),
    day_basis TEXT CHECK(day_basis IS NULL OR day_basis IN ('calendar','weekdays')),
    receipt_on_override TEXT,
    accessible_amount_cap TEXT,
    normal_exit_fee TEXT,
    early_kind TEXT NOT NULL CHECK(early_kind IN ('not_allowed','allowed','unknown')),
    early_settlement_days INTEGER CHECK(early_settlement_days IS NULL OR (early_settlement_days >= 0 AND early_settlement_days <= 365)),
    early_day_basis TEXT CHECK(early_day_basis IS NULL OR early_day_basis IN ('calendar','weekdays')),
    early_fee TEXT,
    early_amount_mode TEXT CHECK(early_amount_mode IS NULL OR early_amount_mode IN ('current_value','fixed_gross')),
    early_gross_amount TEXT,
    confirmed_at TEXT,
    note TEXT,
    revision INTEGER NOT NULL CHECK(revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK((source_kind = 'account_value' AND holding_id IS NULL AND currency IS NULL) OR (source_kind = 'account_cash' AND holding_id IS NULL AND currency IS NOT NULL) OR (source_kind = 'holding' AND holding_id IS NOT NULL)),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT
)`,
	`CREATE UNIQUE INDEX ux_liquidity_policies_account_value ON liquidity_policies(account_id) WHERE source_kind = 'account_value'`,
	`CREATE UNIQUE INDEX ux_liquidity_policies_account_cash ON liquidity_policies(account_id, currency) WHERE source_kind = 'account_cash'`,
	`CREATE UNIQUE INDEX ux_liquidity_policies_holding ON liquidity_policies(holding_id) WHERE source_kind = 'holding'`,
	`CREATE TABLE liquidity_reservations (
    id TEXT PRIMARY KEY NOT NULL,
    household_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('account_value','account_cash','holding')),
    account_id TEXT NOT NULL,
    holding_id TEXT,
    currency TEXT NOT NULL CHECK(currency GLOB '[A-Z][A-Z][A-Z]'),
    label TEXT NOT NULL,
    amount TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK(revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    released_at TEXT,
    CHECK((source_kind = 'account_value' AND holding_id IS NULL) OR (source_kind = 'account_cash' AND holding_id IS NULL) OR (source_kind = 'holding' AND holding_id IS NOT NULL)),
    FOREIGN KEY(household_id) REFERENCES households(id) ON DELETE RESTRICT,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    FOREIGN KEY(holding_id) REFERENCES holdings(id) ON DELETE RESTRICT
)`,
	`CREATE INDEX idx_liquidity_reservations_source ON liquidity_reservations(source_kind, account_id, holding_id, currency)`,
	`CREATE TABLE product_operation_products (
    operation_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('opened','settled','income','reopened','cancelled','valued')),
    PRIMARY KEY(operation_id, product_id, role),
    FOREIGN KEY(operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT,
    FOREIGN KEY(product_id) REFERENCES product_contracts(id) ON DELETE RESTRICT
)`,
	`CREATE INDEX idx_product_operation_products_product ON product_operation_products(product_id, operation_id)`,
	`CREATE TABLE product_operation_activities (
    operation_id TEXT NOT NULL,
    activity_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('acquisition','existing_position','redemption','interest','reversal')),
    product_id TEXT NOT NULL,
    PRIMARY KEY(operation_id, activity_id),
    FOREIGN KEY(operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT,
    FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE RESTRICT,
    FOREIGN KEY(product_id) REFERENCES product_contracts(id) ON DELETE RESTRICT
)`,
	`CREATE UNIQUE INDEX ux_product_operation_activities_activity ON product_operation_activities(activity_id)`,
	`CREATE TABLE product_operation_reservations (
    operation_id TEXT NOT NULL,
    reservation_id TEXT NOT NULL,
    previous_released_at TEXT,
    resulting_released_at TEXT,
    resulting_revision INTEGER NOT NULL,
    PRIMARY KEY(operation_id, reservation_id),
    FOREIGN KEY(operation_id) REFERENCES product_operations(id) ON DELETE RESTRICT,
    FOREIGN KEY(reservation_id) REFERENCES liquidity_reservations(id) ON DELETE RESTRICT
)`,
	`PRAGMA user_version = 12`,
}
