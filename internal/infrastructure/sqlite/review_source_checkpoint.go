package sqlite

import (
	"context"
	"database/sql"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// The caller owns the SAME transaction as the full financial capture/recheck.
// Missing or unknown metadata in supported snapshots retains full-source
// behavior and cannot establish a shadow equivalence.
func readReviewSourceCheckpoint(ctx context.Context, q schemaQuery, id domain.HouseholdID, resolver string) (*domain.ReviewSourceCheckpoint, error) {
	checkpoint := &domain.ReviewSourceCheckpoint{Token: domain.SourceRevisionToken{HouseholdID: id, ResolverPolicyVersion: resolver, EvaluatorVersion: domain.ReviewEvaluator, MetricPolicyVersion: domain.ReviewMetricPolicyVersion}}
	err := q.QueryRowContext(ctx, `SELECT revision,journal_floor,tracking_epoch,tracking_version FROM financial_source_revision WHERE household_id=?`, id.String()).Scan(&checkpoint.Token.Revision, &checkpoint.JournalFloor, &checkpoint.Token.Epoch, &checkpoint.Token.TrackingVersion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !checkpoint.Token.Valid() || checkpoint.Token.TrackingVersion != financialTrackingVersion || checkpoint.JournalFloor < 0 || checkpoint.JournalFloor > checkpoint.Token.Revision {
		// Unknown metadata/policy is not a certificate. C2 keeps the original
		// full-source algorithm available; the existing startup verifier still
		// rejects invalid current storage before ordinary business writes.
		return nil, nil
	}
	return checkpoint, nil
}
