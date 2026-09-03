// Package analytics adapts internal/application.Service's cost/gain read
// surface (HoldingGain, AccountGain, AccountGains, RealizedGain, DividendIncome,
// and their range readers) for the Wails IPC boundary. It is read-only.
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

func (s *Service) AccountGains(ctx context.Context) ([]wire.AccountGainDTO, error) {
	views, err := s.app.AccountGains(ctx, nil)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return wire.FromAccountGains(views), nil
}

// GainScopeRequest mirrors domain.GainScope: an empty request is the explicit
// portfolio scope; setting AccountID or InstrumentID narrows the calculation.
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

// RealizedGain resolves a named trend range ("30d", "ytd", "1y", "all") the same
// way the Analytics range selector is implemented in the frontend.
func (s *Service) RealizedGain(ctx context.Context, scope GainScopeRequest, trendRange string) (wire.RealizedGainDTO, error) {
	domainScope, err := scope.toDomain()
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	parsed, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.RealizedGain(ctx, domainScope, parsed)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromRealizedGain(view), nil
}

func (s *Service) DividendIncomeInRange(ctx context.Context, scope GainScopeRequest, from, to string) (wire.RealizedGainDTO, error) {
	domainScope, err := scope.toDomain()
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.DividendIncomeInRange(ctx, domainScope, from, to)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromDividendIncome(view), nil
}

// DividendIncome resolves a named trend range ("30d", "ytd", "1y", "all") the same
// way the Analytics range selector is implemented in the frontend.
func (s *Service) DividendIncome(ctx context.Context, scope GainScopeRequest, trendRange string) (wire.RealizedGainDTO, error) {
	domainScope, err := scope.toDomain()
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	parsed, err := domain.ParseTrendRange(trendRange)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	view, err := s.app.DividendIncome(ctx, domainScope, parsed)
	if err != nil {
		return wire.RealizedGainDTO{}, apierror.Wrap(err)
	}
	return wire.FromDividendIncome(view), nil
}
