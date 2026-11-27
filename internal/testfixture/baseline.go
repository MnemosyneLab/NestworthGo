// Package testfixture constructs explicitly synthetic existing-history
// ledgers for tests and QA seed commands, never for the running product.
package testfixture

import (
	"context"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/testfixture/repos"
	"github.com/waltwang/nestworth-go/internal/testfixture/storage"
)

// Repository registers a temporary fixture DB, preserving the real database
// handle and transaction wrapper used for the fixture.
func Repository(db *sqlite.DB) *sqlite.Repository { return repos.New(db) }

type BaselineRepository struct{ *sqlite.Repository }

func (r *BaselineRepository) InitializeLedger(ctx context.Context, household domain.Household, members []domain.Member, _ domain.HistoryOrigin) error {
	return storage.ForPath(r.Path()).CreateOnboarding(ctx, household, members)
}

type registration struct {
	repo application.Repository
	now  func() time.Time
}

var registered sync.Map

// NewService registers a fixture's service, repository and clock. The normal
// product constructor has no dependency on this package.
func NewService(repo application.Repository, registries ...application.MarketDataRegistryPort) *application.Service {
	s := application.NewService(repo, registries...)
	registered.Store(s, registration{repo: repo, now: time.Now})
	return s
}
func SetClock(s interface{ SetClock(func() time.Time) }, now func() time.Time) {
	s.SetClock(now)
	if entry, ok := registered.Load(s); ok {
		reg := entry.(registration)
		reg.now = now
		registered.Store(s, reg)
	}
}
func registrationFor(s *application.Service) registration {
	reg, ok := registered.Load(s)
	if !ok {
		panic("fixture service was not registered")
	}
	return reg.(registration)
}
func CaptureSyntheticOrigin(s *application.Service, ctx context.Context, timezone string) (domain.HistoryOrigin, error) {
	return CaptureSyntheticOriginWithCosts(s, ctx, timezone, nil)
}
func CaptureSyntheticOriginWithCosts(s *application.Service, ctx context.Context, timezone string, costs map[domain.HoldingID]string) (domain.HistoryOrigin, error) {
	return CaptureSyntheticOriginOnDate(s, ctx, timezone, "", costs)
}
func CaptureSyntheticOriginOnDate(s *application.Service, ctx context.Context, timezone, day string, costs map[domain.HoldingID]string) (out domain.HistoryOrigin, err error) {
	reg := registrationFor(s)
	err = s.WithWrite(ctx, func(ctx context.Context) error {
		household, err := reg.repo.Household(ctx)
		if err != nil {
			return err
		}
		if household == nil {
			return &domain.Error{Code: domain.ErrNotFound, Message: "synthetic household is missing"}
		}
		snapshot, err := reg.repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return err
		}
		data, err := storage.BuildOrigin(household.ID, snapshot, timezone, day, reg.now(), costs, storage.SelectQuote)
		if err != nil {
			return err
		}
		out, err = storage.ForPath(reg.repo.Path()).CaptureSyntheticOrigin(ctx, data)
		return err
	})
	return
}
