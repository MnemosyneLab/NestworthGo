package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// LoadReviewHistoricalBatch captures every daily review input from a single
// SQLite snapshot. Pure replay happens after this read transaction commits.
func (r *Repository) LoadReviewHistoricalBatch(ctx context.Context, householdID domain.HouseholdID, cutoff time.Time) (domain.ReviewHistoricalBatch, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	defer tx.Rollback()
	batch, err := loadReviewHistoricalBatchQuery(ctx, tx, householdID, cutoff)
	if err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	return batch, nil
}

// Caller owns the transaction. Reusing this helper in the publication
// transaction checks the authoritative facts rather than a review-only
// generation that may not have observed a recent ledger or quote change.
func loadReviewHistoricalBatchQuery(ctx context.Context, query queryer, householdID domain.HouseholdID, cutoff time.Time) (domain.ReviewHistoricalBatch, error) {
	history, err := loadHistoricalSnapshotBatchQuery(ctx, query, householdID, cutoff)
	if err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	gain := domain.GainSnapshot{CostEvents: make(map[domain.HoldingID][]domain.CostBasisEvent), StartingCosts: make(map[domain.HoldingID]*domain.UnitPrice)}
	for _, holding := range history.Portfolio.Holdings {
		gain.CostEvents[holding.ID] = nil
	}
	if err := readGainCosts(ctx, query, householdID, &gain); err != nil {
		return domain.ReviewHistoricalBatch{}, err
	}
	return domain.ReviewHistoricalBatch{History: history, CostEvents: gain.CostEvents}, nil
}
