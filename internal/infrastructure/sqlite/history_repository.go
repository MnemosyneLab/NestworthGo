package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) HistoryOrigin(ctx context.Context, householdID domain.HouseholdID) (*domain.HistoryOrigin, error) {
	row := r.database.SQL.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE household_id = ?`, householdID.String())
	origin, err := scanHistoryOrigin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &origin, nil
}

func (r *Repository) ListHistoryOriginComponents(ctx context.Context, originID domain.HistoryOriginID) ([]domain.HistoryOriginComponent, error) {
	return listHistoryOriginComponentsQuery(ctx, r.database.SQL, originID)
}

func listHistoryOriginComponentsQuery(ctx context.Context, query queryer, originID domain.HistoryOriginID) ([]domain.HistoryOriginComponent, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, origin_id, component_kind, account_id, holding_id, instrument_id, amount, currency, quantity, unit_cost, created_at FROM history_origin_components WHERE origin_id = ? ORDER BY component_kind ASC, account_id ASC, holding_id ASC, id ASC`, originID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.HistoryOriginComponent, 0)
	for rows.Next() {
		var id, origin, kind, createdAt string
		var accountID, holdingID, instrumentID, amount, currency, quantity, unitCost sql.NullString
		if err := rows.Scan(&id, &origin, &kind, &accountID, &holdingID, &instrumentID, &amount, &currency, &quantity, &unitCost, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseHistoryOriginComponentID(id)
		if err != nil {
			return nil, err
		}
		parsedOrigin, err := domain.ParseHistoryOriginID(origin)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		component := domain.HistoryOriginComponent{ID: parsedID, OriginID: parsedOrigin, Kind: domain.HistoryOriginComponentKind(kind), CreatedAt: created.UTC()}
		if accountID.Valid {
			parsed, parseErr := domain.ParseAccountID(accountID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			component.AccountID = &parsed
		}
		if holdingID.Valid {
			parsed, parseErr := domain.ParseHoldingID(holdingID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			component.HoldingID = &parsed
		}
		if instrumentID.Valid {
			parsed, parseErr := domain.ParseInstrumentID(instrumentID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			component.InstrumentID = &parsed
		}
		if amount.Valid && currency.Valid {
			parsedCurrency, parseErr := domain.ParseCurrency(currency.String)
			if parseErr != nil {
				return nil, parseErr
			}
			parsedAmount, parseErr := domain.ParseMoney(amount.String, parsedCurrency)
			if parseErr != nil {
				return nil, parseErr
			}
			component.Amount = &parsedAmount
		}
		if quantity.Valid {
			parsedQuantity, parseErr := domain.ParseQuantity(quantity.String)
			if parseErr != nil {
				return nil, parseErr
			}
			component.Quantity = &parsedQuantity
		}
		if unitCost.Valid {
			parsedUnitCost, parseErr := domain.ParseUnitPrice(unitCost.String)
			if parseErr != nil {
				return nil, parseErr
			}
			component.UnitCost = &parsedUnitCost
		}
		if err := component.Validate(); err != nil {
			return nil, err
		}
		result = append(result, component)
	}
	return result, rows.Err()
}

// HistoryOriginData loads the immutable baseline together with all baseline
// state needed to reconstruct an as-of portfolio. Keeping this read beside
// StartHistory avoids accidentally falling back to today's metadata.
func (r *Repository) HistoryOriginData(ctx context.Context, originID domain.HistoryOriginID) (domain.HistoryOriginData, error) {
	return historyOriginDataQuery(ctx, r.database.SQL, originID)
}

func historyOriginDataQuery(ctx context.Context, query queryer, originID domain.HistoryOriginID) (domain.HistoryOriginData, error) {
	var data domain.HistoryOriginData
	if _, err := domain.ParseHistoryOriginID(originID.String()); err != nil {
		return data, err
	}
	if _, err := scanHistoryOrigin(query.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE id = ?`, originID.String()), &data.Origin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return data, &domain.Error{Code: domain.ErrNotFound, Message: "history origin was not found"}
		}
		return data, err
	}
	components, err := listHistoryOriginComponentsQuery(ctx, query, originID)
	if err != nil {
		return data, err
	}
	data.Components = components

	rows, err := query.QueryContext(ctx, `SELECT account_id, archived_at, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, created_at FROM history_origin_account_states WHERE origin_id = ? ORDER BY account_id`, originID.String())
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var accountID, createdAt string
		var archivedAt sql.NullString
		var includeNetWorth, includeInvestment, includeLiquid int
		if err := rows.Scan(&accountID, &archivedAt, &includeNetWorth, &includeInvestment, &includeLiquid, &createdAt); err != nil {
			rows.Close()
			return data, err
		}
		parsedAccount, err := domain.ParseAccountID(accountID)
		if err != nil {
			rows.Close()
			return data, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			rows.Close()
			return data, err
		}
		archived, err := parseTimePtr(archivedAt)
		if err != nil {
			rows.Close()
			return data, err
		}
		data.AccountStates = append(data.AccountStates, domain.HistoryOriginAccountState{OriginID: originID, AccountID: parsedAccount, ArchivedAt: archived, IncludeInNetWorth: includeNetWorth != 0, IncludeInPortfolio: includeInvestment != 0, IncludeInLiquidAssets: includeLiquid != 0, CreatedAt: created.UTC()})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `SELECT account_id, member_id, share_bps FROM history_origin_ownership WHERE origin_id = ? ORDER BY account_id, member_id`, originID.String())
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var accountID, memberID string
		var shareBPS int
		if err := rows.Scan(&accountID, &memberID, &shareBPS); err != nil {
			rows.Close()
			return data, err
		}
		parsedAccount, err := domain.ParseAccountID(accountID)
		if err != nil {
			rows.Close()
			return data, err
		}
		parsedMember, err := domain.ParseMemberID(memberID)
		if err != nil {
			rows.Close()
			return data, err
		}
		data.Ownership = append(data.Ownership, domain.HistoryOriginOwnership{OriginID: originID, AccountID: parsedAccount, MemberID: parsedMember, ShareBPS: shareBPS})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `SELECT instrument_id, source_kind, created_at FROM history_origin_instrument_preferences WHERE origin_id = ? ORDER BY instrument_id`, originID.String())
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var instrumentID, sourceKind, createdAt string
		if err := rows.Scan(&instrumentID, &sourceKind, &createdAt); err != nil {
			rows.Close()
			return data, err
		}
		parsedInstrument, err := domain.ParseInstrumentID(instrumentID)
		if err != nil {
			rows.Close()
			return data, err
		}
		parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
		if err != nil {
			rows.Close()
			return data, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			rows.Close()
			return data, err
		}
		data.InstrumentPreferences = append(data.InstrumentPreferences, domain.HistoryOriginInstrumentPreference{OriginID: originID, InstrumentID: parsedInstrument, SourceKind: parsedSource, CreatedAt: created.UTC()})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	rows, err = query.QueryContext(ctx, `SELECT currency_a, currency_b, source_kind, created_at FROM history_origin_fx_preferences WHERE origin_id = ? ORDER BY currency_a, currency_b`, originID.String())
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var currencyA, currencyB, sourceKind, createdAt string
		if err := rows.Scan(&currencyA, &currencyB, &sourceKind, &createdAt); err != nil {
			rows.Close()
			return data, err
		}
		parsedA, err := domain.ParseCurrency(currencyA)
		if err != nil {
			rows.Close()
			return data, err
		}
		parsedB, err := domain.ParseCurrency(currencyB)
		if err != nil {
			rows.Close()
			return data, err
		}
		parsedSource, err := domain.ParseQuoteSourceKind(sourceKind)
		if err != nil {
			rows.Close()
			return data, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			rows.Close()
			return data, err
		}
		data.FXPreferences = append(data.FXPreferences, domain.HistoryOriginFXPreference{OriginID: originID, CurrencyA: parsedA, CurrencyB: parsedB, SourceKind: parsedSource, CreatedAt: created.UTC()})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()
	return data, nil
}

func (r *Repository) StartHistory(ctx context.Context, data domain.HistoryOriginData) (domain.HistoryOrigin, error) {
	if err := validateHistoryOriginData(data); err != nil {
		return domain.HistoryOrigin{}, err
	}
	var result domain.HistoryOrigin
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var existingID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM history_origins WHERE household_id = ?`, data.Origin.HouseholdID.String()).Scan(&existingID); err == nil {
			parsedID, parseErr := domain.ParseHistoryOriginID(existingID)
			if parseErr != nil {
				return parseErr
			}
			_, scanErr := scanHistoryOrigin(tx.QueryRowContext(ctx, `SELECT id, household_id, timezone, started_at, created_at FROM history_origins WHERE id = ?`, parsedID.String()), &result)
			return scanErr
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var householdID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE id = ?`, data.Origin.HouseholdID.String()).Scan(&householdID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "Household was not found"}
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origins(id, household_id, timezone, started_at, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), data.Origin.HouseholdID.String(), data.Origin.Timezone, formatTimestamp(data.Origin.StartedAt), formatTimestamp(data.Origin.CreatedAt)); err != nil {
			return err
		}
		for _, component := range data.Components {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_components(id, origin_id, component_kind, account_id, holding_id, instrument_id, amount, currency, quantity, unit_cost, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, component.ID.String(), data.Origin.ID.String(), string(component.Kind), nullableID(component.AccountID), nullableID(component.HoldingID), nullableID(component.InstrumentID), nullableMoney(component.Amount), nullableMoneyCurrency(component.Amount), nullableQuantity(component.Quantity), nullableUnitPrice(component.UnitCost), formatTimestamp(component.CreatedAt)); err != nil {
				return err
			}
		}
		for _, state := range data.AccountStates {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_account_states(origin_id, account_id, archived_at, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, data.Origin.ID.String(), state.AccountID.String(), nullableTime(state.ArchivedAt), boolValue(state.IncludeInNetWorth), boolValue(state.IncludeInPortfolio), boolValue(state.IncludeInLiquidAssets), formatTimestamp(state.CreatedAt)); err != nil {
				return err
			}
		}
		for _, ownership := range data.Ownership {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_ownership(origin_id, account_id, member_id, share_bps) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), ownership.AccountID.String(), ownership.MemberID.String(), ownership.ShareBPS); err != nil {
				return err
			}
		}
		for _, preference := range data.InstrumentPreferences {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_instrument_preferences(origin_id, instrument_id, source_kind, created_at) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), preference.InstrumentID.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
				return err
			}
		}
		for _, preference := range data.FXPreferences {
			if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_fx_preferences(origin_id, currency_a, currency_b, source_kind, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), preference.CurrencyA.String(), preference.CurrencyB.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at) VALUES(?, NULL, NULL, ?)`, data.Origin.HouseholdID.String(), formatTimestamp(data.Origin.CreatedAt)); err != nil {
			return err
		}
		result = data.Origin
		return nil
	})
	return result, err
}

func (r *Repository) CreateOnboardingWithHistory(ctx context.Context, household domain.Household, members []domain.Member, data domain.HistoryOriginData) error {
	if len(members) == 0 {
		return &domain.Error{Code: domain.ErrValidation, Field: "members", Message: "at least one member is required"}
	}
	data.Origin.HouseholdID = household.ID
	if err := validateHistoryOriginData(data); err != nil {
		return err
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM households WHERE singleton_key = 1`).Scan(&existing); err == nil {
			return &domain.Error{Code: domain.ErrConflict, Message: "a Household already exists"}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO households(id, singleton_key, name, base_currency, created_at, updated_at) VALUES(?, 1, ?, ?, ?, ?)`, household.ID.String(), household.Name, household.BaseCurrency.String(), formatTimestamp(household.CreatedAt), formatTimestamp(household.UpdatedAt)); err != nil {
			return err
		}
		for index, member := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO members(id, household_id, name, icon_key, note, sort_order, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, member.ID.String(), household.ID.String(), member.Name, nullableString(member.IconKey), nullableString(member.Note), index, formatTimestamp(member.CreatedAt), formatTimestamp(member.UpdatedAt)); err != nil {
				return err
			}
		}
		return insertHistoryOriginTx(ctx, tx, data)
	})
}

func insertHistoryOriginTx(ctx context.Context, tx *sql.Tx, data domain.HistoryOriginData) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO history_origins(id, household_id, timezone, started_at, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), data.Origin.HouseholdID.String(), data.Origin.Timezone, formatTimestamp(data.Origin.StartedAt), formatTimestamp(data.Origin.CreatedAt)); err != nil {
		return err
	}
	for _, component := range data.Components {
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_components(id, origin_id, component_kind, account_id, holding_id, instrument_id, amount, currency, quantity, unit_cost, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, component.ID.String(), data.Origin.ID.String(), string(component.Kind), nullableID(component.AccountID), nullableID(component.HoldingID), nullableID(component.InstrumentID), nullableMoney(component.Amount), nullableMoneyCurrency(component.Amount), nullableQuantity(component.Quantity), nullableUnitPrice(component.UnitCost), formatTimestamp(component.CreatedAt)); err != nil {
			return err
		}
	}
	for _, state := range data.AccountStates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_account_states(origin_id, account_id, archived_at, include_in_net_worth, include_in_portfolio, include_in_liquid_assets, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, data.Origin.ID.String(), state.AccountID.String(), nullableTime(state.ArchivedAt), boolValue(state.IncludeInNetWorth), boolValue(state.IncludeInPortfolio), boolValue(state.IncludeInLiquidAssets), formatTimestamp(state.CreatedAt)); err != nil {
			return err
		}
	}
	for _, ownership := range data.Ownership {
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_ownership(origin_id, account_id, member_id, share_bps) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), ownership.AccountID.String(), ownership.MemberID.String(), ownership.ShareBPS); err != nil {
			return err
		}
	}
	for _, preference := range data.InstrumentPreferences {
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_instrument_preferences(origin_id, instrument_id, source_kind, created_at) VALUES(?, ?, ?, ?)`, data.Origin.ID.String(), preference.InstrumentID.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
			return err
		}
	}
	for _, preference := range data.FXPreferences {
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_origin_fx_preferences(origin_id, currency_a, currency_b, source_kind, created_at) VALUES(?, ?, ?, ?, ?)`, data.Origin.ID.String(), preference.CurrencyA.String(), preference.CurrencyB.String(), string(preference.SourceKind), formatTimestamp(preference.CreatedAt)); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at) VALUES(?, NULL, NULL, ?)`, data.Origin.HouseholdID.String(), formatTimestamp(data.Origin.CreatedAt))
	return err
}

func validateHistoryOriginData(data domain.HistoryOriginData) error {
	if _, err := domain.ParseHistoryOriginID(data.Origin.ID.String()); err != nil {
		return &domain.Error{Code: domain.ErrValidation, Field: "originId", Message: "origin ID is required"}
	}
	if _, err := domain.NewHistoryOrigin(data.Origin.HouseholdID, data.Origin.Timezone, data.Origin.StartedAt, data.Origin.CreatedAt); err != nil {
		return err
	}
	for index := range data.Components {
		component := data.Components[index]
		component.OriginID = data.Origin.ID
		if err := component.Validate(); err != nil {
			return err
		}
		data.Components[index].OriginID = data.Origin.ID
	}
	return nil
}

func scanHistoryOrigin(row interface{ Scan(...any) error }, destination ...*domain.HistoryOrigin) (domain.HistoryOrigin, error) {
	var id, householdID, timezone, startedAt, createdAt string
	if err := row.Scan(&id, &householdID, &timezone, &startedAt, &createdAt); err != nil {
		return domain.HistoryOrigin{}, err
	}
	parsedID, err := domain.ParseHistoryOriginID(id)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	parsedHousehold, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	result := domain.HistoryOrigin{ID: parsedID, HouseholdID: parsedHousehold, Timezone: timezone, StartedAt: started.UTC(), CreatedAt: created.UTC()}
	if len(destination) > 0 && destination[0] != nil {
		*destination[0] = result
	}
	return result, nil
}

func nullableMoney(value *domain.Money) any {
	if value == nil {
		return nil
	}
	return value.CanonicalAmount()
}

func nullableUnitPrice(value *domain.UnitPrice) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}

func nullableMoneyCurrency(value *domain.Money) any {
	if value == nil {
		return nil
	}
	return value.Currency().String()
}

func nullableQuantity(value *domain.Quantity) any {
	if value == nil {
		return nil
	}
	return value.Canonical()
}
