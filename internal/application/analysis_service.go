package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// AnalysisService loads the immutable inputs for the Phase 1b calculation and
// owns the bounded memo consumed by the Phase 2a projections.
type AnalysisService struct {
	repository      Repository
	now             func() time.Time
	ensureSnapshots func(context.Context, string, string) error

	// memoMu protects the bounded memo and generation. The generation is
	// intentionally process-local: persisted analytics inputs remain the
	// source of truth and a process restart starts with an empty memo.
	memoMu                 sync.Mutex
	analysisDataGeneration uint64
	memo                   []analysisMemoEntry
}

const analysisMemoCapacity = 2

type analysisMemoEntry struct {
	key             string
	generation      uint64
	result          domain.PeriodAnalysisResult
	valuationForced string
}

// ComputeAnalysis exposes the pure Phase 1b kernel for deterministic tests
// and non-Wails callers.  Repository loading and closed-day maintenance stay
// in AnalysisService.Compute.
func ComputeAnalysis(input AnalysisInputs, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	return computeAnalysis(input, query)
}

func NewAnalysisService(repository Repository, clocks ...func() time.Time) *AnalysisService {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	return &AnalysisService{repository: repository, now: now}
}

func (s *AnalysisService) SetSnapshotEnsurer(ensure func(context.Context, string, string) error) {
	s.ensureSnapshots = ensure
}

func (s *AnalysisService) Compute(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	if err := query.Validate(); err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if s == nil || s.repository == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrUnavailable, Message: "analysis repository is not configured"}
	}
	key := analysisQueryHash(query)
	if result, _, ok := s.memoResult(key, false); ok {
		return result, nil
	}
	generation := s.memoGeneration()

	result, err := s.computeUncached(ctx, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	s.memoize(generation, analysisMemoEntry{key: key, generation: generation, result: result})
	return result, nil
}

// ComputeWithValuationFallback is the projection-facing calculation path for
// native valuation. Native is still rejected by the analysis kernel when the
// selected universe has multiple currencies, but the same loaded inputs are
// then computed once as base valuation. Both query keys are memoized so a
// repeated projection does not reload the portfolio and ledger.
func (s *AnalysisService) ComputeWithValuationFallback(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, string, error) {
	if err := query.Validate(); err != nil {
		return domain.PeriodAnalysisResult{}, "", err
	}
	if s == nil || s.repository == nil {
		return domain.PeriodAnalysisResult{}, "", &domain.Error{Code: domain.ErrUnavailable, Message: "analysis repository is not configured"}
	}
	if query.Valuation != domain.ValuationNative {
		result, err := s.Compute(ctx, query)
		return result, "", err
	}

	nativeKey := analysisQueryHash(query)
	if result, forced, ok := s.memoResult(nativeKey, true); ok {
		return result, forced, nil
	}
	generation := s.memoGeneration()
	input, err := s.loadInputs(ctx, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, "", err
	}
	result, err := ComputeAnalysis(input, query)
	if err == nil {
		s.memoize(generation, analysisMemoEntry{key: nativeKey, generation: generation, result: result})
		return result, "", nil
	}
	if !isNativeValuationError(err) {
		return domain.PeriodAnalysisResult{}, "", err
	}

	baseQuery := query
	baseQuery.Valuation = domain.ValuationBase
	result, err = ComputeAnalysis(input, baseQuery)
	if err != nil {
		return domain.PeriodAnalysisResult{}, "", err
	}
	baseKey := analysisQueryHash(baseQuery)
	s.memoize(
		generation,
		analysisMemoEntry{key: nativeKey, generation: generation, result: result, valuationForced: string(domain.ValuationBase)},
		analysisMemoEntry{key: baseKey, generation: generation, result: result},
	)
	return result, string(domain.ValuationBase), nil
}

func isNativeValuationError(err error) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr != nil && domainErr.Code == domain.ErrValidation && domainErr.Field == "valuation"
}

func (s *AnalysisService) memoResult(key string, includeForced bool) (domain.PeriodAnalysisResult, string, bool) {
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	generation := s.analysisDataGeneration
	for index := range s.memo {
		entry := s.memo[index]
		if entry.key != key || entry.generation != generation || (!includeForced && entry.valuationForced != "") {
			continue
		}
		if index > 0 {
			copy(s.memo[1:index+1], s.memo[0:index])
			s.memo[0] = entry
		}
		return entry.result, entry.valuationForced, true
	}
	return domain.PeriodAnalysisResult{}, "", false
}

func (s *AnalysisService) memoGeneration() uint64 {
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	return s.analysisDataGeneration
}

func (s *AnalysisService) memoize(generation uint64, entries ...analysisMemoEntry) {
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	// A successful mutation may have happened while the repository read was
	// in flight. Never publish that old-generation result into the new memo.
	if generation != s.analysisDataGeneration {
		return
	}
	// Insert in reverse so the caller's first entry remains the most recently
	// used one. This matters when a fallback publishes native and base aliases.
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		for memoIndex := range s.memo {
			if s.memo[memoIndex].key == entry.key {
				s.memo = append(s.memo[:memoIndex], s.memo[memoIndex+1:]...)
				break
			}
		}
		s.memo = append([]analysisMemoEntry{entry}, s.memo...)
	}
	if len(s.memo) > analysisMemoCapacity {
		s.memo = s.memo[:analysisMemoCapacity]
	}
}

// Invalidate drops all cached periods and advances the input generation. It
// is called only after an analytics input has been persisted successfully.
func (s *AnalysisService) Invalidate() {
	if s == nil {
		return
	}
	s.memoMu.Lock()
	s.analysisDataGeneration++
	s.memo = nil
	s.memoMu.Unlock()
}

func (s *AnalysisService) AnalysisDataGeneration() uint64 {
	if s == nil {
		return 0
	}
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	return s.analysisDataGeneration
}

func (s *AnalysisService) MemoEntryCount() int {
	if s == nil {
		return 0
	}
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	return len(s.memo)
}

func analysisQueryHash(query domain.AnalysisQuery) string {
	// Keep field order explicit. This is a canonical representation, not a
	// fmt dump of a struct whose formatting could change with a Go release.
	parts := []string{
		string(query.Scope.Kind), query.Scope.ID, query.From, query.To,
		string(query.Valuation), string(query.Basis), fmt.Sprintf("%t", query.IncludeCash),
	}
	filters := query.Filters
	parts = append(parts, optionalAnalysisID(filters.AccountID), optionalAnalysisCurrency(filters.Currency), strings.ToLower(strings.TrimSpace(filters.AssetClass)), optionalAnalysisID(filters.InstrumentID), optionalAnalysisID(filters.MemberID))
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func optionalAnalysisID[T interface{ String() string }](value *T) string {
	if value == nil {
		return ""
	}
	return (*value).String()
}

func optionalAnalysisCurrency(value *domain.CurrencyCode) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func (s *AnalysisService) computeUncached(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	input, err := s.loadInputs(ctx, query)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	return computeAnalysis(input, query)
}

func (s *AnalysisService) loadInputs(ctx context.Context, query domain.AnalysisQuery) (AnalysisInputs, error) {
	portfolio, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return AnalysisInputs{}, err
	}
	if portfolio.Household == nil {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
	}
	origin := portfolio.Origin
	if origin == nil {
		origin, err = s.repository.HistoryOrigin(ctx, portfolio.Household.ID)
		if err != nil {
			return AnalysisInputs{}, err
		}
	}
	if origin == nil {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before analyzing"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	today := s.now().In(location).Format("2006-01-02")
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if query.From < domain.LocalDate(originDate) {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "from", Message: "analysis range precedes the Starting point"}
	}
	if query.To >= domain.LocalDate(today) {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "to", Message: "analysis range must end on a closed day"}
	}
	if s.ensureSnapshots != nil {
		snapshotFrom := string(query.From)
		if query.From > domain.LocalDate(originDate) {
			fromDate, parseErr := time.Parse("2006-01-02", string(query.From))
			if parseErr == nil {
				candidate := fromDate.AddDate(0, 0, -1).Format("2006-01-02")
				if candidate > originDate {
					snapshotFrom = candidate
				} else {
					snapshotFrom = originDate
				}
			}
		}
		if err := s.ensureSnapshots(ctx, snapshotFrom, string(query.To)); err != nil {
			return AnalysisInputs{}, err
		}
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, portfolio.Household.ID, time.Time{})
	if err != nil {
		return AnalysisInputs{}, err
	}
	endLocal, err := time.ParseInLocation("2006-01-02", string(query.To), location)
	if err != nil {
		return AnalysisInputs{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "date must use YYYY-MM-DD"}
	}
	nextMidnight, err := domain.ResolveLocalDateTime(endLocal.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", origin.Timezone)
	if err != nil {
		return AnalysisInputs{}, err
	}
	activities, err := s.repository.ListActivitiesUntil(ctx, portfolio.Household.ID, nextMidnight.Add(-time.Millisecond))
	if err != nil {
		return AnalysisInputs{}, err
	}
	fxQuotes, err := s.repository.ListFXQuotes(ctx, portfolio.Household.ID)
	if err != nil {
		return AnalysisInputs{}, err
	}
	instrumentQuotes := make([]domain.InstrumentQuote, 0)
	for _, instrument := range portfolio.Instruments {
		quotes, quoteErr := s.repository.ListInstrumentQuotes(ctx, instrument.ID)
		if quoteErr != nil {
			return AnalysisInputs{}, quoteErr
		}
		instrumentQuotes = append(instrumentQuotes, quotes...)
	}
	return AnalysisInputs{Origin: *origin, Portfolio: portfolio, Snapshots: snapshots, Activities: activities, InstrumentQuotes: instrumentQuotes, FXQuotes: fxQuotes}, nil
}

// Analyze is the application-facing convenience method used by future Wails
// services. It does not expose a second calculation path.
func (s *Service) Analyze(ctx context.Context, query domain.AnalysisQuery) (domain.PeriodAnalysisResult, error) {
	if s == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrUnavailable, Message: "application service is not configured"}
	}
	return s.analysisService().Compute(ctx, query)
}

func (s *Service) analysisService() *AnalysisService {
	if s.analysis == nil {
		analysis := NewAnalysisService(s.repository, s.clock)
		analysis.SetSnapshotEnsurer(s.ensureClosedDaySnapshots)
		s.analysis = analysis
	}
	return s.analysis
}

// invalidateAnalysis is intentionally best-effort and has no repository
// side effects. Callers invoke it only after a mutation has been persisted so
// a failed or preview operation cannot invalidate a valid memo.
func (s *Service) invalidateAnalysis() {
	if s != nil && s.analysis != nil {
		s.analysis.Invalidate()
	}
}
