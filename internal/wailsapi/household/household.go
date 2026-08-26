// Package household adapts internal/application.Service's Household
// bootstrap and onboarding surface for the Wails IPC boundary. It replaces
// the Fyne Onboarding page and the app-startup Bootstrap call.
package household

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// Service is registered with application.NewService in the Wails main.go.
// It depends only on internal/application, never on internal/infrastructure
// or fyne.io/*, per the technical design's dependency rule (Sec3).
type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

// BootstrapResult mirrors application.Bootstrap. Household is nil until
// onboarding completes; the frontend uses its presence to decide whether to
// show the Onboarding flow or the main application shell.
type BootstrapResult struct {
	Household    *wire.HouseholdDTO    `json:"household"`
	Members      []wire.MemberDTO      `json:"members"`
	Institutions []wire.InstitutionDTO `json:"institutions"`
	Groups       []wire.GroupDTO       `json:"groups"`
}

func fromBootstrap(value application.Bootstrap) BootstrapResult {
	result := BootstrapResult{
		Members:      wire.FromMembers(value.Members),
		Institutions: wire.FromInstitutions(value.Institutions),
		Groups:       wire.FromGroups(value.Groups),
	}
	if value.Household != nil {
		household := wire.FromHousehold(*value.Household)
		result.Household = &household
	}
	return result
}

// Bootstrap loads the current Household plus its active Members,
// Institutions, and Groups in one call, exactly as internal/app.New() and
// Fyne's Controller.Refresh() already do today.
func (s *Service) Bootstrap(ctx context.Context) (BootstrapResult, error) {
	bootstrap, err := s.app.Bootstrap(ctx)
	if err != nil {
		return BootstrapResult{}, apierror.Wrap(err)
	}
	return fromBootstrap(bootstrap), nil
}

// CompleteOnboardingRequest is the Onboarding form's submitted shape. An
// empty Timezone means "history not started yet" (application.Service
// interprets it the same way Fyne's onboarding page already does).
type CompleteOnboardingRequest struct {
	HouseholdName string   `json:"householdName"`
	BaseCurrency  string   `json:"baseCurrency"`
	MemberNames   []string `json:"memberNames"`
	Timezone      string   `json:"timezone,omitempty"`
}

// CompleteOnboarding creates the singleton Household, its base currency,
// and its first Members. It fails with domain.ErrConflict if a Household
// already exists.
func (s *Service) CompleteOnboarding(ctx context.Context, request CompleteOnboardingRequest) error {
	err := s.app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: request.HouseholdName,
		BaseCurrency:  request.BaseCurrency,
		MemberNames:   request.MemberNames,
		Timezone:      request.Timezone,
	})
	return apierror.Wrap(err)
}
