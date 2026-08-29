package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) NetWorthTrend(ctx context.Context, trendRange domain.TrendRange) (domain.NetWorthTrend, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	if bootstrap.Household == nil {
		return domain.NetWorthTrend{}, nil
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	if origin == nil {
		return domain.NetWorthTrend{Range: trendRange, Currency: bootstrap.Household.BaseCurrency}, nil
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	nowLocal := s.clock().In(location)
	year, month, day := nowLocal.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
	todayKey := today.Format("2006-01-02")
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if originDate >= todayKey {
		return domain.NetWorthTrend{Range: trendRange, Currency: bootstrap.Household.BaseCurrency}, nil
	}
	if err := s.ensureClosedDaySnapshots(ctx, originDate, today.AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
		return domain.NetWorthTrend{}, err
	}
	since := time.Time{}
	switch trendRange {
	case domain.Trend30Days:
		since = today.AddDate(0, 0, -29)
	case domain.TrendOneYear:
		since = today.AddDate(0, 0, -364)
	case domain.TrendAllTime:
		since = origin.StartedAt.In(location)
	default:
		return domain.NetWorthTrend{}, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "trend range is not supported"}
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, since)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	points := make([]domain.NetWorthTrendPoint, 0, len(snapshots)+1)
	for _, snapshot := range snapshots {
		if snapshot.LocalDate >= todayKey {
			continue
		}
		points = append(points, domain.NetWorthTrendPoint{LocalDate: snapshot.LocalDate, Value: snapshot.NetWorthAmount, Complete: snapshot.Complete})
	}
	if len(points) == 0 {
		return domain.NetWorthTrend{Range: trendRange, Currency: bootstrap.Household.BaseCurrency}, nil
	}
	current, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	currentMoney, err := domain.NewMoney(current.NetWorth, current.Currency)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	points = append(points, domain.NetWorthTrendPoint{LocalDate: todayKey, Value: &currentMoney, Complete: current.Complete})
	return domain.NetWorthTrend{Range: trendRange, Currency: bootstrap.Household.BaseCurrency, Points: points}, nil
}

func (s *Service) ensureClosedDaySnapshots(ctx context.Context, startDate, yesterday string) error {
	if startDate == "" || yesterday == "" || startDate > yesterday {
		return nil
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return err
	}
	state, err := s.repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		return err
	}
	rebuildFrom := startDate
	if state.DirtyFrom != nil && *state.DirtyFrom != "" && *state.DirtyFrom > rebuildFrom {
		rebuildFrom = *state.DirtyFrom
	}
	if state.LastCompletedClosedOn != nil && *state.LastCompletedClosedOn >= yesterday && (state.DirtyFrom == nil || *state.DirtyFrom == "") {
		return nil
	}
	start, err := time.Parse("2006-01-02", rebuildFrom)
	if err != nil {
		return err
	}
	end, err := time.Parse("2006-01-02", yesterday)
	if err != nil {
		return err
	}
	for cursor := start; !cursor.After(end); {
		chunkEnd := cursor.AddDate(0, 0, 30)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		if _, err := s.RebuildHistoricalSnapshots(ctx, cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02")); err != nil {
			return err
		}
		cursor = chunkEnd.AddDate(0, 0, 1)
	}
	return nil
}
