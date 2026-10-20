package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// LoadAnalysisQuoteBatch preserves all historical/manual/provider candidates.
// Ownership checks and all chunks share one read transaction, including agent
// withdrawal visibility. The rest of analysis retains its existing read boundaries.
func (r *Repository) LoadAnalysisQuoteBatch(ctx context.Context, householdID domain.HouseholdID, instrumentIDs []domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	quotes, err := loadAnalysisQuoteBatch(ctx, tx, householdID, instrumentIDs)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return quotes, nil
}

func loadAnalysisQuoteBatch(ctx context.Context, query queryer, householdID domain.HouseholdID, instrumentIDs []domain.InstrumentID) ([]domain.InstrumentQuote, error) {
	if err := ensureHousehold(ctx, query, householdID); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(instrumentIDs))
	byInstrument := make(map[domain.InstrumentID][]domain.InstrumentQuote, len(instrumentIDs))
	for _, id := range instrumentIDs {
		if _, exists := byInstrument[id]; !exists {
			ids = append(ids, id.String())
			byInstrument[id] = nil
		}
	}
	// Leave room for the ownership parameter below SQLite's traditional 999
	// variable limit. Bound SQL parameters, not history dates or candidate count.
	const batchSize = 900
	for start := 0; start < len(ids); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk := ids[start:min(start+batchSize, len(ids))]
		clause, args := sqlInArgs(chunk)
		rows, err := query.QueryContext(ctx, `SELECT id FROM instruments WHERE household_id = ? AND id IN (`+clause+`)`, append([]any{householdID.String()}, args...)...)
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			count++
		}
		readErr := rows.Err()
		closeErr := rows.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if count != len(chunk) {
			return nil, &domain.Error{Code: domain.ErrNotFound, Message: "instrument was not found in household"}
		}
		// Keep the same projection, withdrawal exclusion, scanner and per-ID
		// ordering as ListInstrumentQuotes. Grouping restores caller ID order.
		rows, err = query.QueryContext(ctx, `SELECT id, instrument_id, unit_price, currency, source_kind, source_key, quoted_at, created_at, delayed, observation_kind, effective_date, provider_timestamp, fetched_at, value_effective_at, binding_revision, source_policy_version, price_basis, timestamp_basis, revision, supersedes_quote_id, split_factor, dividend_cash, conversion_json FROM instrument_quotes WHERE instrument_id IN (`+clause+`) AND NOT EXISTS (SELECT 1 FROM agent_quote_records withdrawn WHERE withdrawn.target_type = 'instrument' AND withdrawn.target_quote_id = instrument_quotes.id) ORDER BY instrument_id, quoted_at DESC, created_at DESC, id DESC`, args...)
		if err != nil {
			return nil, err
		}
		quotes, readErr := scanInstrumentQuotes(rows)
		closeErr = rows.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		for _, quote := range quotes {
			byInstrument[quote.InstrumentID] = append(byInstrument[quote.InstrumentID], quote)
		}
	}
	result := make([]domain.InstrumentQuote, 0)
	for _, id := range instrumentIDs {
		result = append(result, byInstrument[id]...)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// LoadAnalysisAccountSnapshots narrows only activity-free households. Transfers,
// corrections and cost origins therefore always retain the existing full path.
// Even future activities reject admission; this deliberately favors simplicity.
func (r *Repository) LoadAnalysisAccountSnapshots(ctx context.Context, householdID domain.HouseholdID, since, until time.Time, selected domain.AccountID, accounts []domain.AccountID) ([]domain.DailyValuationSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	known := make(map[domain.AccountID]bool, len(accounts))
	for _, id := range accounts {
		known[id] = true
	}
	var hasActivities bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM activities WHERE household_id = ?)`, householdID.String()).Scan(&hasActivities); err != nil {
		return nil, err
	}
	if hasActivities || !known[selected] {
		selected = ""
	}
	result, err := listDailyValuationSnapshotsForAccountQuery(ctx, tx, householdID, since, until, selected, known)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
