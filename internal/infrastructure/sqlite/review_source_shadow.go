package sqlite

import (
	"context"
	"database/sql"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// The caller owns the SAME transaction as the full financial capture/recheck.
// Older verified readonly snapshots have no C1 checkpoint; they retain their
// existing full-source behavior and cannot establish a shadow equivalence.
func readReviewSourceCheckpoint(ctx context.Context, q schemaQuery, id domain.HouseholdID, resolver string) (*domain.ReviewSourceCheckpoint, error) {
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return nil, err
	}
	if version < 20 {
		return nil, nil
	}
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

// WithReviewSourceShadowObserver installs an optional synchronous diagnostic
// sink. It must be concurrency-safe and must not reenter the database: publish
// observations execute while the existing immediate write transaction is held.
// No default telemetry, persistent cache, publication decision or fast path is
// enabled by this hook; observations contain no source values or hashes.
func (r *Repository) WithReviewSourceShadowObserver(observer func(domain.ReviewSourceShadowObservation)) *Repository {
	copy := *r
	copy.reviewSourceShadowObserver = observer
	return &copy
}
