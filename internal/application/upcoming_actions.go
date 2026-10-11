package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// UpcomingActions performs one local read; it never plans or executes a mutation.
func (s *Service) UpcomingActions(ctx context.Context) (domain.UpcomingActions, error) {
	overview, snapshot, sources, err := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if err != nil {
		return domain.UpcomingActions{}, err
	}
	return domain.ProjectUpcomingActions(overview, sources, snapshot)
}
