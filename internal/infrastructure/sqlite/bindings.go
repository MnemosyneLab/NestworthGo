package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func syncInstrumentBindingFromInstrumentTx(ctx context.Context, tx *sql.Tx, instrument domain.Instrument) error {
	if instrument.ProviderKey == nil || instrument.ProviderSymbol == nil {
		return nil
	}
	market := ""
	if instrument.MarketCode != nil {
		market = *instrument.MarketCode
	}
	_, changed, err := syncInstrumentProviderBindingTx(ctx, tx, instrument.ID.String(), *instrument.ProviderKey, *instrument.ProviderSymbol, market, instrument.QuoteCurrency.String(), instrument.UpdatedAt)
	if err != nil || !changed {
		return err
	}
	return markInstrumentBindingHistoryDirtyTx(ctx, tx, instrument.ID, instrument.UpdatedAt)
}

func latestInstrumentObservationKind(source domain.QuoteSourceKind) string {
	if source == domain.QuoteSourceManual {
		return "manual"
	}
	return "realtime"
}

func latestFXObservationKind(source domain.QuoteSourceKind) string {
	if source == domain.QuoteSourceManual {
		return "manual"
	}
	return "latest"
}

func syncInstrumentProviderBindingTx(ctx context.Context, tx *sql.Tx, instrumentID, providerKey, providerSymbol, market, currency string, at time.Time) (int, bool, error) {
	providerKey = strings.TrimSpace(providerKey)
	providerSymbol = strings.TrimSpace(providerSymbol)
	if providerKey == "" || providerSymbol == "" {
		return 0, false, nil
	}
	var revision int
	var currentSymbol string
	err := tx.QueryRowContext(ctx, `SELECT binding_revision, provider_symbol FROM instrument_provider_bindings WHERE instrument_id = ? AND provider_key = ?`, instrumentID, providerKey).Scan(&revision, &currentSymbol)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_bindings(instrument_id, provider_key, provider_symbol, market, currency, enabled, created_at, updated_at, binding_revision, effective_from) VALUES(?, ?, ?, ?, ?, 1, ?, ?, 1, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, 1, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, false, err
		}
		return 1, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	// Binding revisions identify a provider-symbol revision. Market/currency
	// metadata may be learned later (for example, on the first history fetch)
	// without creating a second economic route for the same symbol.
	if currentSymbol == providerSymbol {
		return revision, false, nil
	}
	next := revision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE instrument_provider_bindings SET provider_symbol = ?, market = ?, currency = ?, binding_revision = ?, effective_from = ?, updated_at = ? WHERE instrument_id = ? AND provider_key = ?`, providerSymbol, nullableEmpty(market), nullableEmpty(currency), next, formatTimestamp(at), formatTimestamp(at), instrumentID, providerKey); err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, next, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
		return 0, false, err
	}
	return next, true, nil
}

func markInstrumentBindingHistoryDirtyTx(ctx context.Context, tx *sql.Tx, instrumentID domain.InstrumentID, at time.Time) error {
	var householdID, timezone string
	if err := tx.QueryRowContext(ctx, `SELECT i.household_id, o.timezone FROM instruments i JOIN history_origins o ON o.household_id = i.household_id WHERE i.id = ?`, instrumentID.String()).Scan(&householdID, &timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	parsedHouseholdID, err := domain.ParseHouseholdID(householdID)
	if err != nil {
		return err
	}
	return markHistoryDirtyTx(ctx, tx, parsedHouseholdID, observationEffectiveDate(at, timezone), timezone, at)
}

func listInstrumentProviderBindingRevisionsQuery(ctx context.Context, query queryer, householdID domain.HouseholdID) ([]domain.InstrumentProviderBindingRevision, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT b.instrument_id, b.provider_key, b.provider_symbol, b.market, b.currency,
		       b.enabled, b.binding_revision, b.effective_from, b.created_at
		FROM instrument_provider_binding_revisions b
		JOIN instruments i ON i.id = b.instrument_id
		WHERE i.household_id = ?
		ORDER BY b.instrument_id, b.effective_from, b.created_at, b.binding_revision, b.provider_key`, householdID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.InstrumentProviderBindingRevision, 0)
	for rows.Next() {
		var instrumentID, providerKey, providerSymbol string
		var market, currency sql.NullString
		var enabled, revision int
		var effectiveFrom, createdAt string
		if err := rows.Scan(&instrumentID, &providerKey, &providerSymbol, &market, &currency, &enabled, &revision, &effectiveFrom, &createdAt); err != nil {
			return nil, err
		}
		parsedInstrumentID, err := domain.ParseInstrumentID(instrumentID)
		if err != nil {
			return nil, err
		}
		parsedEffectiveFrom, err := time.Parse(time.RFC3339Nano, effectiveFrom)
		if err != nil {
			return nil, &domain.Error{Code: domain.ErrIntegrity, Field: "effectiveFrom", Message: "stored provider binding timestamp is invalid"}
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, &domain.Error{Code: domain.ErrIntegrity, Field: "createdAt", Message: "stored provider binding timestamp is invalid"}
		}
		parsedCurrency := domain.CurrencyCode(strings.TrimSpace(currency.String))
		if parsedCurrency != "" {
			parsedCurrency, err = domain.ParseCurrency(parsedCurrency.String())
			if err != nil {
				return nil, err
			}
		}
		result = append(result, domain.InstrumentProviderBindingRevision{
			InstrumentID: parsedInstrumentID, ProviderKey: strings.TrimSpace(providerKey), ProviderSymbol: strings.TrimSpace(providerSymbol),
			Market: strings.TrimSpace(market.String), Currency: parsedCurrency, Enabled: enabled != 0, BindingRevision: revision,
			EffectiveFrom: parsedEffectiveFrom.UTC(), CreatedAt: parsedCreatedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}
