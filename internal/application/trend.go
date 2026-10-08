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
		status := domain.TrendPointIncomplete
		if snapshot.ID == "" {
			status = domain.TrendPointMissing
		}
		var netWorth *domain.SignedMoney
		var assets *domain.Money
		var liabilities *domain.Money
		if snapshot.Complete && snapshot.ID != "" {
			netWorth = snapshot.NetWorthAmount
			assets = snapshot.AssetsAmount
			liabilities = snapshot.LiabilitiesAmount
			status = domain.TrendPointComplete
		}
		points = append(points, domain.NetWorthTrendPoint{
			LocalDate:    snapshot.LocalDate,
			NetWorth:     netWorth,
			Assets:       assets,
			Liabilities:  liabilities,
			Status:       status,
			Complete:     snapshot.Complete,
			MissingCount: snapshot.MissingCount,
		})
	}
	if window.todayKey != "" && window.includeCurrent {
		current, err := s.Overview(ctx, domain.AccountFilter{})
		if err != nil {
			return domain.NetWorthTrend{}, err
		}
		point := domain.NetWorthTrendPoint{
			LocalDate: window.todayKey, Complete: current.Complete, Current: true,
			MissingCount: len(current.MissingInputs), Status: domain.TrendPointIncomplete,
		}
		if current.Complete {
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
			point.NetWorth = &netWorth
			point.Assets = &assets
			point.Liabilities = &liabilities
			point.Status = domain.TrendPointComplete
		}
		points = append(points, point)
	}
	if len(points) == 0 {
		return domain.NetWorthTrend{Range: trendRange, Currency: window.currency}, nil
	}
	start, end, change, err := wealthTrendSummary(points, window.currency)
	if err != nil {
		return domain.NetWorthTrend{}, err
	}
	complete := true
	for _, point := range points {
		if !point.Complete {
			complete = false
		}
	}
	reason := ""
	if len(points) < 2 {
		reason = "insufficient_history"
	} else if start == nil || end == nil {
		reason = "missing_boundary"
	}
	return domain.NetWorthTrend{Range: trendRange, Currency: window.currency, Points: points, Start: start, End: end, Change: change,
		StartDate: points[0].LocalDate, EndDate: points[len(points)-1].LocalDate, Complete: complete, SummaryReason: reason}, nil
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
	if window.todayKey != "" && window.includeCurrent {
		current, err := s.Portfolio(ctx, domain.AccountFilter{})
		if err != nil {
			return domain.PortfolioTrend{}, err
		}
		var valued *domain.Money
		if current.Complete && current.ValuedSubtotal != nil {
			money, parseErr := domain.ParseMoney(current.ValuedSubtotal.Amount, current.ValuedSubtotal.Currency)
			if parseErr != nil {
				return domain.PortfolioTrend{}, parseErr
			}
			valued = &money
		}
		points = append(points, domain.PortfolioTrendPoint{
			LocalDate:      window.todayKey,
			ValuedSubtotal: valued,
			Status: func() domain.TrendPointStatus {
				if current.Complete {
					return domain.TrendPointComplete
				}
				return domain.TrendPointIncomplete
			}(),
			Complete:     current.Complete,
			MissingCount: len(current.MissingInputs),
		})
	}
	if len(points) == 0 {
		return domain.PortfolioTrend{Range: trendRange, Currency: window.currency}, nil
	}
	return domain.PortfolioTrend{Range: trendRange, Currency: window.currency, Points: points}, nil
}

type closedTrendWindow struct {
	includeCurrent bool
	currency       domain.CurrencyCode
	todayKey       string
	snapshots      []domain.DailyValuationSnapshot
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
	window := &closedTrendWindow{currency: bootstrap.Household.BaseCurrency, includeCurrent: true}
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
	customFrom, customTo, custom := trendRange.DateBounds()
	if custom {
		window.includeCurrent = customFrom <= window.todayKey && customTo >= window.todayKey
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if originDate >= window.todayKey {
		return window, nil
	}
	since, err := trendSince(trendRange, today, origin.StartedAt.In(location))
	if err != nil {
		return nil, err
	}
	originLocal := origin.StartedAt.In(location)
	originMidnight := time.Date(originLocal.Year(), originLocal.Month(), originLocal.Day(), 0, 0, 0, 0, location)
	if since.Before(originMidnight) {
		since = originMidnight
	}
	first := since.In(location)
	first = time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, location)
	last := today.AddDate(0, 0, -1)
	if custom && customTo < last.Format("2006-01-02") {
		last, _ = time.ParseInLocation("2006-01-02", customTo, location)
	}
	if err := s.ensureClosedDaySnapshots(ctx, first.Format("2006-01-02"), last.Format("2006-01-02")); err != nil {
		return nil, err
	}
	// The repository filters local-date labels, independently of UTC offsets.
	sinceDate, _ := time.Parse("2006-01-02", first.Format("2006-01-02"))
	untilDate, _ := time.Parse("2006-01-02", last.Format("2006-01-02"))
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, sinceDate, untilDate)
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]domain.DailyValuationSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byDate[snapshot.LocalDate] = snapshot
	}
	for cursor := first; !cursor.After(last); cursor = cursor.AddDate(0, 0, 1) {
		key := cursor.Format("2006-01-02")
		if snapshot, ok := byDate[key]; ok {
			window.snapshots = append(window.snapshots, snapshot)
			continue
		}
		// A missing day is a first-class gap, never a zero-valued snapshot.
		window.snapshots = append(window.snapshots, domain.DailyValuationSnapshot{LocalDate: key, MissingCount: 1})
	}
	return window, nil
}

func trendSince(trendRange domain.TrendRange, today, origin time.Time) (time.Time, error) {
	if from, _, ok := trendRange.DateBounds(); ok {
		return time.ParseInLocation("2006-01-02", from, today.Location())
	}
	switch trendRange {
	case domain.Trend30Days:
		return today.AddDate(0, 0, -29), nil
	case domain.TrendYearToDate:
		return time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, today.Location()), nil
	case domain.TrendOneYear:
		return today.AddDate(0, 0, -364), nil
	case domain.TrendAllTime:
		return origin, nil
	default:
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "range", Message: "trend range is not supported"}
	}
}

func wealthTrendSummary(points []domain.NetWorthTrendPoint, currency domain.CurrencyCode) (*domain.SignedMoney, *domain.SignedMoney, *domain.SignedMoney, error) {
	if len(points) == 0 {
		return nil, nil, nil, nil
	}
	var start, end *domain.SignedMoney
	if points[0].Complete {
		start = points[0].NetWorth
	}
	if points[len(points)-1].Complete {
		end = points[len(points)-1].NetWorth
	}
	if len(points) < 2 || start == nil || end == nil {
		return start, end, nil, nil
	}
	change, err := domain.NewSignedMoney(end.Amount().Sub(start.Amount()), currency)
	if err != nil {
		return nil, nil, nil, err
	}
	return start, end, &change, nil
}

func portfolioPointFromSnapshot(snapshot domain.DailyValuationSnapshot, currency domain.CurrencyCode) (domain.PortfolioTrendPoint, bool, error) {
	if snapshot.ID == "" {
		return domain.PortfolioTrendPoint{LocalDate: snapshot.LocalDate, Status: domain.TrendPointMissing, MissingCount: 1}, true, nil
	}
	valued := decimal.Zero
	missing := 0
	complete := true
	hasInstrument := false
	for _, item := range snapshot.Items {
		if item.InstrumentID == nil {
			continue
		}
		hasInstrument = true
		exact := item.BaseAmountExact
		if exact == "" && item.BaseAmount != nil {
			exact = item.BaseAmount.CanonicalAmount()
		}
		if exact != "" {
			amount, parseErr := decimal.NewFromString(exact)
			if parseErr != nil {
				return domain.PortfolioTrendPoint{}, false, &domain.Error{Code: domain.ErrIntegrity, Message: "stored snapshot base amount is invalid"}
			}
			valued = valued.Add(amount)
		}
		if !item.Complete {
			complete = false
			missing++
		}
	}
	if !hasInstrument {
		if snapshot.Complete {
			return domain.PortfolioTrendPoint{}, false, nil
		}
		return domain.PortfolioTrendPoint{LocalDate: snapshot.LocalDate, Status: domain.TrendPointIncomplete, MissingCount: snapshot.MissingCount}, true, nil
	}
	if !snapshot.Complete || !complete || missing > 0 {
		return domain.PortfolioTrendPoint{LocalDate: snapshot.LocalDate, Status: domain.TrendPointIncomplete, MissingCount: missing}, true, nil
	}
	money, err := domain.NewMoney(valued, currency)
	if err != nil {
		return domain.PortfolioTrendPoint{}, false, err
	}
	return domain.PortfolioTrendPoint{
		LocalDate:      snapshot.LocalDate,
		ValuedSubtotal: &money,
		Status:         domain.TrendPointComplete,
		Complete:       true,
		MissingCount:   missing,
	}, true, nil
}

// ensureClosedDaySnapshots retains the caller's inclusive local-date contract.
// Analysis supplies its predecessor (or origin); trends supply their chart window.
func (s *Service) ensureClosedDaySnapshots(ctx context.Context, startDate, endDate string) error {
	if startDate == "" || endDate == "" || startDate > endDate {
		return nil
	}
	return s.withSnapshotMaintenance(ctx, func(ctx context.Context) error {
		household, err := s.requireHousehold(ctx)
		if err != nil {
			return err
		}
		return s.ensureSnapshotCoverage(ctx, household.ID, startDate, endDate)
	})
}
