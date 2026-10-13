package application

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// UpcomingActions performs one local read; it never plans or executes a mutation.
func (s *Service) UpcomingActions(ctx context.Context) (domain.UpcomingActions, error) {
	result, err := s.UpcomingActionsRead(ctx)
	return result.Actions, err
}

type UpcomingActionsReadResult struct {
	Actions          domain.UpcomingActions
	SourceAccountIDs map[string]string
}

// UpcomingActionsRead retains authoritative account identity for check filters
// without parsing display keys or querying products after the capture.
func (s *Service) UpcomingActionsRead(ctx context.Context) (UpcomingActionsReadResult, error) {
	overview, snapshot, sources, err := s.prepareLiquidity(ctx, LiquidityOverviewQuery{}, s.clock())
	if err != nil {
		return UpcomingActionsReadResult{}, err
	}
	actions, err := domain.ProjectUpcomingActions(overview, sources, snapshot)
	if err != nil {
		return UpcomingActionsReadResult{}, err
	}
	ids := map[string]string{}
	for _, source := range sources {
		ids[source.Ref.Key()] = source.AccountID.String()
	}
	return UpcomingActionsReadResult{Actions: actions, SourceAccountIDs: ids}, nil
}
