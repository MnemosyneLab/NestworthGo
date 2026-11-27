package application

import (
	"context"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const marketSyncReviewTTL = 15 * time.Minute

type marketSyncWork struct {
	plan        HistoryRepairPlan
	household   domain.HouseholdID
	zone        string
	instruments []instrumentHistoryTask
	fx          []fxHistoryTask
	latest      []refreshTarget
	marketData  marketDataSnapshot
	provider    domain.ReviewProviderSelection
	preferences []domain.FXPreference
	workspace   string
	epoch       uint64
}

type reviewedMarketSyncKey struct{}
type reviewedMarketSync struct {
	marketData  marketDataSnapshot
	provider    domain.ReviewProviderSelection
	preferences []domain.FXPreference
	epoch       uint64
}

type reviewedMetalSourceKey struct{}

type marketSyncReview struct {
	preview  SyncPlanPreview
	work     marketSyncWork
	version  string
	quoteTTL time.Duration
	day      string
}

// ReviewMarketDataSync retains the exact provider work behind a desktop
// preview. It makes no provider requests and leaves scope-only planning for
// programmatic callers on PreviewMarketDataSync.
func (s *Service) ReviewMarketDataSync(ctx context.Context, request SyncRequest) (SyncPlanPreview, error) {
	var out SyncPlanPreview
	err := s.WithRead(ctx, func(ctx context.Context) error {
		started := s.clock()
		review := marketSyncReview{quoteTTL: s.QuoteCacheTTL()}
		review.work.provider = s.reviewProviderSnapshot()
		review.work.marketData = s.marketDataSnapshot(ctx, refreshTarget{kind: RefreshFXTarget})
		preview, err := s.previewMarketDataSync(ctx, request, &review.work)
		if err != nil {
			return err
		}
		review.work.preferences, err = s.repository.ListFXPreferences(ctx, review.work.household)
		if err != nil {
			return err
		}
		if review.work.provider != s.reviewProviderSnapshot() || review.quoteTTL != s.QuoteCacheTTL() {
			return staleMarketSyncReview()
		}
		day, _, err := domain.LocalCivilDate(started, review.work.zone)
		if err != nil {
			return err
		}
		currentDay, _, err := domain.LocalCivilDate(s.clock(), review.work.zone)
		if err != nil || day != currentDay {
			return staleMarketSyncReview()
		}
		preview.ReviewID = uuid.NewString()
		preview.ExpiresAt = preview.AsOf.Add(marketSyncReviewTTL)
		review.preview, review.day = preview, day
		review.version = s.MobileDataVersion()
		review.work.workspace, review.work.epoch = s.workspaceIdentity(), s.refreshEpoch.Load()
		s.syncMu.Lock()
		if s.syncReviews == nil {
			s.syncReviews = make(map[string]marketSyncReview)
		}
		maps.DeleteFunc(s.syncReviews, func(_ string, pending marketSyncReview) bool {
			return !preview.AsOf.Before(pending.preview.ExpiresAt) || pending.version != review.version
		})
		s.syncReviews[preview.ReviewID] = review
		s.syncMu.Unlock()
		out = preview
		return nil
	})
	return out, err
}

// ConfirmMarketDataSync validates the one-use review under the operation gate
// before admitting a job. An expired or changed review never starts a worker.
func (s *Service) ConfirmMarketDataSync(ctx context.Context, request SyncRequest, reviewID string) (SyncStartResult, error) {
	var result SyncStartResult
	var launch func()
	err := s.WithRead(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.syncMu.Lock()
		review, found := s.syncReviews[reviewID]
		s.syncMu.Unlock()
		if !found || !review.preview.Scope.Equivalent(request) {
			return staleMarketSyncReview()
		}
		s.reviewProviderMu.RLock()
		defer s.reviewProviderMu.RUnlock()
		now := s.clock()
		day, _, err := domain.LocalCivilDate(now, review.work.zone)
		if err != nil ||
			now.Before(review.preview.AsOf) || !now.Before(review.preview.ExpiresAt) || day != review.day ||
			review.version != s.MobileDataVersion() || review.work.workspace != s.workspaceIdentity() ||
			review.work.epoch != s.refreshEpoch.Load() || review.work.provider != s.reviewProviderSnapshotLocked() || review.quoteTTL != s.QuoteCacheTTL() {
			return staleMarketSyncReview()
		}
		result, launch, err = s.admitMarketDataSync(review.preview, review.work.household, &review.work)
		return err
	})
	if launch != nil {
		launch()
	}
	return result, err
}

func staleMarketSyncReview() error {
	return &domain.Error{Code: domain.ErrStalePreview, Field: "reviewId"}
}

func (s *Service) syncMarketRegistry(ctx context.Context) MarketDataRegistryPort {
	if reviewed, ok := ctx.Value(reviewedMarketSyncKey{}).(reviewedMarketSync); ok {
		return reviewed.marketData.registry
	}
	return s.MarketDataRegistry()
}

func (s *Service) syncFXProvider(ctx context.Context) string {
	if reviewed, ok := ctx.Value(reviewedMarketSyncKey{}).(reviewedMarketSync); ok {
		return reviewed.marketData.fxProviderKey
	}
	return s.FXProviderKey()
}

func (s *Service) syncFXPreferences(ctx context.Context, household domain.HouseholdID) ([]domain.FXPreference, error) {
	if reviewed, ok := ctx.Value(reviewedMarketSyncKey{}).(reviewedMarketSync); ok {
		return reviewed.preferences, nil
	}
	return s.repository.ListFXPreferences(ctx, household)
}

func (s *Service) requireReviewedInstrumentSource(ctx context.Context, approved domain.Instrument) error {
	current, err := s.repository.Instrument(ctx, approved.HouseholdID, approved.ID)
	if err != nil {
		return err
	}
	if current.ArchivedAt != nil || current.QuoteSource != domain.QuoteSourceProvider ||
		current.ProviderBindingRevision != approved.ProviderBindingRevision ||
		current.QuoteCurrency != approved.QuoteCurrency || current.Type != approved.Type ||
		instrumentMarket(current) != instrumentMarket(approved) {
		return staleMarketSyncReview()
	}
	return nil
}

// Hold the selected provider stable through a guarded write. The ordinary
// write permit used by the caller fences changes to the pair preference.
func (s *Service) withReviewedFXPolicy(ctx context.Context, household domain.HouseholdID, a, b domain.CurrencyCode, fn func() error) error {
	reviewed, ok := ctx.Value(reviewedMarketSyncKey{}).(reviewedMarketSync)
	if !ok {
		return fn()
	}
	s.reviewProviderMu.RLock()
	defer s.reviewProviderMu.RUnlock()
	if reviewed.provider != s.reviewProviderSnapshotLocked() {
		return staleMarketSyncReview()
	}
	current, err := s.repository.ListFXPreferences(ctx, household)
	if err != nil {
		return err
	}
	source := func(preferences []domain.FXPreference) domain.QuoteSourceKind {
		if preference := findFXPreference(preferences, a, b); preference != nil {
			return preference.SourceKind
		}
		return domain.QuoteSourceProvider
	}
	if source(reviewed.preferences) != source(current) {
		return staleMarketSyncReview()
	}
	return fn()
}

func (s *Service) persistLatestMarketQuote(ctx context.Context, epoch uint64, target refreshTarget, persist func(context.Context) error) error {
	return s.persistRefreshWrite(ctx, epoch, func(ctx context.Context) error {
		if _, reviewed := ctx.Value(reviewedMarketSyncKey{}).(reviewedMarketSync); !reviewed {
			return persist(ctx)
		}
		if target.kind == RefreshInstrumentTarget {
			if err := s.requireReviewedInstrumentSource(ctx, target.instrument); err != nil {
				return err
			}
			return s.withReviewedMetalFXPolicy(ctx, target.instrument, func() error { return persist(ctx) })
		}
		if metal, ok := ctx.Value(reviewedMetalSourceKey{}).(domain.Instrument); ok {
			if err := s.requireReviewedInstrumentSource(ctx, metal); err != nil {
				return err
			}
		}
		return s.withReviewedFXPolicy(ctx, target.householdID, target.baseCurrency, target.quoteCurrency, func() error { return persist(ctx) })
	})
}

func (s *Service) withReviewedMetalFXPolicy(ctx context.Context, instrument domain.Instrument, fn func() error) error {
	if instrument.UsesMetalConversion() && instrument.QuoteCurrency != "USD" {
		return s.withReviewedFXPolicy(ctx, instrument.HouseholdID, "USD", instrument.QuoteCurrency, fn)
	}
	return fn()
}
