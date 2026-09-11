package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// SaveDailyValuationSnapshotAndMarkCompleted persists the snapshot and
// advances the completion marker in one transaction so a crash or failure can
// never leave the snapshot stored while the state stays stale.
func (r *Repository) SaveDailyValuationSnapshotAndMarkCompleted(ctx context.Context, snapshot domain.DailyValuationSnapshot, updatedAt time.Time) (bool, error) {
	var state domain.DailySnapshotState
	var stateErr error
	state, stateErr = r.DailySnapshotState(ctx, snapshot.HouseholdID)
	if stateErr == nil {
		if snapshot.InputGeneration == 0 {
			snapshot.InputGeneration = state.InputGeneration
		}
		if snapshot.ResolverPolicyVersion == "" {
			snapshot.ResolverPolicyVersion = state.ResolverPolicyVersion
		}
	}
	return r.saveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, updatedAt, -1)
}

func (r *Repository) SaveDailyValuationSnapshotAndMarkCompletedAtGeneration(ctx context.Context, snapshot domain.DailyValuationSnapshot, updatedAt time.Time, expectedGeneration int) (bool, error) {
	return r.saveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, updatedAt, expectedGeneration)
}

func (r *Repository) saveDailyValuationSnapshotAndMarkCompleted(ctx context.Context, snapshot domain.DailyValuationSnapshot, updatedAt time.Time, expectedGeneration int) (bool, error) {
	var appended bool
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		var generation int
		var policy sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT input_generation, resolver_policy_version FROM history_snapshot_state WHERE household_id = ?`, snapshot.HouseholdID.String()).Scan(&generation, &policy); err != nil {
			return err
		}
		if expectedGeneration >= 0 && generation != expectedGeneration {
			return snapshotGenerationChanged()
		}
		if expectedGeneration < 0 {
			expectedGeneration = generation
		}
		if snapshot.InputGeneration == 0 {
			snapshot.InputGeneration = expectedGeneration
		}
		if snapshot.ResolverPolicyVersion == "" {
			snapshot.ResolverPolicyVersion = policy.String
		}
		var err error
		appended, err = saveDailyValuationSnapshotTx(ctx, tx, snapshot)
		if err != nil {
			return err
		}
		return markDailySnapshotCompletedAtGenerationTx(ctx, tx, snapshot.HouseholdID, snapshot.LocalDate, updatedAt, expectedGeneration)
	})
	return appended, err
}

func saveDailyValuationSnapshotTx(ctx context.Context, tx *sql.Tx, snapshot domain.DailyValuationSnapshot) (bool, error) {
	var existingID, existingHash string
	var existingRevision int
	err := tx.QueryRowContext(ctx, `SELECT id, revision, content_hash FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = ? ORDER BY revision DESC LIMIT 1`, snapshot.HouseholdID.String(), snapshot.LocalDate).Scan(&existingID, &existingRevision, &existingHash)
	if err == nil && existingHash == snapshot.ContentHash {
		policy := snapshot.ResolverPolicyVersion
		if policy == "" {
			policy = domain.MarketDataResolverPolicy
		}
		// The economic result can be identical after an input generation change.
		// Refresh its provenance nevertheless, so a successful conditional
		// completion never leaves a current snapshot labelled with an older
		// generation.
		_, updateErr := tx.ExecContext(ctx, `UPDATE daily_valuation_snapshots SET input_generation = ?, resolver_policy_version = ? WHERE id = ?`, snapshot.InputGeneration, policy, existingID)
		return false, updateErr
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
	policy := snapshot.ResolverPolicyVersion
	if policy == "" {
		policy = domain.MarketDataResolverPolicy
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO daily_valuation_snapshots(id, household_id, local_date, cutoff_at, revision, supersedes_id, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at, input_generation, resolver_policy_version) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID.String(), snapshot.HouseholdID.String(), snapshot.LocalDate, formatTimestamp(snapshot.CutoffAt), snapshot.Revision, nullableSnapshotID(snapshot.SupersedesID), snapshot.ContentHash, nullableMoneyAmount(snapshot.AssetsAmount), nullableMoneyAmount(snapshot.LiabilitiesAmount), nullableSignedMoneyAmount(snapshot.NetWorthAmount), snapshot.Currency.String(), boolValue(snapshot.Complete), snapshot.ComponentCount, snapshot.MissingCount, snapshot.GenerationReason, formatTimestamp(snapshot.CreatedAt), snapshot.InputGeneration, policy); err != nil {
		return false, err
	}
	for _, item := range snapshot.Items {
		if item.ID == "" {
			item.ID = domain.NewDailyValuationSnapshotItemID()
		}
		if err := item.ValidateNativeAmount(); err != nil {
			return false, err
		}
		if err := item.ValidateBaseAmountExact(); err != nil {
			return false, err
		}
		if err := validateSnapshotItemProvenance(ctx, tx, snapshot.HouseholdID, item); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO daily_valuation_snapshot_items(id, snapshot_id, account_id, holding_id, instrument_id, native_amount, native_currency, base_amount, base_currency, quote_id, fx_quote_id, state_observation_id, preference_observation_id, complete, missing_reason, fx_preference_observation_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID.String(), snapshot.ID.String(), item.AccountID.String(), nullableSnapshotHoldingID(item.HoldingID), nullableSnapshotInstrumentID(item.InstrumentID), nullableSnapshotString(item.NativeAmount), nullableSnapshotCurrency(item.NativeCurrency), snapshotItemStoredBaseAmount(item), snapshot.Currency.String(), nullableSnapshotStringPtr(item.QuoteID), nullableSnapshotStringPtr(item.FXQuoteID), nullableSnapshotAccountObservationID(item.StateObservationID), nullableSnapshotPreferenceObservationID(item.PreferenceObservationID), boolValue(item.Complete), nullableSnapshotStringPtr(item.MissingReason), nullableSnapshotFXPreferenceObservationID(item.FXPreferenceObservationID)); err != nil {
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
	return markDailySnapshotCompletedAtGenerationTx(ctx, tx, householdID, localDate, updatedAt, -1)
}

func markDailySnapshotCompletedAtGenerationTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, localDate string, updatedAt time.Time, expectedGeneration int) error {
	nextDate, err := nextSnapshotDate(localDate)
	if err != nil {
		return err
	}
	query := `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from = ? THEN ? ELSE dirty_from END, last_completed_closed_on = CASE WHEN last_completed_closed_on IS NULL OR last_completed_closed_on < ? THEN ? ELSE last_completed_closed_on END, updated_at = ? WHERE household_id = ?`
	args := []any{localDate, nextDate, localDate, localDate, formatTimestamp(updatedAt), householdID.String()}
	if expectedGeneration >= 0 {
		query += ` AND input_generation = ?`
		args = append(args, expectedGeneration)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if err := requireAffected(result, "history snapshot state"); err != nil {
		if expectedGeneration >= 0 {
			return snapshotGenerationChanged()
		}
		return err
	}
	return nil
}

func (r *Repository) CompleteDailySnapshotRange(ctx context.Context, householdID domain.HouseholdID, targetDate string, updatedAt time.Time) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return completeDailySnapshotRangeTx(ctx, tx, householdID, targetDate, updatedAt, -1)
	})
}

func (r *Repository) CompleteDailySnapshotRangeAtGeneration(ctx context.Context, householdID domain.HouseholdID, targetDate string, updatedAt time.Time, expectedGeneration int) error {
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		return completeDailySnapshotRangeTx(ctx, tx, householdID, targetDate, updatedAt, expectedGeneration)
	})
}

func completeDailySnapshotRangeTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, targetDate string, updatedAt time.Time, expectedGeneration int) error {
	query := `UPDATE history_snapshot_state SET dirty_from = CASE WHEN dirty_from IS NULL OR dirty_from > ? THEN NULL ELSE dirty_from END, dirty_to = CASE WHEN dirty_from IS NULL OR dirty_from > ? THEN NULL ELSE dirty_to END, updated_at = ? WHERE household_id = ?`
	args := []any{targetDate, targetDate, formatTimestamp(updatedAt), householdID.String()}
	if expectedGeneration >= 0 {
		query += ` AND input_generation = ?`
		args = append(args, expectedGeneration)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if err := requireAffected(result, "history snapshot state"); err != nil && expectedGeneration >= 0 {
		return snapshotGenerationChanged()
	} else if err != nil {
		return err
	}
	return nil
}

func snapshotGenerationChanged() error {
	return &domain.Error{Code: domain.ErrConflict, Field: "inputGeneration", Message: "snapshot input generation changed during rebuild"}
}

func (r *Repository) DailySnapshotState(ctx context.Context, householdID domain.HouseholdID) (domain.DailySnapshotState, error) {
	var dirtyFrom, dirtyTo, lastCompleted, policy sql.NullString
	var generation sql.NullInt64
	if err := r.database.SQL.QueryRowContext(ctx, `SELECT dirty_from, dirty_to, last_completed_closed_on, input_generation, resolver_policy_version FROM history_snapshot_state WHERE household_id = ?`, householdID.String()).Scan(&dirtyFrom, &dirtyTo, &lastCompleted, &generation, &policy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DailySnapshotState{}, &domain.Error{Code: domain.ErrNotFound, Message: "history snapshot state was not found"}
		}
		return domain.DailySnapshotState{}, err
	}
	state := domain.DailySnapshotState{HouseholdID: householdID, InputGeneration: int(generation.Int64)}
	if dirtyFrom.Valid {
		state.DirtyFrom = &dirtyFrom.String
	}
	if dirtyTo.Valid {
		state.DirtyTo = &dirtyTo.String
	}
	if lastCompleted.Valid {
		state.LastCompletedClosedOn = &lastCompleted.String
	}
	if policy.Valid {
		state.ResolverPolicyVersion = policy.String
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

func (r *Repository) ListDailyValuationSnapshots(ctx context.Context, householdID domain.HouseholdID, since, until time.Time) ([]domain.DailyValuationSnapshot, error) {
	return listDailyValuationSnapshotsQuery(ctx, r.database.SQL, householdID, since, until)
}

func listDailyValuationSnapshotsQuery(ctx context.Context, db queryer, householdID domain.HouseholdID, since, until time.Time) ([]domain.DailyValuationSnapshot, error) {
	query := `SELECT id, local_date, cutoff_at, revision, supersedes_id, content_hash, assets_amount, liabilities_amount, net_worth_amount, currency, complete, component_count, missing_count, generation_reason, created_at, input_generation, resolver_policy_version FROM daily_valuation_snapshots WHERE household_id = ? AND revision = (SELECT MAX(latest.revision) FROM daily_valuation_snapshots latest WHERE latest.household_id = daily_valuation_snapshots.household_id AND latest.local_date = daily_valuation_snapshots.local_date)`
	args := []any{householdID.String()}
	if !since.IsZero() {
		query += ` AND local_date >= ?`
		args = append(args, since.Format("2006-01-02"))
	}
	if !until.IsZero() {
		query += ` AND local_date <= ?`
		args = append(args, until.Format("2006-01-02"))
	}
	query += ` ORDER BY local_date ASC`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.DailyValuationSnapshot
	for rows.Next() {
		var id, localDate, cutoffAt, contentHash, currency, generationReason, createdAt string
		var revision, componentCount, missingCount, complete int
		var inputGeneration sql.NullInt64
		var resolverPolicy sql.NullString
		var supersedes, assets, liabilities, netWorth sql.NullString
		if err := rows.Scan(&id, &localDate, &cutoffAt, &revision, &supersedes, &contentHash, &assets, &liabilities, &netWorth, &currency, &complete, &componentCount, &missingCount, &generationReason, &createdAt, &inputGeneration, &resolverPolicy); err != nil {
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
		snapshot := domain.DailyValuationSnapshot{ID: parsedID, HouseholdID: householdID, LocalDate: localDate, CutoffAt: cutoff.UTC(), Revision: revision, ContentHash: contentHash, Currency: baseCurrency, Complete: complete != 0, ComponentCount: componentCount, MissingCount: missingCount, GenerationReason: generationReason, CreatedAt: created.UTC(), InputGeneration: int(inputGeneration.Int64), ResolverPolicyVersion: resolverPolicy.String}
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
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := attachDailyValuationSnapshotItems(ctx, db, result); err != nil {
		return nil, err
	}
	return result, nil
}

func attachDailyValuationSnapshotItems(ctx context.Context, query queryer, snapshots []domain.DailyValuationSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	ids := make([]string, len(snapshots))
	indexByID := make(map[string]int, len(snapshots))
	currencies := make(map[string]domain.CurrencyCode, len(snapshots))
	for index := range snapshots {
		id := snapshots[index].ID.String()
		ids[index] = id
		indexByID[id] = index
		currencies[id] = snapshots[index].Currency
		snapshots[index].Items = nil
	}
	clause, args := sqlInArgs(ids)
	rows, err := query.QueryContext(ctx, `SELECT i.snapshot_id, i.id, i.account_id, i.holding_id, i.instrument_id, i.native_amount, i.native_currency, i.base_amount, i.base_currency, i.quote_id, i.fx_quote_id, i.state_observation_id, i.preference_observation_id, i.complete, i.missing_reason, i.fx_preference_observation_id, a.tracking_mode FROM daily_valuation_snapshot_items i JOIN accounts a ON a.id = i.account_id WHERE i.snapshot_id IN (`+clause+`) ORDER BY i.snapshot_id, i.account_id, i.id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var snapshotID, id, accountID, trackingMode string
		var holdingID, instrumentID, nativeAmount, nativeCurrency, baseAmount, rowBaseCurrency, quoteID, fxQuoteID, stateObservationID, preferenceObservationID, missingReason, fxPreferenceObservationID sql.NullString
		var complete int
		if err := rows.Scan(&snapshotID, &id, &accountID, &holdingID, &instrumentID, &nativeAmount, &nativeCurrency, &baseAmount, &rowBaseCurrency, &quoteID, &fxQuoteID, &stateObservationID, &preferenceObservationID, &complete, &missingReason, &fxPreferenceObservationID, &trackingMode); err != nil {
			return err
		}
		index, ok := indexByID[snapshotID]
		if !ok {
			continue
		}
		parsedSnapshot, err := domain.ParseDailyValuationSnapshotID(snapshotID)
		if err != nil {
			return err
		}
		item, err := scanDailyValuationSnapshotItem(id, parsedSnapshot, accountID, holdingID, instrumentID, nativeAmount, nativeCurrency, baseAmount, rowBaseCurrency, quoteID, fxQuoteID, stateObservationID, preferenceObservationID, complete, missingReason, fxPreferenceObservationID, trackingMode, currencies[snapshotID])
		if err != nil {
			return err
		}
		snapshots[index].Items = append(snapshots[index].Items, item)
	}
	return rows.Err()
}

func scanDailyValuationSnapshotItem(id string, snapshotID domain.DailyValuationSnapshotID, accountID string, holdingID, instrumentID, nativeAmount, nativeCurrency, baseAmount, rowBaseCurrency, quoteID, fxQuoteID, stateObservationID, preferenceObservationID sql.NullString, complete int, missingReason, fxPreferenceObservationID sql.NullString, trackingMode string, baseCurrency domain.CurrencyCode) (domain.DailyValuationSnapshotItem, error) {
	parsedID, err := domain.ParseDailyValuationSnapshotItemID(id)
	if err != nil {
		return domain.DailyValuationSnapshotItem{}, err
	}
	parsedAccount, err := domain.ParseAccountID(accountID)
	if err != nil {
		return domain.DailyValuationSnapshotItem{}, err
	}
	item := domain.DailyValuationSnapshotItem{ID: parsedID, SnapshotID: snapshotID, AccountID: parsedAccount, Complete: complete != 0}
	if holdingID.Valid && holdingID.String != "" {
		value, parseErr := domain.ParseHoldingID(holdingID.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.HoldingID = &value
	}
	if instrumentID.Valid && instrumentID.String != "" {
		value, parseErr := domain.ParseInstrumentID(instrumentID.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.InstrumentID = &value
	}
	if nativeAmount.Valid && nativeCurrency.Valid {
		currency, parseErr := domain.ParseCurrency(nativeCurrency.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.NativeAmount, item.NativeCurrency = nativeAmount.String, currency
		if parseErr := item.ValidateNativeAmount(); parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
	}
	if baseAmount.Valid && baseAmount.String != "" {
		currency := baseCurrency
		if rowBaseCurrency.Valid && rowBaseCurrency.String != "" {
			currency, err = domain.ParseCurrency(rowBaseCurrency.String)
			if err != nil {
				return domain.DailyValuationSnapshotItem{}, err
			}
		}
		exact, parseErr := domain.ParseNativeAmount(baseAmount.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.BaseAmountExact = exact
		parsed, parsedErr := decimal.NewFromString(exact)
		if parsedErr != nil {
			return domain.DailyValuationSnapshotItem{}, &domain.Error{Code: domain.ErrIntegrity, Message: "stored snapshot base amount is invalid"}
		}
		rounded, roundErr := domain.NewMoney(parsed, currency)
		if roundErr != nil {
			return domain.DailyValuationSnapshotItem{}, roundErr
		}
		item.BaseAmount = &rounded
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
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.StateObservationID = &value
	}
	if preferenceObservationID.Valid && preferenceObservationID.String != "" {
		value, parseErr := domain.ParseInstrumentPreferenceObservationID(preferenceObservationID.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.PreferenceObservationID = &value
	}
	if fxPreferenceObservationID.Valid && fxPreferenceObservationID.String != "" {
		value, parseErr := domain.ParseFXPreferenceObservationID(fxPreferenceObservationID.String)
		if parseErr != nil {
			return domain.DailyValuationSnapshotItem{}, parseErr
		}
		item.FXPreferenceObservationID = &value
	}
	if missingReason.Valid && missingReason.String != "" {
		item.MissingReason = &missingReason.String
	}
	if trackingMode != string(domain.TrackingHoldings) {
		item.ClassificationBasis = domain.ClassificationCurrentMetadataDerived
	}
	return item, nil
}

func snapshotItemStoredBaseAmount(item domain.DailyValuationSnapshotItem) any {
	if item.BaseAmountExact != "" {
		return item.BaseAmountExact
	}
	return nullableMoneyAmount(item.BaseAmount)
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

func validateSnapshotItemProvenance(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, item domain.DailyValuationSnapshotItem) error {
	if item.QuoteID != nil && *item.QuoteID != "" {
		quoteID, err := domain.ParseInstrumentQuoteID(*item.QuoteID)
		if err != nil {
			return asStoredIntegrity("quoteId", err)
		}
		if err := requireHouseholdRow(ctx, tx, `SELECT COUNT(*) FROM instrument_quotes q JOIN instruments i ON i.id = q.instrument_id WHERE q.id = ? AND i.household_id = ?`, quoteID.String(), householdID.String(), "quoteId", "snapshot quote provenance does not exist in this household"); err != nil {
			return err
		}
	}
	if item.FXQuoteID != nil && *item.FXQuoteID != "" {
		fxQuoteID, err := domain.ParseFXQuoteID(*item.FXQuoteID)
		if err != nil {
			return asStoredIntegrity("fxQuoteId", err)
		}
		if err := requireHouseholdRow(ctx, tx, `SELECT COUNT(*) FROM fx_quotes WHERE id = ? AND household_id = ?`, fxQuoteID.String(), householdID.String(), "fxQuoteId", "snapshot FX quote provenance does not exist in this household"); err != nil {
			return err
		}
	}
	if item.StateObservationID != nil {
		if err := requireHouseholdRow(ctx, tx, `SELECT COUNT(*) FROM account_state_observations o JOIN accounts a ON a.id = o.account_id WHERE o.id = ? AND a.household_id = ?`, item.StateObservationID.String(), householdID.String(), "stateObservationId", "snapshot account-state provenance does not exist in this household"); err != nil {
			return err
		}
	}
	if item.PreferenceObservationID != nil {
		if err := requireHouseholdRow(ctx, tx, `SELECT COUNT(*) FROM instrument_preference_observations o JOIN instruments i ON i.id = o.instrument_id WHERE o.id = ? AND i.household_id = ?`, item.PreferenceObservationID.String(), householdID.String(), "preferenceObservationId", "snapshot instrument-preference provenance does not exist in this household"); err != nil {
			return err
		}
	}
	if item.FXPreferenceObservationID != nil {
		if err := requireHouseholdRow(ctx, tx, `SELECT COUNT(*) FROM fx_preference_observations WHERE id = ? AND household_id = ?`, item.FXPreferenceObservationID.String(), householdID.String(), "fxPreferenceObservationId", "snapshot FX-preference provenance does not exist in this household"); err != nil {
			return err
		}
	}
	return nil
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
