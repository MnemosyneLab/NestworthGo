// Package analytics adapts internal/application.Service's cost/gain read
// surface (HoldingGain, AccountGain, RealizedGain, RealizedGainInRange) for
// the Wails IPC boundary. It is read-only and depends on the instrument and
// holding services' data already being loaded by the frontend.
package analytics

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type Service struct {
	app *application.Service
}

func NewService(app *application.Service) *Service {
	return &Service{app: app}
}

func (s *Service) HoldingGain(ctx context.Context, holdingID string) (wire.HoldingGainDTO, error) {
	id, err := domain.ParseHoldingID(holdingID)
	if err != nil {
		return wire.HoldingGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.HoldingGain(ctx, id)
	if err != nil {
		return wire.HoldingGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromHoldingGain(view), nil
}

func (s *Service) AccountGain(ctx context.Context, accountID string) (wire.AccountGainDTO, error) {
	id, err := domain.ParseAccountID(accountID)
	if err != nil {
		return wire.AccountGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.AccountGain(ctx, id)
	if err != nil {
		return wire.AccountGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromAccountGain(view), nil
}

// GainScopeRequest mirrors domain.GainScope: nil fields mean "not filtered."
type GainScopeRequest struct {
	AccountID    *string `json:"accountId,omitempty"`
	InstrumentID *string `json:"instrumentId,omitempty"`
}

func (r GainScopeRequest) toDomain() (domain.GainScope, error) {
	scope := domain.GainScope{}
	if r.AccountID != nil {
		id, err := domain.ParseAccountID(*r.AccountID)
		if err != nil {
			return domain.GainScope{}, err
		}
		scope.AccountID = &id
	}
	if r.InstrumentID != nil {
		id, err := domain.ParseInstrumentID(*r.InstrumentID)
		if err != nil {
			return domain.GainScope{}, err
		}
		scope.InstrumentID = &id
	}
	return scope, nil
}

func (s *Service) RealizedGainInRange(ctx context.Context, scope GainScopeRequest, from, to string) (wire.RealizedGainDTO, error) {
	domainScope, err := scope.toDomain()
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.RealizedGainInRange(ctx, domainScope, from, to)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromRealizedGain(view), nil
}

// RealizedGain resolves a named trend range ("30d", "1y", "all") the same
// way the Analytics range selector already does in the interaction brief.
func (s *Service) RealizedGain(ctx context.Context, scope GainScopeRequest, trendRange string) (wire.RealizedGainDTO, error) {
	domainScope, err := scope.toDomain()
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.RealizedGain(ctx, domainScope, domain.TrendRange(trendRange))
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromRealizedGain(view), nil
}
