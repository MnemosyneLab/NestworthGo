package application

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) NetWorthTrend(ctx context.Context, trendRange domain.TrendRange) (domain.NetWorthTrend, error) {
	window, err := s.closedTrendWindow(ctx, trendRange)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	if window == nil {
		return domain.NetWorthTrend{Range: trendRange}, nil
	}
	points := make([]domain.NetWorthTrendPoint, 0, len(window.snapshots)+1)
	for _, snapshot := range window.snapshots {
		if snapshot.LocalDate >= window.todayKey {
			continue
		}
		points = append(points, domain.NetWorthTrendPoint{
			LocalDate:    snapshot.LocalDate,
			NetWorth:     snapshot.NetWorthAmount,
			Assets:       snapshot.AssetsAmount,
			Liabilities:  snapshot.LiabilitiesAmount,
			Complete:     snapshot.Complete,
			MissingCount: snapshot.MissingCount,
		})
	}
	if window.todayKey != "" {
		current, err := s.Overview(ctx, domain.AccountFilter{})
		if err != nil {
			return domain.NetWorthTrend{}, err
		}
		netWorth, err := domain.NewSignedMoney(current.NetWorth, current.Currency)
		if err != nil {
			return domain.NetWorthTrend{}, err
		}
		assets, err := domain.NewMoney(current.Assets, current.Currency)
		if err != nil {
			return domain.NetWorthTrend{}, err
		}
		liabilities, err := domain.NewMoney(current.Liabilities, current.Currency)
		if err != nil {
			return domain.NetWorthTrend{}, err
		}
		points = append(points, domain.NetWorthTrendPoint{
			LocalDate:    window.todayKey,
			NetWorth:     &netWorth,
			Assets:       &assets,
			Liabilities:  &liabilities,
			Complete:     current.Complete,
			MissingCount: len(current.MissingInputs),
		})
	}
	if len(points) == 0 {
		return domain.NetWorthTrend{Range: trendRange, Currency: window.currency}, nil
	}
	start, end, change, err := wealthTrendSummary(points, window.currency)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	return domain.NetWorthTrend{Range: trendRange, Currency: window.currency, Points: points, Start: start, End: end, Change: change}, nil
}

func (s *Service) PortfolioTrend(ctx context.Context, trendRange domain.TrendRange) (domain.PortfolioTrend, error) {
	window, err := s.closedTrendWindow(ctx, trendRange)
	if err != nil {
		return domain.PortfolioTrend{}, err
	}
	if window == nil {
		return domain.PortfolioTrend{Range: trendRange}, nil
	}
	points := make([]domain.PortfolioTrendPoint, 0, len(window.snapshots)+1)
	for _, snapshot := range window.snapshots {
		if snapshot.LocalDate >= window.todayKey {
			continue
		}
		point, include, err := portfolioPointFromSnapshot(snapshot, window.currency)
		if err != nil {
			return domain.PortfolioTrend{}, err
		}
		if include {
			points = append(points, point)
		}
	}
	if window.todayKey != "" {
		current, err := s.Portfolio(ctx, domain.AccountFilter{})
		if err != nil {
			return domain.PortfolioTrend{}, err
		}
		var valued *domain.Money
		if current.ValuedSubtotal != nil {
			money, parseErr := domain.ParseMoney(current.ValuedSubtotal.Amount, current.ValuedSubtotal.Currency)
			if parseErr != nil {
				return domain.PortfolioTrend{}, parseErr
			}
			valued = &money
		}
		points = append(points, domain.PortfolioTrendPoint{
			LocalDate:      window.todayKey,
			ValuedSubtotal: valued,
			Complete:       current.Complete,
			MissingCount:   len(current.MissingInputs),
		})
	}
	if len(points) == 0 {
		return domain.PortfolioTrend{Range: trendRange, Currency: window.currency}, nil
	}
	return domain.PortfolioTrend{Range: trendRange, Currency: window.currency, Points: points}, nil
}

type closedTrendWindow struct {
	currency  domain.CurrencyCode
	todayKey  string
	snapshots []domain.DailyValuationSnapshot
}

func (s *Service) closedTrendWindow(ctx context.Context, trendRange domain.TrendRange) (*closedTrendWindow, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if bootstrap.Household == nil {
		return nil, nil
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return nil, err
	}
	window := &closedTrendWindow{currency: bootstrap.Household.BaseCurrency}
	if origin == nil {
		return window, nil
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return nil, err
	}
	nowLocal := s.clock().In(location)
	year, month, day := nowLocal.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
	window.todayKey = today.Format("2006-01-02")
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if originDate >= window.todayKey {
		return window, nil
	}
	if err := s.ensureClosedDaySnapshots(ctx, originDate, today.AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
		return nil, err
	}
	since, err := trendSince(trendRange, today, origin.StartedAt.In(location))
	if err != nil {
		return nil, err
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, since)
	if err != nil {
		return nil, err
	}
	window.snapshots = snapshots
	return window, nil
}

func trendSince(trendRange domain.TrendRange, today, origin time.Time) (time.Time, error) {
	switch trendRange {
	case domain.Trend30Days:
		return today.AddDate(0, 0, -29), nil
	case domain.TrendOneYear:
		return today.AddDate(0, 0, -364), nil
	case domain.TrendAllTime:
		return origin, nil
	default:
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "trend range is not supported"}
	}
}

func wealthTrendSummary(points []domain.NetWorthTrendPoint, currency domain.CurrencyCode) (*domain.SignedMoney, *domain.SignedMoney, *domain.SignedMoney, error) {
	var start, end *domain.SignedMoney
	for _, point := range points {
		if point.NetWorth != nil {
			start = point.NetWorth
			break
		}
	}
	for index := len(points) - 1; index >= 0; index-- {
		if points[index].NetWorth != nil {
			end = points[index].NetWorth
			break
		}
	}
	if start == nil || end == nil {
		return start, end, nil, nil
	}
	change, err := domain.NewSignedMoney(end.Amount().Sub(start.Amount()), currency)
	if err != nil {
		return nil, nil, nil, err
	}
	return start, end, &change, nil
}

func portfolioPointFromSnapshot(snapshot domain.DailyValuationSnapshot, currency domain.CurrencyCode) (domain.PortfolioTrendPoint, bool, error) {
	valued := decimal.Zero
	missing := 0
	complete := true
	hasInstrument := false
	for _, item := range snapshot.Items {
		if item.InstrumentID == nil {
			continue
		}
		hasInstrument = true
		if item.BaseAmount != nil {
			valued = valued.Add(item.BaseAmount.Amount())
		}
		if !item.Complete {
			complete = false
			missing++
		}
	}
	if !hasInstrument {
		return domain.PortfolioTrendPoint{}, false, nil
	}
	money, err := domain.NewMoney(valued, currency)
	if err != nil {
		return domain.PortfolioTrendPoint{}, false, err
	}
	return domain.PortfolioTrendPoint{
		LocalDate:      snapshot.LocalDate,
		ValuedSubtotal: &money,
		Complete:       complete && missing == 0,
		MissingCount:   missing,
	}, true, nil
}

func closedDayRebuildFrom(originDate, yesterday string, state domain.DailySnapshotState) (string, bool, error) {
	if originDate == "" || yesterday == "" || originDate > yesterday {
		return "", true, nil
	}
	rebuildFrom := originDate
	dirty := ""
	if state.DirtyFrom != nil {
		dirty = *state.DirtyFrom
	}
	lastCompleted := ""
	if state.LastCompletedClosedOn != nil {
		lastCompleted = *state.LastCompletedClosedOn
	}
	if dirty != "" && dirty > rebuildFrom {
		rebuildFrom = dirty
	}
	if lastCompleted != "" && lastCompleted >= yesterday && dirty == "" {
		return "", true, nil
	}
	if dirty == "" && lastCompleted != "" {
		next, err := nextClosedDay(lastCompleted)
		if err != nil {
			return "", false, err
		}
		if next > rebuildFrom {
			rebuildFrom = next
		}
	}
	if rebuildFrom > yesterday {
		return "", true, nil
	}
	return rebuildFrom, false, nil
}

func nextClosedDay(localDate string) (string, error) {
	parsed, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", err
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02"), nil
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
	rebuildFrom, skip, err := closedDayRebuildFrom(startDate, yesterday, state)
	if err != nil {
		return err
	}
	if skip {
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
