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
	_, err := syncInstrumentProviderBindingTx(ctx, tx, instrument.ID.String(), *instrument.ProviderKey, *instrument.ProviderSymbol, market, instrument.QuoteCurrency.String(), instrument.UpdatedAt)
	return err
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

func syncInstrumentProviderBindingTx(ctx context.Context, tx *sql.Tx, instrumentID, providerKey, providerSymbol, market, currency string, at time.Time) (int, error) {
	providerKey = strings.TrimSpace(providerKey)
	providerSymbol = strings.TrimSpace(providerSymbol)
	if providerKey == "" || providerSymbol == "" {
		return 0, nil
	}
	var revision int
	var currentSymbol string
	err := tx.QueryRowContext(ctx, `SELECT binding_revision, provider_symbol FROM instrument_provider_bindings WHERE instrument_id = ? AND provider_key = ?`, instrumentID, providerKey).Scan(&revision, &currentSymbol)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_bindings(instrument_id, provider_key, provider_symbol, market, currency, enabled, created_at, updated_at, binding_revision, effective_from) VALUES(?, ?, ?, ?, ?, 1, ?, ?, 1, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, 1, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
			return 0, err
		}
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if currentSymbol == providerSymbol {
		return revision, nil
	}
	next := revision + 1
	if _, err := tx.ExecContext(ctx, `UPDATE instrument_provider_bindings SET provider_symbol = ?, market = ?, currency = ?, binding_revision = ?, effective_from = ?, updated_at = ? WHERE instrument_id = ? AND provider_key = ?`, providerSymbol, nullableEmpty(market), nullableEmpty(currency), next, formatTimestamp(at), formatTimestamp(at), instrumentID, providerKey); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_provider_binding_revisions(instrument_id, provider_key, binding_revision, provider_symbol, market, currency, enabled, effective_from, created_at) VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?)`, instrumentID, providerKey, next, providerSymbol, nullableEmpty(market), nullableEmpty(currency), formatTimestamp(at), formatTimestamp(at)); err != nil {
		return 0, err
	}
	return next, nil
}
