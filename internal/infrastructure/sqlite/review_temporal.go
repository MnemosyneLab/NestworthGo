package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type reviewTemporalClock struct {
	mu         sync.Mutex
	session    string
	generation uint64
	last       time.Time
}

// Provider selection changes invalidate all prior temporal contexts without
// pretending that wall time moved backwards or touching financial source rows.
func (r *Repository) InvalidateReviewProviderSelection() {
	c := &r.database.reviewClock
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = uuid.NewString()
	c.generation = 0
}

func (r *Repository) ObserveReviewClock(now time.Time) {
	r.database.reviewClock.observe(now)
}

// Shared by repository copies using this DB; exact observed wall time is kept,
// never max(now,last). A rollback invalidates every outstanding older stage.
func (c *reviewTemporalClock) observe(now time.Time) (string, bool) {
	// time.Now carries a monotonic reading. Wall-clock rollback must not be
	// hidden by Time.Before preferring that still-increasing monotonic value.
	now = now.UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == "" {
		c.session = uuid.NewString()
	}
	rollback := !c.last.IsZero() && now.Before(c.last)
	if rollback {
		if c.generation == ^uint64(0) {
			c.session = uuid.NewString()
			c.generation = 0
		} else {
			c.generation++
		}
	}
	c.last = now
	return c.session + "/" + strconv.FormatUint(c.generation, 10), rollback
}

// Same snapshot as capture. Economic activity inclusion and current position
// use captureNow; inspect all future activities, including replaced/reversed
// events and transfer legs. Latest main's quote readers already return uncut
// history: quoted_at fences are deliberately conservative early invalidations.
// State/ownership/cost/preference/provider/market selection uses fixed closed-day
// cutoffs; the complete exclusion proof is documented with the C2 evidence.
func buildReviewTemporal(ctx context.Context, q schemaQuery, capture domain.ReviewProjectionCapture, epoch string) (*domain.ReviewTemporalCertificate, error) {
	certificate := &domain.ReviewTemporalCertificate{EvaluatedAt: capture.SourceCutoff.UTC(), PolicyVersion: domain.ReviewTemporalPolicyVersion, DependencyScope: domain.ReviewTemporalDependencyScope, ClockEpoch: epoch, Source: capture.SourceRevision, ConfigRevision: capture.Control.ConfigRevision, StateRevision: capture.Control.ReviewStateRevision, RuleRevisions: map[domain.ReviewRuleID]int{}}
	bound := func(at time.Time) {
		if at.After(capture.SourceCutoff) && (certificate.NextTransitionAt.IsZero() || at.Before(certificate.NextTransitionAt)) {
			certificate.NextTransitionAt = at.UTC()
		}
	}
	calendars := map[string]bool{capture.Historical.History.Origin.Timezone: true}
	for _, rule := range capture.Rules {
		calendars[rule.Spec.Schedule.Timezone] = true
		certificate.RuleRevisions[rule.ID] = rule.RowRevision
	}
	for _, suppression := range capture.Suppressions {
		calendars[suppression.Timezone] = true
	}
	for timezone := range calendars {
		next, err := domain.ReviewNextCalendarBoundary(capture.SourceCutoff, timezone)
		if err != nil {
			return nil, err
		}
		bound(next)
	}
	bound(capture.Historical.History.Origin.StartedAt)
	for _, holding := range capture.Historical.History.Portfolio.Holdings {
		bound(holding.CreatedAt)
	}
	queries := []string{
		`SELECT effective_at FROM activities WHERE household_id=?`,
		`SELECT q.quoted_at FROM instrument_quotes q JOIN instruments i ON i.id=q.instrument_id WHERE i.household_id=?`,
		`SELECT quoted_at FROM fx_quotes WHERE household_id=?`,
	}
	for _, statement := range queries {
		rows, err := q.QueryContext(ctx, statement, capture.HouseholdID.String())
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			if err = ctx.Err(); err != nil {
				break
			}
			var raw string
			if err = rows.Scan(&raw); err != nil {
				break
			}
			var at time.Time
			at, err = time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				// No empty result or malformed timestamp can mean infinity.
				err = storedIntegrity("time", "review temporal source is invalid")
				break
			}
			bound(at)
		}
		err = errors.Join(err, rows.Err(), rows.Close())
		if err != nil {
			return nil, err
		}
	}
	if !certificate.Covers(capture.SourceCutoff) {
		return nil, storedIntegrity("time", "review temporal interval is incomplete")
	}
	return certificate, nil
}

func reviewTemporalDigest(certificate *domain.ReviewTemporalCertificate) string {
	if certificate == nil {
		return ""
	}
	raw, _ := json.Marshal(certificate)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// Pure DB read plus the shared clock observation. It never refreshes financial
// data or rewrites the old publication. A changed status expires page cursors.
func (r *Repository) reviewTemporalReadStatus(ctx context.Context, q schemaQuery, id domain.HouseholdID, control *domain.ReviewProjectionControl, now time.Time, connectionID uint64) error {
	epoch, rollback := r.database.reviewClock.observe(now)
	certificate, err := readReviewTemporal(ctx, q, id)
	if err != nil {
		return err
	}
	if control.ProjectionStatus != "complete" {
		return nil
	}
	fresh := !rollback && certificate != nil && certificate.ClockEpoch == epoch && certificate.Covers(now) && certificate.Source != nil
	if fresh {
		current, err := readReviewSourceCheckpoint(ctx, q, id, certificate.Source.Token.ResolverPolicyVersion)
		if err != nil {
			return err
		}
		fresh = current != nil && current.Valid() && current.Token == certificate.Source.Token && certificate.Source.Token.Revision >= current.JournalFloor
	}
	if fresh && r.CertifiedReviewEnabled() && certificate.NoChangeCompletenessVersion == domain.ReviewCertifiedCompletenessVersion {
		observed, err := r.certifiedConnectionCheckpoint(ctx, q, connectionID)
		if err != nil {
			return err
		}
		_, tracking, err := r.verifiedCertifiedTracking(ctx, q)
		if err != nil {
			return err
		}
		fresh = connectionID != 0 && tracking && r.validReviewMAC(certificate) && !certificate.CertifiedNeedsReconcile && observed == certificate.CertifiedConnection && certificate.ConfigRevision == control.ConfigRevision && certificate.StateRevision == control.ReviewStateRevision
	}
	if !fresh {
		control.ProjectionStatus = "stale"
		// Distinguish a rollback epoch even if both reads are already stale.
		control.PublishedTemporalRevision += "/" + epoch
	}
	return nil
}

func readReviewTemporal(ctx context.Context, q schemaQuery, id domain.HouseholdID) (*domain.ReviewTemporalCertificate, error) {
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return nil, err
	}
	if version < 21 {
		return nil, nil
	}
	var raw string
	err := q.QueryRowContext(ctx, `SELECT certificate_json FROM review_temporal_checkpoints WHERE household_id=?`, id.String()).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var certificate domain.ReviewTemporalCertificate
	if err := json.Unmarshal([]byte(raw), &certificate); err != nil {
		return nil, fmt.Errorf("invalid review temporal checkpoint: %w", err)
	}
	return &certificate, nil
}

func persistReviewTemporal(ctx context.Context, tx *sql.Tx, id domain.HouseholdID, certificate domain.ReviewTemporalCertificate, asOf time.Time) error {
	certificate.EvaluatedAt = asOf.UTC()
	raw, err := json.Marshal(certificate)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO review_temporal_checkpoints(household_id,evaluated_at,next_transition_at,certificate_json) VALUES(?,?,?,?) ON CONFLICT(household_id) DO UPDATE SET evaluated_at=excluded.evaluated_at,next_transition_at=excluded.next_transition_at,certificate_json=excluded.certificate_json WHERE certificate_json<>excluded.certificate_json`, id.String(), asOf.UTC().Format(time.RFC3339Nano), certificate.NextTransitionAt.UTC().Format(time.RFC3339Nano), string(raw))
	return err
}
