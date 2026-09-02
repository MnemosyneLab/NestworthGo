package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// SaveDailyValuationSnapshotAndMarkCompleted persists the snapshot and
// advances the completion marker in one transaction so a crash or failure can
// never leave the snapshot stored while the state stays stale.
func (r *Repository) SaveDailyValuationSnapshotAndMarkCompleted(ctx context.Context, snapshot domain.DailyValuationSnapshot, updatedAt time.Time) (bool, error) {
	var appended bool
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		appended, err = saveDailyValuationSnapshotTx(ctx, tx, snapshot)
		if err != nil {
			return err
		}
		return markDailySnapshotCompletedTx(ctx, tx, snapshot.HouseholdID, snapshot.LocalDate, updatedAt)
	})
	return appended, err
}

func saveDailyValuationSnapshotTx(ctx context.Context, tx *sql.Tx, snapshot domain.DailyValuationSnapshot) (bool, error) {
	var existingID, existingHash string
	var existingRevision int
	err := tx.QueryRowContext(ctx, `SELECT id, revision, content_hash FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = ? ORDER BY revision DESC LIMIT 1`, snapshot.HouseholdID.String(), snapshot.LocalDate).Scan(&existingID, &existingRevision, &existingHash)
	if err == nil && existingHash == snapshot.ContentHash {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if snapshot.ID == "" {
		snapshot.ID = domain.NewDailyValuationSnapshotID()
	}
	if snapshot.Revision == 0 {
		snapshot.Revision = existingRevision + 1
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = snapshot.CutoffAt
	}
	if existingID != "" {
		previous, parseErr := domain.ParseDailyValuationSnapshotID(existingID)
		if parseErr != nil {
			return false, parseErr
		}
		snapshot.SupersedesID = &previous
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO daily_valuation_snapshots(id, household_id, local_date, cutoff_at, revision, supersedes_id, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID.String(), snapshot.HouseholdID.String(), snapshot.LocalDate, formatTimestamp(snapshot.CutoffAt), snapshot.Revision, nullableSnapshotID(snapshot.SupersedesID), snapshot.ContentHash, nullableMoneyAmount(snapshot.AssetsAmount), nullableMoneyAmount(snapshot.LiabilitiesAmount), nullableSignedMoneyAmount(snapshot.NetWorthAmount), snapshot.Currency.String(), boolValue(snapshot.Complete), snapshot.ComponentCount, snapshot.MissingCount, snapshot.GenerationReason, formatTimestamp(snapshot.CreatedAt)); err != nil {
		return false, err
	}
	for _, item := range snapshot.Items {
		if item.ID == "" {
			item.ID = domain.NewDailyValuationSnapshotItemID()
		}
		if err := item.ValidateNativeAmount(); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO daily_valuation_snapshot_items(id, snapshot_id, account_id, holding_id, instrument_id, native_amount, native_currency, base_amount, base_currency, quote_id, fx_quote_id, state_observation_id, preference_observation_id, complete, missing_reason, fx_preference_observation_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID.String(), snapshot.ID.String(), item.AccountID.String(), nullableSnapshotHoldingID(item.HoldingID), nullableSnapshotInstrumentID(item.InstrumentID), nullableSnapshotString(item.NativeAmount), nullableSnapshotCurrency(item.NativeCurrency), nullableMoneyAmount(item.BaseAmount), snapshot.Currency.String(), nullableSnapshotStringPtr(item.QuoteID), nullableSnapshotStringPtr(item.FXQuoteID), nullableSnapshotAccountObservationID(item.StateObservationID), nullableSnapshotPreferenceObservationID(item.PreferenceObservationID), boolValue(item.Complete), nullableSnapshotStringPtr(item.MissingReason), nullableSnapshotFXPreferenceObservationID(item.FXPreferenceObservationID)); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *Repository) MarkDailySnapshotCompleted(ctx context.Context, householdID domain.HouseholdID, localDate string, updatedAt time.Time) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return markDailySnapshotCompletedTx(ctx, tx, householdID, localDate, updatedAt)
	})
}

func markDailySnapshotCompletedTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, localDate string, updatedAt time.Time) error {
	nextDate, err := nextSnapshotDate(localDate)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from = ? THEN ? ELSE dirty_from END, last_completed_closed_on = CASE WHEN last_completed_closed_on IS NULL OR last_completed_closed_on < ? THEN ? ELSE last_completed_closed_on END, updated_at = ? WHERE household_id = ?`, localDate, nextDate, localDate, localDate, formatTimestamp(updatedAt), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "history snapshot state")
}

func (r *Repository) CompleteDailySnapshotRange(ctx context.Context, householdID domain.HouseholdID, targetDate string, updatedAt time.Time) error {
	result, err := r.database.SQL.ExecContext(ctx, `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from IS NULL OR dirty_from > ? THEN NULL ELSE dirty_from END, updated_at = ? WHERE household_id = ?`, targetDate, formatTimestamp(updatedAt), householdID.String())
	if err != nil {
		return err
	}
	return requireAffected(result, "history snapshot state")
}

func (r *Repository) DailySnapshotState(ctx context.Context, householdID domain.HouseholdID) (domain.DailySnapshotState, error) {
	var dirtyFrom, lastCompleted sql.NullString
	if err := r.database.SQL.QueryRowContext(ctx, `SELECT dirty_from, last_completed_closed_on FROM history_snapshot_state WHERE household_id = ?`, householdID.String()).Scan(&dirtyFrom, &lastCompleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DailySnapshotState{}, &domain.Error{Code: domain.ErrNotFound, Message: "history snapshot state was not found"}
		}
		return domain.DailySnapshotState{}, err
	}
	state := domain.DailySnapshotState{HouseholdID: householdID}
	if dirtyFrom.Valid {
		state.DirtyFrom = &dirtyFrom.String
	}
	if lastCompleted.Valid {
		state.LastCompletedClosedOn = &lastCompleted.String
	}
	return state, nil
}

func nextSnapshotDate(localDate string) (string, error) {
	parsed, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "localDate", Message: "snapshot date must use YYYY-MM-DD"}
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02"), nil
}

func (r *Repository) ListDailyValuationSnapshots(ctx context.Context, householdID domain.HouseholdID, since time.Time) ([]domain.DailyValuationSnapshot, error) {
	query := `SELECT id, local_date, cutoff_at, revision, supersedes_id, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at FROM daily_valuation_snapshots WHERE household_id = ? AND revision = (SELECT MAX(latest.revision) FROM daily_valuation_snapshots latest WHERE latest.household_id = daily_valuation_snapshots.household_id AND latest.local_date = daily_valuation_snapshots.local_date)`
	args := []any{householdID.String()}
	if !since.IsZero() {
		query += ` AND local_date >= ?`
		args = append(args, since.Format("2006-01-02"))
	}
	query += ` ORDER BY local_date ASC`
	rows, err := r.database.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var result []domain.DailyValuationSnapshot
	for rows.Next() {
		var id, localDate, cutoffAt, contentHash, currency, generationReason, createdAt string
		var revision, componentCount, missingCount, complete int
		var supersedes, assets, liabilities, netWorth sql.NullString
		if err := rows.Scan(&id, &localDate, &cutoffAt, &revision, &supersedes, &contentHash, &assets, &liabilities, &netWorth, &currency, &complete, &componentCount, &missingCount, &generationReason, &createdAt); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseDailyValuationSnapshotID(id)
		if err != nil {
			return nil, err
		}
		cutoff, err := time.Parse(time.RFC3339Nano, cutoffAt)
		if err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		baseCurrency, err := domain.ParseCurrency(currency)
		if err != nil {
			return nil, err
		}
		snapshot := domain.DailyValuationSnapshot{ID: parsedID, HouseholdID: householdID, LocalDate: localDate, CutoffAt: cutoff.UTC(), Revision: revision, ContentHash: contentHash, Currency: baseCurrency, Complete: complete != 0, ComponentCount: componentCount, MissingCount: missingCount, GenerationReason: generationReason, CreatedAt: created.UTC()}
		if supersedes.Valid && supersedes.String != "" {
			value, parseErr := domain.ParseDailyValuationSnapshotID(supersedes.String)
			if parseErr != nil {
				return nil, parseErr
			}
			snapshot.SupersedesID = &value
		}
		for _, source := range []struct {
			value sql.NullString
			out   **domain.Money
		}{{assets, &snapshot.AssetsAmount}, {liabilities, &snapshot.LiabilitiesAmount}} {
			if !source.value.Valid || source.value.String == "" {
				continue
			}
			value, parseErr := domain.ParseMoney(source.value.String, baseCurrency)
			if parseErr != nil {
				return nil, parseErr
			}
			*source.out = &value
		}
		if netWorth.Valid && netWorth.String != "" {
			value, parseErr := domain.ParseSignedMoney(netWorth.String, baseCurrency)
			if parseErr != nil {
				return nil, parseErr
			}
			snapshot.NetWorthAmount = &value
		}
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range result {
		result[index].Items, err = r.listDailyValuationSnapshotItems(ctx, result[index].ID, result[index].Currency)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) listDailyValuationSnapshotItems(ctx context.Context, snapshotID domain.DailyValuationSnapshotID, baseCurrency domain.CurrencyCode) ([]domain.DailyValuationSnapshotItem, error) {
	rows, err := r.database.SQL.QueryContext(ctx, `SELECT i.id, i.account_id, i.holding_id, i.instrument_id, i.native_amount, i.native_currency, i.base_amount, i.base_currency, i.quote_id, i.fx_quote_id, i.state_observation_id, i.preference_observation_id, i.complete, i.missing_reason, i.fx_preference_observation_id, a.tracking_mode FROM daily_valuation_snapshot_items i JOIN accounts a ON a.id = i.account_id WHERE i.snapshot_id = ? ORDER BY i.account_id, i.id`, snapshotID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.DailyValuationSnapshotItem
	for rows.Next() {
		var id, accountID, trackingMode string
		var holdingID, instrumentID, nativeAmount, nativeCurrency, baseAmount, rowBaseCurrency, quoteID, fxQuoteID, stateObservationID, preferenceObservationID, missingReason, fxPreferenceObservationID sql.NullString
		var complete int
		if err := rows.Scan(&id, &accountID, &holdingID, &instrumentID, &nativeAmount, &nativeCurrency, &baseAmount, &rowBaseCurrency, &quoteID, &fxQuoteID, &stateObservationID, &preferenceObservationID, &complete, &missingReason, &fxPreferenceObservationID, &trackingMode); err != nil {
			return nil, err
		}
		parsedID, err := domain.ParseDailyValuationSnapshotItemID(id)
		if err != nil {
			return nil, err
		}
		parsedAccount, err := domain.ParseAccountID(accountID)
		if err != nil {
			return nil, err
		}
		item := domain.DailyValuationSnapshotItem{ID: parsedID, SnapshotID: snapshotID, AccountID: parsedAccount, Complete: complete != 0}
		if holdingID.Valid && holdingID.String != "" {
			value, parseErr := domain.ParseHoldingID(holdingID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.HoldingID = &value
		}
		if instrumentID.Valid && instrumentID.String != "" {
			value, parseErr := domain.ParseInstrumentID(instrumentID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.InstrumentID = &value
		}
		if nativeAmount.Valid && nativeCurrency.Valid {
			currency, parseErr := domain.ParseCurrency(nativeCurrency.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.NativeAmount, item.NativeCurrency = nativeAmount.String, currency
			if parseErr := item.ValidateNativeAmount(); parseErr != nil {
				return nil, parseErr
			}
		}
		if baseAmount.Valid && baseAmount.String != "" {
			currency := baseCurrency
			if rowBaseCurrency.Valid && rowBaseCurrency.String != "" {
				currency, err = domain.ParseCurrency(rowBaseCurrency.String)
				if err != nil {
					return nil, err
				}
			}
			value, parseErr := domain.ParseMoney(baseAmount.String, currency)
			if parseErr != nil {
				return nil, parseErr
			}
			item.BaseAmount = &value
		}
		if quoteID.Valid && quoteID.String != "" {
			item.QuoteID = &quoteID.String
		}
		if fxQuoteID.Valid && fxQuoteID.String != "" {
			item.FXQuoteID = &fxQuoteID.String
		}
		if stateObservationID.Valid && stateObservationID.String != "" {
			value, parseErr := domain.ParseAccountStateObservationID(stateObservationID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.StateObservationID = &value
		}
		if preferenceObservationID.Valid && preferenceObservationID.String != "" {
			value, parseErr := domain.ParseInstrumentPreferenceObservationID(preferenceObservationID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.PreferenceObservationID = &value
		}
		if fxPreferenceObservationID.Valid && fxPreferenceObservationID.String != "" {
			value, parseErr := domain.ParseFXPreferenceObservationID(fxPreferenceObservationID.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.FXPreferenceObservationID = &value
		}
		if missingReason.Valid && missingReason.String != "" {
			item.MissingReason = &missingReason.String
		}
		if trackingMode != string(domain.TrackingHoldings) {
			item.ClassificationBasis = domain.ClassificationCurrentMetadataDerived
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func nullableSnapshotID(value *domain.DailyValuationSnapshotID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotHoldingID(value *domain.HoldingID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotInstrumentID(value *domain.InstrumentID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotAccountObservationID(value *domain.AccountStateObservationID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotPreferenceObservationID(value *domain.InstrumentPreferenceObservationID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotFXPreferenceObservationID(value *domain.FXPreferenceObservationID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableSnapshotString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableSnapshotStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableSnapshotCurrency(value domain.CurrencyCode) any {
	if value == "" {
		return nil
	}
	return value.String()
}
