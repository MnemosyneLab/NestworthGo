package application

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (s *Service) InstrumentQuoteSeries(ctx context.Context, id domain.InstrumentID, trendRange domain.TrendRange, sourceFilter domain.QuoteSourceFilter) (domain.QuoteSeries, error) {
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	empty := domain.QuoteSeries{Range: trendRange}
	if bootstrap.Household == nil {
		return empty, nil
	}
	instrument, err := s.repository.Instrument(ctx, bootstrap.Household.ID, id)
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	location := s.seriesLocation(ctx)
	quotes, err := s.InstrumentQuoteHistory(ctx, id, quoteHistoryQuery(trendRange, sourceFilter, s.clock(), location, "", ""))
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	observations := make([]domain.QuoteSeriesPoint, 0, len(quotes))
	for _, quote := range quotes {
		observations = append(observations, domain.QuoteSeriesPoint{
			ID:         quote.ID.String(),
			QuotedAt:   quote.QuotedAt,
			CreatedAt:  quote.CreatedAt,
			Value:      quote.UnitPrice.Canonical(),
			SourceKind: quote.SourceKind,
			SourceKey:  quote.SourceKey,
			Delayed:    quote.Delayed,
		})
	}
	outsideRange, err := s.instrumentQuotesOutsideRange(ctx, id, trendRange, sourceFilter, location, len(observations))
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	return domain.QuoteSeries{
		Range:           trendRange,
		DisplayCurrency: instrument.QuoteCurrency,
		Points:          chartQuotePoints(observations, trendRange, location),
		Observations:    observations,
		OutsideRange:    outsideRange,
	}, nil
}

func (s *Service) FXQuoteSeries(ctx context.Context, currencyA, currencyB domain.CurrencyCode, trendRange domain.TrendRange, sourceFilter domain.QuoteSourceFilter) (domain.QuoteSeries, error) {
	base, quoteCurrency, err := parseOrientedPair(currencyA, currencyB)
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	bootstrap, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	empty := domain.QuoteSeries{Range: trendRange, BaseCurrency: base, QuoteCurrency: quoteCurrency}
	if bootstrap.Household == nil {
		return empty, nil
	}
	location := s.seriesLocation(ctx)
	quotes, err := s.FXQuoteHistory(ctx, quoteHistoryQuery(trendRange, sourceFilter, s.clock(), location, base, quoteCurrency))
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	observations := make([]domain.QuoteSeriesPoint, 0, len(quotes))
	for _, quote := range quotes {
		value, orientErr := fxRateInDirection(quote, base, quoteCurrency)
		if orientErr != nil {
			continue
		}
		observations = append(observations, domain.QuoteSeriesPoint{
			ID:         quote.ID.String(),
			QuotedAt:   quote.QuotedAt,
			CreatedAt:  quote.CreatedAt,
			Value:      value,
			SourceKind: quote.SourceKind,
			SourceKey:  quote.SourceKey,
			Delayed:    quote.Delayed,
		})
	}
	sortQuoteSeriesNewestFirst(observations)
	outsideRange, err := s.fxQuotesOutsideRange(ctx, base, quoteCurrency, trendRange, sourceFilter, location, len(observations))
	if err != nil {
		return domain.QuoteSeries{}, err
	}
	return domain.QuoteSeries{
		Range:         trendRange,
		BaseCurrency:  base,
		QuoteCurrency: quoteCurrency,
		Points:        chartQuotePoints(observations, trendRange, location),
		Observations:  observations,
		OutsideRange:  outsideRange,
	}, nil
}

func parseOrientedPair(currencyA, currencyB domain.CurrencyCode) (domain.CurrencyCode, domain.CurrencyCode, error) {
	base, err := domain.ParseSupportedCurrency(currencyA.String())
	if err != nil {
		return "", "", err
	}
	quoteCurrency, err := domain.ParseSupportedCurrency(currencyB.String())
	if err != nil {
		return "", "", err
	}
	if base == quoteCurrency {
		return "", "", &domain.Error{Code: domain.ErrValidation, Field: "pair", Message: "FX pair currencies must be different"}
	}
	return base, quoteCurrency, nil
}

func fxRateInDirection(quote domain.FXQuote, base, quoteCurrency domain.CurrencyCode) (string, error) {
	if quote.BaseCurrency == base && quote.QuoteCurrency == quoteCurrency {
		return quote.Rate.Canonical(), nil
	}
	if quote.BaseCurrency == quoteCurrency && quote.QuoteCurrency == base {
		inverted, err := domain.NewFxRate(decimal.NewFromInt(1).Div(quote.Rate.Decimal()).RoundBank(12))
		if err != nil {
			return "", err
		}
		return inverted.Canonical(), nil
	}
	return "", &domain.Error{Code: domain.ErrValidation, Field: "pair", Message: "FX quote does not match the requested pair"}
}

func quoteHistoryQuery(trendRange domain.TrendRange, sourceFilter domain.QuoteSourceFilter, now time.Time, location *time.Location, currencyA, currencyB domain.CurrencyCode) QuoteHistoryQuery {
	query := QuoteHistoryQuery{SourceKind: sourceFilter.SourceKind(), CurrencyA: currencyA, CurrencyB: currencyB}
	if location == nil {
		location = time.UTC
	}
	localNow := now.In(location)
	year, month, day := localNow.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, location)
	switch trendRange {
	case domain.Trend30Days:
		from := today.AddDate(0, 0, -29)
		query.From = &from
	case domain.TrendOneYear:
		from := today.AddDate(0, 0, -364)
		query.From = &from
	}
	return query
}

func (s *Service) instrumentQuotesOutsideRange(ctx context.Context, id domain.InstrumentID, trendRange domain.TrendRange, sourceFilter domain.QuoteSourceFilter, location *time.Location, inRangeCount int) (bool, error) {
	if inRangeCount > 0 || trendRange == domain.TrendAllTime {
		return false, nil
	}
	all, err := s.InstrumentQuoteHistory(ctx, id, quoteHistoryQuery(domain.TrendAllTime, sourceFilter, s.clock(), location, "", ""))
	if err != nil {
		return false, err
	}
	return len(all) > 0, nil
}

func (s *Service) fxQuotesOutsideRange(ctx context.Context, base, quoteCurrency domain.CurrencyCode, trendRange domain.TrendRange, sourceFilter domain.QuoteSourceFilter, location *time.Location, inRangeCount int) (bool, error) {
	if inRangeCount > 0 || trendRange == domain.TrendAllTime {
		return false, nil
	}
	all, err := s.FXQuoteHistory(ctx, quoteHistoryQuery(domain.TrendAllTime, sourceFilter, s.clock(), location, base, quoteCurrency))
	if err != nil {
		return false, err
	}
	return len(all) > 0, nil
}

func (s *Service) seriesLocation(ctx context.Context) *time.Location {
	origin, err := s.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		return time.UTC
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

func chartQuotePoints(observations []domain.QuoteSeriesPoint, trendRange domain.TrendRange, location *time.Location) []domain.QuoteSeriesPoint {
	if len(observations) == 0 {
		return []domain.QuoteSeriesPoint{}
	}
	if location == nil {
		location = time.UTC
	}
	selected := map[string]domain.QuoteSeriesPoint{}
	for _, observation := range observations {
		key := observation.QuotedAt.UTC().Format(time.RFC3339Nano)
		if trendRange != domain.Trend30Days {
			key = observation.QuotedAt.In(location).Format("2006-01-02")
		}
		current, exists := selected[key]
		if !exists || quoteLater(observation.QuotedAt, observation.CreatedAt, observation.ID, current.QuotedAt, current.CreatedAt, current.ID) {
			selected[key] = observation
		}
	}
	points := make([]domain.QuoteSeriesPoint, 0, len(selected))
	for _, point := range selected {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool {
		if !points[i].QuotedAt.Equal(points[j].QuotedAt) {
			return points[i].QuotedAt.Before(points[j].QuotedAt)
		}
		if !points[i].CreatedAt.Equal(points[j].CreatedAt) {
			return points[i].CreatedAt.Before(points[j].CreatedAt)
		}
		return points[i].ID < points[j].ID
	})
	return points
}

func sortQuoteSeriesNewestFirst(points []domain.QuoteSeriesPoint) {
	sort.SliceStable(points, func(i, j int) bool {
		return quoteLater(points[i].QuotedAt, points[i].CreatedAt, points[i].ID, points[j].QuotedAt, points[j].CreatedAt, points[j].ID)
	})
}
