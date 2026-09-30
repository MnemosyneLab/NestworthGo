package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Recalculation can legitimately restore an earlier value/timestamp. Deduplicate
// against the current version, rather than any quote that ever had that value.
func appendConvertedInstrumentQuoteTx(ctx context.Context, tx *sql.Tx, quote domain.InstrumentQuote, rawAt time.Time) (bool, error) {
	var id, value, quotedAt, evidence string
	var delayed, revision int
	err := tx.QueryRowContext(ctx, `SELECT q.id, q.unit_price, q.quoted_at, q.conversion_json, q.delayed, COALESCE(q.revision, 1)
		FROM instrument_quotes q WHERE q.instrument_id = ? AND q.currency = ? AND q.source_kind = ? AND q.source_key = ?
		AND COALESCE(q.observation_kind, '') <> 'close' AND (`+metalRawQuoteKeySQL("q")+`) = ?
		ORDER BY q.revision DESC, q.created_at DESC, q.id DESC LIMIT 1`,
		quote.InstrumentID.String(), quote.Currency.String(), string(quote.SourceKind), quote.SourceKey, rawAt.Format(time.RFC3339Nano)).Scan(&id, &value, &quotedAt, &evidence, &delayed, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && value == quote.UnitPrice.Canonical() && quotedAt == formatTimestamp(quote.QuotedAt) && evidence == quote.ConversionJSON && delayed == boolValue(quote.Delayed) {
		_, err = tx.ExecContext(ctx, `UPDATE instrument_quotes SET fetched_at = ? WHERE id = ?`, formatTimestamp(quote.CreatedAt), id)
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO instrument_quotes(id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, fetched_at, revision, conversion_json)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		quote.ID.String(), quote.InstrumentID.String(), quote.UnitPrice.Canonical(), quote.Currency.String(), string(quote.SourceKind), quote.SourceKey,
		formatTimestamp(quote.QuotedAt), formatTimestamp(quote.CreatedAt), boolValue(quote.Delayed), latestInstrumentObservationKind(quote.SourceKind),
		formatTimestamp(quote.CreatedAt), revision+1, quote.ConversionJSON)
	return err == nil, mapPortfolioWriteError(err, "converted instrument quote")
}
