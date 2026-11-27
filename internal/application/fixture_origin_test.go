package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

func (s *Service) CaptureSyntheticOrigin(ctx context.Context, timezone string) (domain.HistoryOrigin, error) {
	return s.CaptureSyntheticOriginWithCosts(ctx, timezone, nil)
}
func (s *Service) CaptureSyntheticOriginWithCosts(ctx context.Context, timezone string, costs map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	return s.CaptureSyntheticOriginOnDate(ctx, timezone, "", costs)
}
func (s *Service) CaptureSyntheticOriginOnDate(ctx context.Context, timezone, day string, costs map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	ctx, unlock, err := s.beginLedgerWrite(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	defer unlock()
	household, err := s.repository.Household(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	if household == nil {
		return domain.HistoryOrigin{}, onboardingRequired()
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	data, err := storage.BuildOrigin(household.ID, snapshot, timezone, day, s.clock(), costs, selectInstrumentQuote)
	if err != nil {
		return domain.HistoryOrigin{}, err
	}
	result, err := storage.ForPath(s.repository.Path()).CaptureSyntheticOrigin(ctx, data)
	if err == nil {
		s.invalidateAnalysis()
	}
	return result, err
}
