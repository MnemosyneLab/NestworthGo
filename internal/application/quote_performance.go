package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type quotePerformanceReader interface {
	ReadQuotePerformanceEvidence(context.Context, domain.Instrument, string) ([]domain.InstrumentQuote, []string, error)
}

func (s *Service) InstrumentQuotePerformance(ctx context.Context, id domain.InstrumentID) (domain.QuotePerformanceReport, error) {
	b, err := s.Bootstrap(ctx)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	if b.Household == nil {
		return domain.QuotePerformanceReport{Items: []domain.QuotePerformance{}, UnavailableReason: "insufficient_daily_history"}, nil
	}
	i, err := s.repository.Instrument(ctx, b.Household.ID, id)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	product, err := s.repository.ProductByInstrument(ctx, id)
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	if product != nil {
		return domain.QuotePerformanceReport{Items: []domain.QuotePerformance{}, UnavailableReason: "unsupported_instrument"}, nil
	}
	reader, ok := s.repository.(quotePerformanceReader)
	if !ok {
		return domain.QuotePerformanceReport{}, &domain.Error{Code: domain.ErrUnavailable, Message: "local price history is unavailable"}
	}
	quotes, unsafe, err := reader.ReadQuotePerformanceEvidence(ctx, i, s.clock().In(s.seriesLocation(ctx)).Format(time.DateOnly))
	if err != nil {
		return domain.QuotePerformanceReport{}, err
	}
	return domain.ProjectQuotePerformanceReport(i, quotes, unsafe), nil
}
