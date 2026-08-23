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
	nowLocal := s.now().In(location)
	year, month, day := nowLocal.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
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
		points = append(points, domain.NetWorthTrendPoint{LocalDate: snapshot.LocalDate, Value: snapshot.NetWorthAmount, Complete: snapshot.Complete})
	}
	current, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	currentMoney, err := domain.NewMoney(current.NetWorth, current.Currency)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	todayKey := s.now().In(location).Format("2006-01-02")
	points = append(points, domain.NetWorthTrendPoint{LocalDate: todayKey, Value: &currentMoney, Complete: current.Complete})
	return domain.NetWorthTrend{Range: trendRange, Currency: bootstrap.Household.BaseCurrency, Points: points}, nil
}
