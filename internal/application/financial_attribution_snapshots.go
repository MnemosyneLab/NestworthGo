package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Keep the existing dirty/source-generation rebuild semantics, but do not
// mistake its global completion watermark for coverage of a requested range.
// This runs inside the same application coordinator as capture/publication.
func (s *Service) ensureAttributionSnapshots(ctx context.Context, householdID domain.HouseholdID, left, right string) error {
	if err := s.ensureClosedDaySnapshots(ctx, left, right); err != nil {
		return err
	}
	start, _ := time.Parse("2006-01-02", left)
	end, _ := time.Parse("2006-01-02", right)
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, householdID, start, end)
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		present[snapshot.LocalDate] = true
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
		if !present[day.Format("2006-01-02")] {
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
		return rebuildGap(end)
	}
	return nil
}
