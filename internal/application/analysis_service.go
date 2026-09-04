package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// AnalysisService loads the immutable inputs for the Phase 1a calculation.
// It intentionally has no Wails or projection concerns: later phases consume
// PeriodAnalysisResult rather than replaying the ledger per view.
type AnalysisService struct {
	repository      Repository
	now             func() time.Time
	ensureSnapshots func(context.Context, string, string) error
}

// ComputeAnalysis exposes the pure Phase 1a kernel for deterministic tests
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
	portfolio, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	if portfolio.Household == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrNotFound, Message: "household was not found"}
	}
	origin := portfolio.Origin
	if origin == nil {
		origin, err = s.repository.HistoryOrigin(ctx, portfolio.Household.ID)
		if err != nil {
			return domain.PeriodAnalysisResult{}, err
		}
	}
	if origin == nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before analyzing"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	today := s.now().In(location).Format("2006-01-02")
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	if query.From < domain.LocalDate(originDate) {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "from", Message: "analysis range precedes the Starting point"}
	}
	if query.To >= domain.LocalDate(today) {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "to", Message: "analysis range must end on a closed day"}
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
			return domain.PeriodAnalysisResult{}, err
		}
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, portfolio.Household.ID, time.Time{})
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	endLocal, err := time.ParseInLocation("2006-01-02", string(query.To), location)
	if err != nil {
		return domain.PeriodAnalysisResult{}, &domain.Error{Code: domain.ErrValidation, Field: "to", Message: "date must use YYYY-MM-DD"}
	}
	nextMidnight, err := domain.ResolveLocalDateTime(endLocal.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", origin.Timezone)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	activities, err := s.repository.ListActivitiesUntil(ctx, portfolio.Household.ID, nextMidnight.Add(-time.Millisecond))
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	fxQuotes, err := s.repository.ListFXQuotes(ctx, portfolio.Household.ID)
	if err != nil {
		return domain.PeriodAnalysisResult{}, err
	}
	instrumentQuotes := make([]domain.InstrumentQuote, 0)
	for _, instrument := range portfolio.Instruments {
		quotes, quoteErr := s.repository.ListInstrumentQuotes(ctx, instrument.ID)
		if quoteErr != nil {
			return domain.PeriodAnalysisResult{}, quoteErr
		}
		instrumentQuotes = append(instrumentQuotes, quotes...)
	}
	return computeAnalysis(AnalysisInputs{Origin: *origin, Portfolio: portfolio, Snapshots: snapshots, Activities: activities, InstrumentQuotes: instrumentQuotes, FXQuotes: fxQuotes}, query)
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
