package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Acknowledgement owns only the handling fields on an episode. Reconciliation
// may replace evidence concurrently but must never replay an old ack value.
func (r *Repository) AcknowledgeReviewEpisodeV2(ctx context.Context, m domain.ReviewAcknowledgeMutation) (domain.ReviewAcknowledgement, error) {
	var empty domain.ReviewAcknowledgement
	if _, err := domain.ParseHouseholdID(m.HouseholdID.String()); err != nil {
		return empty, err
	}
	if _, err := domain.ParseReviewEpisodeID(string(m.EpisodeID)); err != nil {
		return empty, err
	}
	if _, err := domain.ParseMutationID(string(m.MutationID)); err != nil {
		return empty, err
	}
	if m.ExpectedRevision < 1 {
		return empty, &domain.Error{Code: domain.ErrValidation, Field: "expectedRevision", Message: "invalid episode revision"}
	}
	request, err := jsoncodec.Marshal(struct {
		HouseholdID      domain.HouseholdID
		EpisodeID        domain.ReviewEpisodeID
		MutationID       domain.MutationID
		ExpectedRevision int
	}{m.HouseholdID, m.EpisodeID, m.MutationID, m.ExpectedRevision})
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256(request)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var previousHash, previousJSON string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,result_json FROM investment_review_mutation_receipts WHERE household_id=? AND mutation_id=?`, m.HouseholdID.String(), string(m.MutationID)).Scan(&previousHash, &previousJSON)
	if err == nil {
		if previousHash != requestHash {
			return empty, reviewRuleConflict("mutation ID was already used for a different request")
		}
		var previous domain.ReviewAcknowledgement
		if err := jsoncodec.Unmarshal([]byte(previousJSON), &previous); err != nil {
			return empty, err
		}
		r.database.reviewInitialPublicationAllowed.Store(false)
		if err := tx.Commit(); err != nil {
			return empty, err
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	integrity, err := r.beforeCertifiedHandling(ctx, tx, m.HouseholdID)
	if err != nil {
		return empty, err
	}
	var revision int
	var acknowledged, resolved sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT e.revision,e.acknowledged_at,e.resolved_at FROM investment_review_episodes e JOIN investment_review_rules r ON r.id=e.rule_id WHERE r.household_id=? AND e.id=?`, m.HouseholdID.String(), string(m.EpisodeID)).Scan(&revision, &acknowledged, &resolved)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, reviewRuleConflict("review episode no longer exists")
	}
	if err != nil {
		return empty, err
	}
	if revision != m.ExpectedRevision || resolved.Valid {
		return empty, reviewRuleConflict("episode changed; reload before acknowledging")
	}
	result := domain.ReviewAcknowledgement{EpisodeID: m.EpisodeID, Revision: revision}
	if acknowledged.Valid {
		result.AcknowledgedAt, err = time.Parse(time.RFC3339Nano, acknowledged.String)
		if err != nil {
			return empty, err
		}
	} else {
		result.AcknowledgedAt = m.At.UTC()
		result.Revision++
		at := result.AcknowledgedAt.Format(time.RFC3339Nano)
		changed, err := tx.ExecContext(ctx, `UPDATE investment_review_episodes SET acknowledged_at=?,revision=revision+1 WHERE id=? AND revision=? AND resolved_at IS NULL AND acknowledged_at IS NULL`, at, string(m.EpisodeID), m.ExpectedRevision)
		if err != nil {
			return empty, err
		}
		count, err := changed.RowsAffected()
		if err != nil {
			return empty, err
		}
		if count != 1 {
			return empty, reviewRuleConflict("episode changed; reload before acknowledging")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE investment_review_control SET review_state_revision=review_state_revision+1 WHERE household_id=?`, m.HouseholdID.String()); err != nil {
			return empty, err
		}
	}
	encoded, err := jsoncodec.Marshal(result)
	if err != nil {
		return empty, err
	}
	if !acknowledged.Valid {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_episode_events(id,episode_id,event_key,kind,payload_json,created_at) VALUES(?,?,?,?,?,?)`, string(domain.NewReviewEpisodeID()), string(m.EpisodeID), "ack/"+string(m.EpisodeID), "acknowledged", string(encoded), result.AcknowledgedAt.Format(time.RFC3339Nano)); err != nil {
			return empty, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_review_mutation_receipts(household_id,mutation_id,request_hash,result_json,created_at) VALUES(?,?,?,?,?)`, m.HouseholdID.String(), string(m.MutationID), requestHash, string(encoded), m.At.UTC().Format(time.RFC3339Nano)); err != nil {
		return empty, err
	}
	if err := r.afterCertifiedHandling(ctx, tx, m.HouseholdID, integrity); err != nil {
		return empty, err
	}
	r.database.reviewInitialPublicationAllowed.Store(false)
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return result, nil
}
