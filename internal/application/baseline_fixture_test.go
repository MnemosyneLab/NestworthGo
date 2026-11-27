package application

import (
	"context"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

// completeBaselineFixture is exclusively a test construction mechanism for
// historical-origin fixtures. It stages the existing state whose origin the
// fixture will capture explicitly; normal CompleteOnboarding always creates
// an empty origin. Keeping the fixture separate avoids silently changing the
// financial facts of tests that compare existing balances, costs and replay.
func (s *Service) completeBaselineFixture(ctx context.Context, input OnboardingInput) error {
	currency, err := domain.ParseSupportedCurrency(strings.ToUpper(strings.TrimSpace(input.BaseCurrency)))
	if err != nil {
		return err
	}
	household, err := domain.NewHousehold(input.HouseholdName, currency, s.clock())
	if err != nil {
		return err
	}
	members := make([]domain.Member, 0, len(input.MemberNames))
	for index, name := range input.MemberNames {
		member, err := domain.NewMember(household.ID, name, s.clock())
		if err != nil {
			return err
		}
		member.SortOrder = index
		members = append(members, member)
	}
	if err := storage.ForPath(s.repository.Path()).CreateOnboarding(ctx, household, members); err != nil {
		return err
	}
	return s.ensureDefaultDirectory(ctx, household)
}
