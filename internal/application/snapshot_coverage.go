package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Plan requested coverage from actual rows and the existing durable dirty range,
// rather than using its global completion watermark as proof of coverage.
// This runs inside the same application coordinator as capture/publication.
func (s *Service) ensureSnapshotCoverage(ctx context.Context, householdID domain.HouseholdID, left, right string) error {
	_, generationAware := s.repository.(GenerationAwareSnapshotRepository)
	state, err := s.repository.DailySnapshotState(ctx, householdID)
	if err != nil {
		return err
	}
	start, startErr := time.Parse("2006-01-02", left)
	end, endErr := time.Parse("2006-01-02", right)
	if startErr != nil || endErr != nil || end.Before(start) {
		return &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, householdID, start, end)
	if err != nil {
		return err
	}
	present := make(map[string]domain.DailyValuationSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		present[snapshot.LocalDate] = snapshot
	}
	var gapStart time.Time
	rebuildGap := func(gapEnd time.Time) error {
		for cursor := gapStart; !cursor.After(gapEnd); {
			chunkEnd := cursor.AddDate(0, 0, 30)
			if chunkEnd.After(gapEnd) {
				chunkEnd = gapEnd
			}
			if _, err := s.RebuildHistoricalSnapshots(ctx, cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02")); err != nil {
				return err
			}
			cursor = chunkEnd.AddDate(0, 0, 1)
		}
		gapStart = time.Time{}
		return nil
	}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := day.Format("2006-01-02")
		snapshot, exists := present[key]
		dirty := state.DirtyFrom != nil && key >= *state.DirtyFrom && (state.DirtyTo == nil || key <= *state.DirtyTo)
		// An earlier pending prefix cannot be consumed by this request. Its
		// coarse dirty range can still include later rows already rebuilt from
		// this generation. Reuse that evidence on repeat reads; generation zero
		// and repositories without generation guards retain conservative rebuilds.
		if dirty && generationAware && state.InputGeneration > 0 && *state.DirtyFrom < left && snapshot.InputGeneration == state.InputGeneration {
			dirty = false
		}
		if !exists || dirty || snapshotHashNeedsRebuild(snapshot.ContentHash) || snapshot.ResolverPolicyVersion != domain.MarketDataResolverPolicy {
			if gapStart.IsZero() {
				gapStart = day
			}
		} else if !gapStart.IsZero() {
			if err := rebuildGap(day.AddDate(0, 0, -1)); err != nil {
				return err
			}
		}
	}
	if !gapStart.IsZero() {
		if err := rebuildGap(end); err != nil {
			return err
		}
	}
	// Even a no-op or a rebuild without a dirty prefix must reject a plan made
	// before a concurrent source revision. Batches retain their own generation
	// guards; this also catches revisions between batches or coverage reads.
	current, err := s.repository.DailySnapshotState(ctx, householdID)
	if err != nil {
		return err
	}
	if current.InputGeneration != state.InputGeneration {
		return &domain.Error{Code: domain.ErrConflict, Field: "inputGeneration", Message: "snapshot input generation changed during rebuild"}
	}
	// The existing per-day save consumes only a matching dirty prefix and keeps
	// the watermark monotonic. Its range-completion operation assumes every day
	// from dirty_from through target was rebuilt: only invoke it when this request
	// actually covered that prefix. A later request must retain earlier pending
	// days, even if its own snapshots now reflect the same corrected facts.
	if state.DirtyFrom != nil && *state.DirtyFrom >= left && *state.DirtyFrom <= right {
		if generationRepo, ok := s.repository.(GenerationAwareSnapshotRepository); ok {
			return generationRepo.CompleteDailySnapshotRangeAtGeneration(ctx, householdID, right, s.clock(), state.InputGeneration)
		}
		return s.repository.CompleteDailySnapshotRange(ctx, householdID, right, s.clock())
	}
	return nil
}
