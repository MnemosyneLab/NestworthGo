// Package testfixture constructs historical-origin baselines for synthetic
// tests and QA seed commands, never for a running application.
package testfixture

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// BaselineRepository deliberately stages the synthetic current-state facts
// before tests capture their declared existing-history baseline. It must not
// be used for onboarding acceptance tests or real user data.
type BaselineRepository struct{ *sqlite.Repository }

func (r *BaselineRepository) CreateOnboardingWithHistory(ctx context.Context, household domain.Household, members []domain.Member, _ domain.HistoryOriginData) error {
	return r.Repository.CreateOnboarding(ctx, household, members)
}
