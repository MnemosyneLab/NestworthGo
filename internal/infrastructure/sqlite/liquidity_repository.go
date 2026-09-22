package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) ReadLiquiditySnapshot(ctx context.Context) (domain.LiquiditySnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	defer tx.Rollback()
	portfolio, err := readPortfolioSnapshotQuery(ctx, tx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	snapshot := domain.LiquiditySnapshot{Portfolio: portfolio}
	if portfolio.Household == nil {
		if err := tx.Commit(); err != nil {
			return domain.LiquiditySnapshot{}, err
		}
		return snapshot, nil
	}
	policies, err := listLiquidityPoliciesQuery(ctx, tx, portfolio.Household.ID)
	if err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	reservations, err := listLiquidityReservationsQuery(ctx, tx, portfolio.Household.ID, true)
	if err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	contracts, err := listProductContractsQuery(ctx, tx, portfolio.Household.ID)
	if err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	snapshot.Policies = policies
	snapshot.Reservations = reservations
	snapshot.Contracts = contracts
	if err := tx.Commit(); err != nil {
		return domain.LiquiditySnapshot{}, err
	}
	return snapshot, nil
}

func (r *Repository) SaveLiquidityPolicy(ctx context.Context, policy domain.LiquidityPolicy) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return upsertLiquidityPolicyTx(ctx, tx, policy)
	})
}

func (r *Repository) DeleteLiquidityPolicy(ctx context.Context, id domain.LiquidityPolicyID, expectedRevision int) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM liquidity_policies WHERE id = ? AND revision = ?`, id.String(), expectedRevision)
		if err != nil {
			return err
		}
		return requireAffected(result, "policy")
	})
}

func (r *Repository) SaveLiquidityReservation(ctx context.Context, reservation domain.LiquidityReservation) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO liquidity_reservations(id, household_id, source_kind, account_id, holding_id, currency, label, amount, revision, created_at, updated_at, released_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET label = excluded.label, amount = excluded.amount, revision = excluded.revision, updated_at = excluded.updated_at, released_at = excluded.released_at`,
			reservation.ID.String(), reservation.HouseholdID.String(), string(reservation.Source.Kind), reservation.Source.AccountID.String(), nullableID(reservation.Source.HoldingID), reservation.Currency.String(), reservation.Label, reservation.Amount.CanonicalAmount(), reservation.Revision, formatTimestamp(reservation.CreatedAt), formatTimestamp(reservation.UpdatedAt), nullableTime(reservation.ReleasedAt))
		return err
	})
}

func (r *Repository) ListLiquidityPolicies(ctx context.Context, householdID domain.HouseholdID) ([]domain.LiquidityPolicy, error) {
	return listLiquidityPoliciesQuery(ctx, r.database.SQL, householdID)
}

func (r *Repository) ListLiquidityReservations(ctx context.Context, householdID domain.HouseholdID, includeReleased bool) ([]domain.LiquidityReservation, error) {
	return listLiquidityReservationsQuery(ctx, r.database.SQL, householdID, includeReleased)
}

func listProductContractsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.ProductContract, error) {
	rows, err := query.QueryContext(ctx, productContractSelect+` WHERE household_id = ? ORDER BY created_at, id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ProductContract{}
	for rows.Next() {
		contract, err := scanProductContract(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, contract)
	}
	return result, rows.Err()
}

func listLiquidityPoliciesQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.LiquidityPolicy, error) {
	rows, err := query.QueryContext(ctx, `SELECT p.id, p.household_id, p.source_kind, p.account_id, p.holding_id, p.currency, p.access_kind, p.unlock_on, p.settlement_days, p.day_basis, p.receipt_on_override, p.accessible_amount_cap, p.normal_exit_fee, p.early_kind, p.early_settlement_days, p.early_day_basis, p.early_fee, p.early_amount_mode, p.early_gross_amount, p.confirmed_at, p.note, p.revision, p.created_at, p.updated_at, CASE p.source_kind WHEN 'account_cash' THEN p.currency WHEN 'account_value' THEN a.default_currency WHEN 'holding' THEN i.quote_currency END FROM liquidity_policies p JOIN accounts a ON a.id = p.account_id LEFT JOIN holdings h ON h.id = p.holding_id LEFT JOIN instruments i ON i.id = h.instrument_id WHERE p.household_id = ? ORDER BY p.created_at, p.id`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.LiquidityPolicy{}
	for rows.Next() {
		policy, err := scanLiquidityPolicy(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, policy)
	}
	return result, rows.Err()
}

func listLiquidityReservationsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, includeReleased bool) ([]domain.LiquidityReservation, error) {
	statement := `SELECT id, household_id, source_kind, account_id, holding_id, currency, label, amount, revision, created_at, updated_at, released_at FROM liquidity_reservations WHERE household_id = ?`
	if !includeReleased {
		statement += ` AND released_at IS NULL`
	}
	statement += ` ORDER BY created_at, id`
	rows, err := query.QueryContext(ctx, statement, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.LiquidityReservation{}
	for rows.Next() {
		reservation, err := scanLiquidityReservation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, reservation)
	}
	return result, rows.Err()
}

func upsertLiquidityPolicyTx(ctx context.Context, tx *sql.Tx, policy domain.LiquidityPolicy) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO liquidity_policies(id, household_id, source_kind, account_id, holding_id, currency, access_kind, unlock_on, settlement_days, day_basis, receipt_on_override, accessible_amount_cap, normal_exit_fee, early_kind, early_settlement_days, early_day_basis, early_fee, early_amount_mode, early_gross_amount, confirmed_at, note, revision, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET access_kind = excluded.access_kind, unlock_on = excluded.unlock_on, settlement_days = excluded.settlement_days, day_basis = excluded.day_basis, receipt_on_override = excluded.receipt_on_override, accessible_amount_cap = excluded.accessible_amount_cap, normal_exit_fee = excluded.normal_exit_fee, early_kind = excluded.early_kind, early_settlement_days = excluded.early_settlement_days, early_day_basis = excluded.early_day_basis, early_fee = excluded.early_fee, early_amount_mode = excluded.early_amount_mode, early_gross_amount = excluded.early_gross_amount, confirmed_at = excluded.confirmed_at, note = excluded.note, revision = excluded.revision, updated_at = excluded.updated_at`,
		policy.ID.String(), policy.HouseholdID.String(), string(policy.Source.Kind), policy.Source.AccountID.String(), nullableID(policy.Source.HoldingID), nullableCurrency(policy.Source.Currency), string(policy.AccessKind), nullableString(policy.UnlockOn), nullableInt(policy.SettlementDays), nullableDayBasis(policy.DayBasis), nullableString(policy.ReceiptOnOverride), nullableMoneyAmount(policy.AccessibleAmountCap), nullableMoneyAmount(policy.NormalExitFee), string(policy.EarlyKind), nullableInt(policy.EarlySettlementDays), nullableDayBasis(policy.EarlyDayBasis), nullableMoneyAmount(policy.EarlyFee), nullableEarlyMode(policy.EarlyAmountMode), nullableMoneyAmount(policy.EarlyGrossAmount), nullableTime(policy.ConfirmedAt), nullableString(policy.Note), policy.Revision, formatTimestamp(policy.CreatedAt), formatTimestamp(policy.UpdatedAt))
	return err
}

func scanLiquidityPolicy(row scanner) (domain.LiquidityPolicy, error) {
	var id, householdID, sourceKind, accountID, accessKind, earlyKind, createdAt, updatedAt string
	var holdingID, currency, unlockOn, dayBasis, receiptOverride, cap, fee, earlyBasis, earlyFee, earlyMode, earlyGross, confirmedAt, note sql.NullString
	var settlement, earlySettlement sql.NullInt64
	var revision int
	var amountCurrency string
	if err := row.Scan(&id, &householdID, &sourceKind, &accountID, &holdingID, &currency, &accessKind, &unlockOn, &settlement, &dayBasis, &receiptOverride, &cap, &fee, &earlyKind, &earlySettlement, &earlyBasis, &earlyFee, &earlyMode, &earlyGross, &confirmedAt, &note, &revision, &createdAt, &updatedAt, &amountCurrency); err != nil {
		return domain.LiquidityPolicy{}, err
	}
	parsedID, err := domain.ParseLiquidityPolicyID(id)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	parsedAccount, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	parsedKind, err := domain.ParseLiquiditySourceKind(sourceKind)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	source := domain.LiquiditySourceRef{Kind: parsedKind, AccountID: parsedAccount}
	if holdingID.Valid {
		parsed, err := domain.ParseHoldingID(holdingID.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		source.HoldingID = &parsed
	}
	if currency.Valid {
		parsed, err := domain.ParseCurrency(currency.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		source.Currency = &parsed
	}
	parsedAccess, err := domain.ParseAccessKind(accessKind)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	parsedEarly, err := domain.ParseEarlyKind(earlyKind)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	policy := domain.LiquidityPolicy{ID: parsedID, HouseholdID: parsedHousehold, Source: source, AccessKind: parsedAccess, EarlyKind: parsedEarly, Revision: revision, CreatedAt: created, UpdatedAt: updated}
	policy.UnlockOn = nullableStringValue(unlockOn)
	policy.ReceiptOnOverride = nullableStringValue(receiptOverride)
	policy.Note = nullableStringValue(note)
	if settlement.Valid {
		value := int(settlement.Int64)
		policy.SettlementDays = &value
	}
	if dayBasis.Valid {
		parsed, err := domain.ParseDayBasis(dayBasis.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.DayBasis = &parsed
	}
	if earlySettlement.Valid {
		value := int(earlySettlement.Int64)
		policy.EarlySettlementDays = &value
	}
	if earlyBasis.Valid {
		parsed, err := domain.ParseDayBasis(earlyBasis.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.EarlyDayBasis = &parsed
	}
	if earlyMode.Valid {
		parsed, err := domain.ParseEarlyAmountMode(earlyMode.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.EarlyAmountMode = &parsed
	}
	if confirmedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, confirmedAt.String)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.ConfirmedAt = &parsed
	}
	currencyCode, err := domain.ParseCurrency(amountCurrency)
	if err != nil {
		return domain.LiquidityPolicy{}, err
	}
	if cap.Valid {
		money, err := domain.ParseMoney(cap.String, currencyCode)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.AccessibleAmountCap = &money
	}
	if fee.Valid {
		money, err := domain.ParseMoney(fee.String, currencyCode)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.NormalExitFee = &money
	}
	if earlyFee.Valid {
		money, err := domain.ParseMoney(earlyFee.String, currencyCode)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.EarlyFee = &money
	}
	if earlyGross.Valid {
		money, err := domain.ParseMoney(earlyGross.String, currencyCode)
		if err != nil {
			return domain.LiquidityPolicy{}, err
		}
		policy.EarlyGrossAmount = &money
	}
	return policy, nil
}

func scanLiquidityReservation(row scanner) (domain.LiquidityReservation, error) {
	var id, householdID, sourceKind, accountID, currency, label, amount, createdAt, updatedAt string
	var holdingID, releasedAt sql.NullString
	var revision int
	if err := row.Scan(&id, &householdID, &sourceKind, &accountID, &holdingID, &currency, &label, &amount, &revision, &createdAt, &updatedAt, &releasedAt); err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedID, err := domain.ParseLiquidityReservationID(id)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedAccount, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedKind, err := domain.ParseLiquiditySourceKind(sourceKind)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedCurrency, err := domain.ParseCurrency(currency)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	parsedAmount, err := domain.ParseMoney(amount, parsedCurrency)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.LiquidityReservation{}, err
	}
	source := domain.LiquiditySourceRef{Kind: parsedKind, AccountID: parsedAccount, Currency: &parsedCurrency}
	if parsedKind == domain.SourceHolding {
		source.Currency = nil
	}
	if holdingID.Valid {
		parsed, err := domain.ParseHoldingID(holdingID.String)
		if err != nil {
			return domain.LiquidityReservation{}, err
		}
		source.HoldingID = &parsed
	}
	reservation := domain.LiquidityReservation{ID: parsedID, HouseholdID: parsedHousehold, Source: source, Label: label, Amount: parsedAmount, Currency: parsedCurrency, Revision: revision, CreatedAt: created, UpdatedAt: updated}
	if releasedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, releasedAt.String)
		if err != nil {
			return domain.LiquidityReservation{}, err
		}
		reservation.ReleasedAt = &parsed
	}
	return reservation, nil
}

func nullableCurrency(value *domain.CurrencyCode) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableDayBasis(value *domain.DayBasis) any {
	if value == nil {
		return nil
	}
	return string(*value)
}

func nullableEarlyMode(value *domain.EarlyAmountMode) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
