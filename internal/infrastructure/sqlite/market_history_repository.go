package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (r *Repository) CommitInstrumentHistory(ctx context.Context, request InstrumentHistoryCommit) (HistoryCommitResult, error) {
	var bases []string
	for _, observation := range request.Observations {
		bases = append(bases, observation.PriceBasis)
	}
	if err := refuseFailClosedHistory(request.Status, request.Reason, request.Adapter, bases); err != nil {
		return HistoryCommitResult{}, err
	}
	var result HistoryCommitResult
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := refuseFailClosedHistory(request.Status, request.Reason, request.Adapter, bases); err != nil {
			return err
		}
		var householdID, quoteCurrency, providerKey, providerSymbol, market sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT household_id, quote_currency, provider_key, provider_symbol, market_code FROM instruments WHERE id = ?`, request.InstrumentID.String()).Scan(&householdID, &quoteCurrency, &providerKey, &providerSymbol, &market); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found"}
			}
			return err
		}
		if householdID.String != request.HouseholdID.String() {
			return &domain.Error{Code: domain.ErrValidation, Field: "householdId", Message: "instrument does not belong to this household"}
		}
		if request.QuoteCurrency != "" && quoteCurrency.String != request.QuoteCurrency.String() {
			return &domain.Error{Code: domain.ErrValidation, Field: "currency", Message: "quote currency does not match instrument"}
		}
		if strings.TrimSpace(request.ProviderKey) == "" || strings.TrimSpace(request.ProviderSymbol) == "" {
			return &domain.Error{Code: domain.ErrValidation, Field: "provider", Message: "provider binding is required"}
		}
		if providerKey.Valid && providerKey.String != "" && providerKey.String != request.ProviderKey {
			return &domain.Error{Code: domain.ErrValidation, Field: "providerKey", Message: "persisted provider key does not match the batch binding"}
		}
		if providerSymbol.Valid && providerSymbol.String != "" && providerSymbol.String != request.ProviderSymbol {
			return &domain.Error{Code: domain.ErrValidation, Field: "providerSymbol", Message: "persisted provider symbol does not match the batch binding"}
		}
		fetchedAt := request.FetchedAt
		if fetchedAt.IsZero() {
			fetchedAt = time.Now().UTC()
		}
		marketCode := request.Market
		if marketCode == "" && market.Valid {
			marketCode = market.String
		}
		revision, _, err := syncInstrumentProviderBindingTx(ctx, tx, request.InstrumentID.String(), request.ProviderKey, request.ProviderSymbol, marketCode, quoteCurrency.String, fetchedAt)
		if err != nil {
			return err
		}
		policy := sourcePolicyVersion(request.SourcePolicy)
		changed := false
		observationDates := make(map[string]struct{}, len(request.Observations))
		for _, observation := range request.Observations {
			wrote, err := persistInstrumentObservationTx(ctx, tx, request, observation, revision, policy, fetchedAt)
			if err != nil {
				return err
			}
			if wrote.persisted {
				result.PersistedObservations++
				changed = true
			}
			if wrote.statusReconciled {
				changed = true
			}
			if wrote.newRevision {
				result.NewRevisions++
				changed = true
			}
			if wrote.slot {
				result.CanonicalSlots++
			}
			observationDates[observation.MarketDate] = struct{}{}
		}
		coverage, err := persistInstrumentCoverageTx(ctx, tx, request, revision, policy, fetchedAt, observationDates)
		if err != nil {
			return err
		}
		result.CoverageDays += coverage
		if coverage > 0 {
			changed = true
		}
		if !changed {
			result.Unchanged = true
			generation, err := currentInputGenerationTx(ctx, tx, request.HouseholdID)
			result.InputGeneration = generation
			return err
		}
		dirtyFrom, dirtyTo := instrumentHistoryDirtyBounds(request.Observations, request.VerifiedRanges, request.PendingRanges)
		generation, err := bumpHistoryInputGenerationTx(ctx, tx, request.HouseholdID, dirtyFrom, dirtyTo, fetchedAt)
		if err != nil {
			return err
		}
		result.InputGeneration = generation
		return nil
	})
	return result, err
}

func (r *Repository) CommitFXHistory(ctx context.Context, request FXHistoryCommit) (HistoryCommitResult, error) {
	if err := refuseFailClosedHistory(request.Status, request.Reason, request.Adapter, nil); err != nil {
		return HistoryCommitResult{}, err
	}
	var result HistoryCommitResult
	err := r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if err := refuseFailClosedHistory(request.Status, request.Reason, request.Adapter, nil); err != nil {
			return err
		}
		if err := ensureHousehold(ctx, tx, request.HouseholdID); err != nil {
			return err
		}
		fetchedAt := request.FetchedAt
		if fetchedAt.IsZero() {
			fetchedAt = time.Now().UTC()
		}
		policy := sourcePolicyVersion(request.SourcePolicy)
		changed := false
		observationDates := make(map[string]struct{}, len(request.Observations))
		for _, observation := range request.Observations {
			wrote, err := persistFXObservationTx(ctx, tx, request, observation, policy, fetchedAt)
			if err != nil {
				return err
			}
			if wrote.persisted {
				result.PersistedObservations++
				changed = true
			}
			if wrote.statusReconciled {
				changed = true
			}
			if wrote.newRevision {
				result.NewRevisions++
				changed = true
			}
			if wrote.slot {
				result.CanonicalSlots++
			}
			observationDates[observation.MarketDate] = struct{}{}
		}
		coverage, err := persistFXCoverageTx(ctx, tx, request, policy, fetchedAt, observationDates)
		if err != nil {
			return err
		}
		result.CoverageDays += coverage
		if coverage > 0 {
			changed = true
		}
		if !changed {
			result.Unchanged = true
			generation, err := currentInputGenerationTx(ctx, tx, request.HouseholdID)
			result.InputGeneration = generation
			return err
		}
		dirtyFrom, dirtyTo := fxHistoryDirtyBounds(request.Observations, request.VerifiedRanges, request.PendingRanges)
		generation, err := bumpHistoryInputGenerationTx(ctx, tx, request.HouseholdID, dirtyFrom, dirtyTo, fetchedAt)
		if err != nil {
			return err
		}
		result.InputGeneration = generation
		return nil
	})
	return result, err
}

type persistWrite struct {
	persisted        bool
	newRevision      bool
	slot             bool
	statusReconciled bool
}

func persistInstrumentObservationTx(ctx context.Context, tx *sql.Tx, request InstrumentHistoryCommit, observation InstrumentHistoryObservation, bindingRevision int, policy string, fetchedAt time.Time) (persistWrite, error) {
	price, err := domain.ParseUnitPrice(observation.Value)
	if err != nil {
		return persistWrite{}, err
	}
	currency := request.QuoteCurrency
	if observation.Currency != "" {
		parsed, parseErr := domain.ParseCurrency(observation.Currency)
		if parseErr != nil {
			return persistWrite{}, parseErr
		}
		currency = parsed
	}
	quotedAt := observation.ValueEffectiveAt
	if quotedAt.IsZero() {
		quotedAt = fetchedAt
	}
	kind := observation.Kind
	if kind == "" {
		kind = observationKindClose
	}
	statusReconciled := false
	if kind == observationKindClose {
		statusReconciled, err = clearDayStatusTx(ctx, tx, "instrument", request.InstrumentID.String(), request.ProviderKey, request.HouseholdID.String(), bindingRevision, policy, observation.MarketDate)
		if err != nil {
			return persistWrite{}, err
		}
	}
	if existing, err := canonicalInstrumentSlotQuoteTx(ctx, tx, request.InstrumentID.String(), request.ProviderKey, bindingRevision, policy, observation.MarketDate, kind); err != nil {
		return persistWrite{}, err
	} else if existing != nil && existing.unitPrice == price.Canonical() && existing.currency == currency.String() && existing.priceBasis == observation.PriceBasis && existing.valueEffectiveAt == formatTimestamp(observation.ValueEffectiveAt) {
		return persistWrite{statusReconciled: statusReconciled}, nil
	} else if existing != nil {
		quoteID := domain.NewInstrumentQuoteID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, provider_timestamp, fetched_at, value_effective_at, binding_revision, source_policy_version, price_basis, timestamp_basis, revision, supersedes_quote_id, split_factor, dividend_cash) VALUES(?, ?, ?, ?, 'provider', ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			quoteID.String(), request.InstrumentID.String(), price.Canonical(), currency.String(), request.ProviderKey, formatTimestamp(quotedAt), formatTimestamp(fetchedAt), kind, observation.MarketDate, nullableTimeValue(observation.ProviderTimestamp), formatTimestamp(fetchedAt), formatTimestamp(observation.ValueEffectiveAt), bindingRevision, policy, observation.PriceBasis, observation.TimestampBasis, existing.revision+1, existing.id, nullableEmpty(observation.SplitFactor), nullableEmpty(observation.DividendCash)); err != nil {
			return persistWrite{}, mapPortfolioWriteError(err, "instrument quote")
		}
		if err := upsertInstrumentObservationSlotTx(ctx, tx, request.InstrumentID.String(), request.ProviderKey, bindingRevision, policy, observation.MarketDate, kind, quoteID.String(), fetchedAt); err != nil {
			return persistWrite{}, err
		}
		return persistWrite{persisted: true, newRevision: true, slot: kind == observationKindClose, statusReconciled: statusReconciled}, nil
	}
	quoteID := domain.NewInstrumentQuoteID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, provider_timestamp, fetched_at, value_effective_at, binding_revision, source_policy_version, price_basis, timestamp_basis, revision, split_factor, dividend_cash) VALUES(?, ?, ?, ?, 'provider', ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		quoteID.String(), request.InstrumentID.String(), price.Canonical(), currency.String(), request.ProviderKey, formatTimestamp(quotedAt), formatTimestamp(fetchedAt), kind, observation.MarketDate, nullableTimeValue(observation.ProviderTimestamp), formatTimestamp(fetchedAt), formatTimestamp(observation.ValueEffectiveAt), bindingRevision, policy, observation.PriceBasis, observation.TimestampBasis, nullableEmpty(observation.SplitFactor), nullableEmpty(observation.DividendCash)); err != nil {
		return persistWrite{}, mapPortfolioWriteError(err, "instrument quote")
	}
	slot := kind == observationKindClose
	if slot {
		if err := upsertInstrumentObservationSlotTx(ctx, tx, request.InstrumentID.String(), request.ProviderKey, bindingRevision, policy, observation.MarketDate, kind, quoteID.String(), fetchedAt); err != nil {
			return persistWrite{}, err
		}
	}
	return persistWrite{persisted: true, newRevision: true, slot: slot, statusReconciled: statusReconciled}, nil
}

func persistFXObservationTx(ctx context.Context, tx *sql.Tx, request FXHistoryCommit, observation FXHistoryObservation, policy string, fetchedAt time.Time) (persistWrite, error) {
	rate, err := domain.ParseFxRate(observation.Rate)
	if err != nil {
		return persistWrite{}, err
	}
	base := request.BaseCurrency
	quote := request.QuoteCurrency
	if observation.BaseCurrency != "" {
		parsed, parseErr := domain.ParseCurrency(observation.BaseCurrency)
		if parseErr != nil {
			return persistWrite{}, parseErr
		}
		base = parsed
	}
	if observation.QuoteCurrency != "" {
		parsed, parseErr := domain.ParseCurrency(observation.QuoteCurrency)
		if parseErr != nil {
			return persistWrite{}, parseErr
		}
		quote = parsed
	}
	quotedAt := observation.ValueEffectiveAt
	if quotedAt.IsZero() {
		quotedAt = fetchedAt
	}
	kind := observation.Kind
	if kind == "" {
		kind = observationKindDailyReference
	}
	statusReconciled := false
	if kind == observationKindDailyReference {
		targetID := base.String() + "/" + quote.String()
		statusReconciled, err = clearDayStatusTx(ctx, tx, "fx", targetID, request.ProviderKey, request.HouseholdID.String(), 0, policy, observation.MarketDate)
		if err != nil {
			return persistWrite{}, err
		}
	}
	if existing, err := canonicalFXSlotQuoteTx(ctx, tx, request.HouseholdID.String(), base.String(), quote.String(), request.ProviderKey, policy, observation.MarketDate, kind); err != nil {
		return persistWrite{}, err
	} else if existing != nil && existing.unitPrice == rate.Canonical() {
		return persistWrite{statusReconciled: statusReconciled}, nil
	} else if existing != nil {
		quoteID := domain.NewFXQuoteID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, fetched_at, value_effective_at, source_policy_version, timestamp_basis, revision, supersedes_quote_id) VALUES(?, ?, ?, ?, ?, 'provider', ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)`,
			quoteID.String(), request.HouseholdID.String(), base.String(), quote.String(), rate.Canonical(), request.ProviderKey, formatTimestamp(quotedAt), formatTimestamp(fetchedAt), kind, observation.MarketDate, formatTimestamp(fetchedAt), formatTimestamp(observation.ValueEffectiveAt), policy, observation.TimestampBasis, existing.revision+1, existing.id); err != nil {
			return persistWrite{}, mapPortfolioWriteError(err, "FX quote")
		}
		if err := upsertFXObservationSlotTx(ctx, tx, request.HouseholdID.String(), base.String(), quote.String(), request.ProviderKey, policy, observation.MarketDate, kind, quoteID.String(), fetchedAt); err != nil {
			return persistWrite{}, err
		}
		return persistWrite{persisted: true, newRevision: true, slot: kind == observationKindDailyReference, statusReconciled: statusReconciled}, nil
	}
	quoteID := domain.NewFXQuoteID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO fx_quotes(id, household_id, base_currency, quote_currency, rate, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, fetched_at, value_effective_at, source_policy_version, timestamp_basis, revision) VALUES(?, ?, ?, ?, ?, 'provider', ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, 1)`,
		quoteID.String(), request.HouseholdID.String(), base.String(), quote.String(), rate.Canonical(), request.ProviderKey, formatTimestamp(quotedAt), formatTimestamp(fetchedAt), kind, observation.MarketDate, formatTimestamp(fetchedAt), formatTimestamp(observation.ValueEffectiveAt), policy, observation.TimestampBasis); err != nil {
		return persistWrite{}, mapPortfolioWriteError(err, "FX quote")
	}
	slot := kind == observationKindDailyReference
	if slot {
		if err := upsertFXObservationSlotTx(ctx, tx, request.HouseholdID.String(), base.String(), quote.String(), request.ProviderKey, policy, observation.MarketDate, kind, quoteID.String(), fetchedAt); err != nil {
			return persistWrite{}, err
		}
	}
	return persistWrite{persisted: true, newRevision: true, slot: slot, statusReconciled: statusReconciled}, nil
}

func persistInstrumentCoverageTx(ctx context.Context, tx *sql.Tx, request InstrumentHistoryCommit, bindingRevision int, policy string, fetchedAt time.Time, observed map[string]struct{}) (int, error) {
	targetID := request.InstrumentID.String()
	written := 0
	// Verified no-observation is negative cache. Uncertain/truncated batches
	// must not write it even if a caller supplied verified ranges.
	if strings.EqualFold(strings.TrimSpace(request.Status), mappingStatusMapped) {
		for _, rng := range request.VerifiedRanges {
			dates, err := inclusiveDates(rng)
			if err != nil {
				return written, err
			}
			for _, date := range dates {
				if _, ok := observed[date]; ok {
					continue
				}
				changed, err := upsertDayStatusTx(ctx, tx, "instrument", targetID, request.ProviderKey, request.HouseholdID.String(), bindingRevision, policy, date, coverageStatusNoObservation, request.Reason, fetchedAt, request.NextCheckAt, noObservationExpiry(date, fetchedAt))
				if err != nil {
					return written, err
				}
				if changed {
					written++
				}
			}
		}
	}
	for _, rng := range request.PendingRanges {
		dates, err := inclusiveDates(rng)
		if err != nil {
			return written, err
		}
		for _, date := range dates {
			if _, ok := observed[date]; ok {
				continue
			}
			changed, err := upsertDayStatusTx(ctx, tx, "instrument", targetID, request.ProviderKey, request.HouseholdID.String(), bindingRevision, policy, date, coverageStatusPending, request.Reason, fetchedAt, request.NextCheckAt, nil)
			if err != nil {
				return written, err
			}
			if changed {
				written++
			}
		}
	}
	return written, nil
}

func persistFXCoverageTx(ctx context.Context, tx *sql.Tx, request FXHistoryCommit, policy string, fetchedAt time.Time, observed map[string]struct{}) (int, error) {
	targetID := request.BaseCurrency.String() + "/" + request.QuoteCurrency.String()
	written := 0
	if strings.EqualFold(strings.TrimSpace(request.Status), mappingStatusMapped) {
		for _, rng := range request.VerifiedRanges {
			dates, err := inclusiveDates(rng)
			if err != nil {
				return written, err
			}
			for _, date := range dates {
				if _, ok := observed[date]; ok {
					continue
				}
				changed, err := upsertDayStatusTx(ctx, tx, "fx", targetID, request.ProviderKey, request.HouseholdID.String(), 0, policy, date, coverageStatusNoObservation, request.Reason, fetchedAt, request.NextCheckAt, noObservationExpiry(date, fetchedAt))
				if err != nil {
					return written, err
				}
				if changed {
					written++
				}
			}
		}
	}
	for _, rng := range request.PendingRanges {
		dates, err := inclusiveDates(rng)
		if err != nil {
			return written, err
		}
		for _, date := range dates {
			if _, ok := observed[date]; ok {
				continue
			}
			changed, err := upsertDayStatusTx(ctx, tx, "fx", targetID, request.ProviderKey, request.HouseholdID.String(), 0, policy, date, coverageStatusPending, request.Reason, fetchedAt, request.NextCheckAt, nil)
			if err != nil {
				return written, err
			}
			if changed {
				written++
			}
		}
	}
	return written, nil
}

type slotQuote struct {
	id               string
	unitPrice        string
	currency         string
	priceBasis       string
	valueEffectiveAt string
	revision         int
}

func canonicalInstrumentSlotQuoteTx(ctx context.Context, tx *sql.Tx, instrumentID, providerKey string, bindingRevision int, policy, marketDate, kind string) (*slotQuote, error) {
	if kind != observationKindClose {
		return nil, nil
	}
	var quote slotQuote
	err := tx.QueryRowContext(ctx, `
		SELECT q.id, q.unit_price, q.currency, COALESCE(q.price_basis, ''), COALESCE(q.value_effective_at, ''), q.revision
		FROM instrument_observation_slots s
		JOIN instrument_quotes q ON q.id = s.quote_id
		WHERE s.instrument_id = ? AND s.provider_key = ? AND s.binding_revision = ? AND s.source_policy_version = ? AND s.market_date = ? AND s.observation_kind = ?`,
		instrumentID, providerKey, bindingRevision, policy, marketDate, kind).Scan(&quote.id, &quote.unitPrice, &quote.currency, &quote.priceBasis, &quote.valueEffectiveAt, &quote.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &quote, nil
}

func canonicalFXSlotQuoteTx(ctx context.Context, tx *sql.Tx, householdID, base, quote, providerKey, policy, marketDate, kind string) (*slotQuote, error) {
	if kind != observationKindDailyReference {
		return nil, nil
	}
	var found slotQuote
	err := tx.QueryRowContext(ctx, `
		SELECT q.id, q.rate, q.quote_currency, '', COALESCE(q.value_effective_at, ''), q.revision
		FROM fx_observation_slots s
		JOIN fx_quotes q ON q.id = s.quote_id
		WHERE s.household_id = ? AND s.base_currency = ? AND s.quote_currency = ? AND s.provider_key = ? AND s.source_policy_version = ? AND s.market_date = ? AND s.observation_kind = ?`,
		householdID, base, quote, providerKey, policy, marketDate, kind).Scan(&found.id, &found.unitPrice, &found.currency, &found.priceBasis, &found.valueEffectiveAt, &found.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &found, nil
}

func upsertInstrumentObservationSlotTx(ctx context.Context, tx *sql.Tx, instrumentID, providerKey string, bindingRevision int, policy, marketDate, kind, quoteID string, updatedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO instrument_observation_slots(instrument_id, provider_key, binding_revision, source_policy_version, market_date, observation_kind, quote_id, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instrument_id, provider_key, binding_revision, source_policy_version, market_date, observation_kind)
		DO UPDATE SET quote_id = excluded.quote_id, updated_at = excluded.updated_at`,
		instrumentID, providerKey, bindingRevision, policy, marketDate, kind, quoteID, formatTimestamp(updatedAt))
	return err
}

func upsertFXObservationSlotTx(ctx context.Context, tx *sql.Tx, householdID, base, quote, providerKey, policy, marketDate, kind, quoteID string, updatedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO fx_observation_slots(household_id, base_currency, quote_currency, provider_key, source_policy_version, market_date, observation_kind, quote_id, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(household_id, base_currency, quote_currency, provider_key, source_policy_version, market_date, observation_kind)
		DO UPDATE SET quote_id = excluded.quote_id, updated_at = excluded.updated_at`,
		householdID, base, quote, providerKey, policy, marketDate, kind, quoteID, formatTimestamp(updatedAt))
	return err
}

func upsertDayStatusTx(ctx context.Context, tx *sql.Tx, targetType, targetID, providerKey, householdID string, bindingRevision int, policy, effectiveDate, status, reason string, checkedAt time.Time, nextCheckAt *time.Time, expiresAt *time.Time) (bool, error) {
	if strings.TrimSpace(reason) == "" {
		reason = status
	}
	var existingStatus, existingReason sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT status, reason FROM market_data_day_status
		WHERE target_type = ? AND target_id = ? AND provider_key = ? AND household_id = ? AND binding_revision = ? AND source_policy_version = ? AND effective_date = ?`,
		targetType, targetID, providerKey, householdID, bindingRevision, policy, effectiveDate).Scan(&existingStatus, &existingReason)
	if err == nil && existingStatus.String == status && existingReason.String == reason {
		_, err = tx.ExecContext(ctx, `
			UPDATE market_data_day_status
			SET checked_at = ?, next_check_at = ?, expires_at = ?
			WHERE target_type = ? AND target_id = ? AND provider_key = ? AND household_id = ? AND binding_revision = ? AND source_policy_version = ? AND effective_date = ?`,
			formatTimestamp(checkedAt), nullableTimePtr(nextCheckAt), nullableTimePtr(expiresAt),
			targetType, targetID, providerKey, householdID, bindingRevision, policy, effectiveDate)
		return false, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO market_data_day_status(target_type, target_id, provider_key, household_id, binding_revision, source_policy_version, effective_date, status, reason, checked_at, next_check_at, expires_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_type, target_id, provider_key, household_id, binding_revision, source_policy_version, effective_date)
		DO UPDATE SET status = excluded.status, reason = excluded.reason, checked_at = excluded.checked_at, next_check_at = excluded.next_check_at, expires_at = excluded.expires_at`,
		targetType, targetID, providerKey, householdID, bindingRevision, policy, effectiveDate, status, reason, formatTimestamp(checkedAt), nullableTimePtr(nextCheckAt), nullableTimePtr(expiresAt))
	if err != nil {
		return false, err
	}
	return true, nil
}

// clearDayStatusTx removes only the status for the exact canonical identity.
// It intentionally runs in the same transaction as the observation/slot write
// so a pending day cannot survive a successful canonical observation, and a
// different provider, binding, policy, household, or pair is never touched.
func clearDayStatusTx(ctx context.Context, tx *sql.Tx, targetType, targetID, providerKey, householdID string, bindingRevision int, policy, effectiveDate string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		DELETE FROM market_data_day_status
		WHERE target_type = ? AND target_id = ? AND provider_key = ? AND household_id = ? AND binding_revision = ? AND source_policy_version = ? AND effective_date = ?`,
		targetType, targetID, providerKey, householdID, bindingRevision, policy, effectiveDate)
	if err != nil {
		return false, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return removed > 0, nil
}

func noObservationExpiry(effectiveDate string, checkedAt time.Time) *time.Time {
	expiry, err := domain.NoObservationExpiresAt(effectiveDate, checkedAt)
	if err != nil {
		fallback := checkedAt.Add(domain.RecentNoObservationTTL)
		return &fallback
	}
	return &expiry
}

func bumpHistoryInputGenerationTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID, dirtyFrom, dirtyTo string, at time.Time) (int, error) {
	var timezone, startedAt string
	generationAdvanced := false
	if err := tx.QueryRowContext(ctx, `SELECT timezone, started_at FROM history_origins WHERE household_id = ?`, householdID.String()).Scan(&timezone, &startedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	} else {
		if dirtyFrom != "" {
			var err error
			generationAdvanced, err = markHistoryDirtyAndAdvanceGenerationTx(ctx, tx, householdID, dirtyFrom, timezone, at)
			if err != nil {
				return 0, err
			}
		}
		dirtyTo = expandDirtyToLocalBound(dirtyTo, timezone, startedAt, at)
	}
	generationExpression := "input_generation = input_generation + 1"
	if generationAdvanced {
		generationExpression = "input_generation = input_generation"
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE history_snapshot_state
		SET `+generationExpression+`,
		    resolver_policy_version = CASE WHEN resolver_policy_version IS NULL OR resolver_policy_version = '' THEN ? ELSE resolver_policy_version END,
		    dirty_to = CASE WHEN ? = '' THEN dirty_to WHEN dirty_to IS NULL OR dirty_to < ? THEN ? ELSE dirty_to END,
		    updated_at = ?
		WHERE household_id = ?`, domain.MarketDataResolverPolicy, dirtyTo, dirtyTo, dirtyTo, formatTimestamp(at), householdID.String())
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, nil
	}
	return currentInputGenerationTx(ctx, tx, householdID)
}

func currentInputGenerationTx(ctx context.Context, tx *sql.Tx, householdID domain.HouseholdID) (int, error) {
	var generation sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT input_generation FROM history_snapshot_state WHERE household_id = ?`, householdID.String()).Scan(&generation); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return int(generation.Int64), nil
}

func instrumentHistoryDirtyBounds(observations []InstrumentHistoryObservation, ranges ...[]DateSpan) (string, string) {
	dates := make([]string, 0, len(observations))
	for _, observation := range observations {
		dates = append(dates, observation.MarketDate)
	}
	for _, spans := range ranges {
		for _, span := range spans {
			dates = append(dates, span.Start, span.End)
		}
	}
	return marketDateBounds(dates)
}

func fxHistoryDirtyBounds(observations []FXHistoryObservation, ranges ...[]DateSpan) (string, string) {
	dates := make([]string, 0, len(observations))
	for _, observation := range observations {
		dates = append(dates, observation.MarketDate)
	}
	for _, spans := range ranges {
		for _, span := range spans {
			dates = append(dates, span.Start, span.End)
		}
	}
	return marketDateBounds(dates)
}

// marketDateBounds returns the earliest and latest YYYY-MM-DD market-date
// labels. Dirty ranges use these session labels, not ValueEffectiveAt.UTC(),
// so a US close that lands on the next UTC calendar day still dirties the
// market date that household snapshots resolve against.
func marketDateBounds(dates []string) (string, string) {
	earliest, latest := "", ""
	for _, date := range dates {
		date = strings.TrimSpace(date)
		if date == "" {
			continue
		}
		if earliest == "" || date < earliest {
			earliest = date
		}
		if latest == "" || date > latest {
			latest = date
		}
	}
	return earliest, latest
}

// expandDirtyToLocalBound raises dirty_to to the last closed household local
// day so carried-forward snapshots after the latest market date are rebuilt.
func expandDirtyToLocalBound(dirtyTo, timezone, startedAt string, at time.Time) string {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return dirtyTo
	}
	originDate := ""
	if startedAt != "" {
		origin, parseErr := time.Parse(time.RFC3339Nano, startedAt)
		if parseErr != nil {
			origin, parseErr = time.Parse(time.RFC3339, startedAt)
		}
		if parseErr == nil {
			originDate = origin.In(location).Format("2006-01-02")
		}
	}
	lastClosed := lastClosedLocalDate(at, location)
	if lastClosed == "" {
		return dirtyTo
	}
	if originDate != "" && lastClosed < originDate {
		lastClosed = originDate
	}
	if dirtyTo == "" || lastClosed > dirtyTo {
		return lastClosed
	}
	return dirtyTo
}

func lastClosedLocalDate(at time.Time, location *time.Location) string {
	if location == nil || at.IsZero() {
		return ""
	}
	today := at.In(location).Format("2006-01-02")
	parsed, err := time.Parse("2006-01-02", today)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 0, -1).Format("2006-01-02")
}

func nullableEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableTimePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return formatTimestamp(*value)
}

func nullableTimeValue(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return formatTimestamp(value)
}
