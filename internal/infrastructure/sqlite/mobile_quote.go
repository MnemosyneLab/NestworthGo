package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const mobileQuotePrefix = "mobile.quote-receipt.v1."

func mobileQuoteKey(hh domain.HouseholdID, id string) string {
	return mobileQuotePrefix + hh.String() + "." + id
}

func (r *Repository) LookupMobileQuote(ctx context.Context, hh domain.HouseholdID, id string) (*domain.MobileQuoteReceipt, error) {
	var raw []byte
	err := r.database.SQL.QueryRowContext(ctx, `SELECT value FROM app_configuration WHERE key=?`, mobileQuoteKey(hh, id)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt domain.MobileQuoteReceipt
	if err := jsoncodec.Unmarshal(raw, &receipt); err != nil {
		return nil, storedIntegrity("quote", "invalid mobile receipt")
	}
	if receipt.HouseholdID != hh || receipt.OperationID != id || receipt.Validate() != nil {
		return nil, storedIntegrity("quote", "inconsistent mobile receipt")
	}
	if err := verifyMobileQuoteFact(ctx, r.database.SQL, receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *Repository) CommitMobileQuote(ctx context.Context, m domain.MobileQuoteReceipt, quote *domain.InstrumentQuote, fx *domain.FXQuote) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if (quote == nil) == (fx == nil) {
		return storedIntegrity("quote", "one quote is required")
	}
	raw, err := jsoncodec.Marshal(m)
	if err != nil {
		return err
	}
	return r.database.WithTx(ctx, func(tx *sql.Tx) error {
		if quote != nil {
			if m.Kind != "instrument" || m.InstrumentID != quote.InstrumentID || m.FactID != quote.ID.String() || m.Value != quote.UnitPrice.Canonical() || m.Currency != quote.Currency.String() || !m.QuotedAt.Equal(quote.QuotedAt) || quote.SourceKind != domain.QuoteSourceManual {
				return storedIntegrity("quote", "receipt does not match quote")
			}
			var hh string
			if err := tx.QueryRowContext(ctx, `SELECT household_id FROM instruments WHERE id=?`, quote.InstrumentID.String()).Scan(&hh); err != nil {
				return err
			}
			if hh != m.HouseholdID.String() {
				return storedIntegrity("quote", "quote ownership mismatch")
			}
			if err := appendInstrumentQuoteTx(ctx, tx, *quote); err != nil {
				return err
			}
			if err := markQuoteHistoryDirtyTx(ctx, tx, quote.InstrumentID, quote.QuotedAt, quote.CreatedAt); err != nil {
				return err
			}
		} else {
			if m.Kind != "fx" || m.HouseholdID != fx.HouseholdID || m.FactID != fx.ID.String() || m.Value != fx.Rate.Canonical() || m.Currency != fx.BaseCurrency.String() || m.QuoteCurrency != fx.QuoteCurrency.String() || !m.QuotedAt.Equal(fx.QuotedAt) || fx.SourceKind != domain.QuoteSourceManual {
				return storedIntegrity("quote", "receipt does not match FX quote")
			}
			if err := appendFXQuoteTx(ctx, tx, *fx); err != nil {
				return err
			}
			if err := markFXQuoteHistoryDirtyTx(ctx, tx, fx.HouseholdID, fx.QuotedAt, fx.CreatedAt); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO app_configuration(key,value) VALUES(?,?)`, mobileQuoteKey(m.HouseholdID, m.OperationID), string(raw))
		return err
	})
}

func verifyMobileQuoteFact(ctx context.Context, query schemaQuery, m domain.MobileQuoteReceipt) error {
	var count int
	var err error
	if m.Kind == "instrument" {
		err = query.QueryRowContext(ctx, `SELECT COUNT(*) FROM instrument_quotes q JOIN instruments i ON i.id=q.instrument_id WHERE q.id=? AND i.household_id=? AND q.instrument_id=? AND q.unit_price=? AND q.currency=? AND q.source_kind='manual' AND q.quoted_at=? AND q.created_at=?`, m.FactID, m.HouseholdID.String(), m.InstrumentID.String(), m.Value, m.Currency, formatTimestamp(m.QuotedAt), formatTimestamp(m.RecordedAt)).Scan(&count)
	} else {
		err = query.QueryRowContext(ctx, `SELECT COUNT(*) FROM fx_quotes WHERE id=? AND household_id=? AND rate=? AND base_currency=? AND quote_currency=? AND source_kind='manual' AND quoted_at=? AND created_at=?`, m.FactID, m.HouseholdID.String(), m.Value, m.Currency, m.QuoteCurrency, formatTimestamp(m.QuotedAt), formatTimestamp(m.RecordedAt)).Scan(&count)
	}
	if err != nil {
		return err
	}
	if count != 1 {
		return storedIntegrity("quote", "mobile receipt and observation disagree")
	}
	return nil
}

// Private receipts follow the existing configuration-backed business receipt
// pattern. Startup and backup verification validate them without a schema bump.
func verifyMobileQuoteReceipts(ctx context.Context, query schemaQuery) error {
	rows, err := query.QueryContext(ctx, `SELECT key,value FROM app_configuration WHERE substr(key,1,?)=?`, len(mobileQuotePrefix), mobileQuotePrefix)
	if err != nil {
		return err
	}
	var receipts []domain.MobileQuoteReceipt
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return err
		}
		var m domain.MobileQuoteReceipt
		if jsoncodec.Unmarshal(raw, &m) != nil || m.Validate() != nil || key != mobileQuoteKey(m.HouseholdID, m.OperationID) {
			rows.Close()
			return storedIntegrity("quote", "invalid mobile quote receipt")
		}
		receipts = append(receipts, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, m := range receipts {
		if err := verifyMobileQuoteFact(ctx, query, m); err != nil {
			return err
		}
	}
	return nil
}
