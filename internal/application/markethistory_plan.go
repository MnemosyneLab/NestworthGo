package application

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type InstrumentRepairNeed struct {
	InstrumentID         domain.InstrumentID
	ProviderKey          string
	ProviderSymbol       string
	Market               string
	OpeningAnchorDate    string
	OpeningAnchorMissing bool
	FetchRange           DateRange
	MissingRanges        []DateRange
}

type HistoryRepairPlan struct {
	HouseholdID             domain.HouseholdID
	OriginLocalDate         string
	YesterdayLocal          string
	LastFinalizedMarketDate string
	ResolverPolicyVersion   string
	Instruments             []InstrumentRepairNeed
	ManualFX                bool
}

// PlanMarketDataRepair builds the coverage/gap and opening-anchor plan for
// the current household without performing provider HTTP.
func (s *Service) PlanMarketDataRepair(ctx context.Context) (HistoryRepairPlan, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	if origin == nil {
		return HistoryRepairPlan{}, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before planning market-data repair"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return HistoryRepairPlan{}, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	now := s.clock()
	today := now.In(location).Format("2006-01-02")
	yesterday, err := previousLocalDate(today)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	finalized, err := domain.LastFinalizedUSEquityMarketDate(now)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	coverage, err := s.repository.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	preferences, err := s.repository.ListFXPreferences(ctx, household.ID)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	plan := HistoryRepairPlan{
		HouseholdID:             household.ID,
		OriginLocalDate:         originDate,
		YesterdayLocal:          yesterday,
		LastFinalizedMarketDate: finalized,
		ResolverPolicyVersion:   domain.MarketDataResolverPolicy,
		ManualFX:                hasManualFX(preferences),
	}
	for _, item := range coverage {
		need, planErr := planInstrumentRepairNeed(item, originDate, finalized)
		if planErr != nil {
			return HistoryRepairPlan{}, planErr
		}
		plan.Instruments = append(plan.Instruments, need)
	}
	return plan, nil
}

func planInstrumentRepairNeed(coverage domain.InstrumentHistoryCoverage, originDate, lastFinalized string) (InstrumentRepairNeed, error) {
	closes := make([]domain.OracleClose, 0, len(coverage.CloseMarketDates))
	for _, date := range coverage.CloseMarketDates {
		closes = append(closes, domain.OracleClose{MarketDate: date})
	}
	anchor, missing := domain.FindOpeningAnchor(originDate, closes)
	lookbackStart := originDate
	if windows := domain.OpeningAnchorLookbackWindows(); len(windows) > 0 {
		parsed, err := time.Parse("2006-01-02", originDate)
		if err != nil {
			return InstrumentRepairNeed{}, err
		}
		lookbackStart = parsed.AddDate(0, 0, -windows[len(windows)-1]).Format("2006-01-02")
	}
	fetchStart := lookbackStart
	if !missing && anchor != "" && anchor < fetchStart {
		fetchStart = anchor
	}
	if lastFinalized < fetchStart {
		return InstrumentRepairNeed{}, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "last finalized market date precedes the required fetch start"}
	}
	covered := make(map[string]struct{}, len(coverage.CloseMarketDates)+len(coverage.NoObservationDates))
	for _, date := range coverage.CloseMarketDates {
		covered[date] = struct{}{}
	}
	for _, date := range coverage.NoObservationDates {
		covered[date] = struct{}{}
	}
	required, err := domain.InclusiveMarketDates(fetchStart, lastFinalized)
	if err != nil {
		return InstrumentRepairNeed{}, err
	}
	missingDates := make([]string, 0)
	for _, date := range required {
		if _, ok := covered[date]; !ok {
			missingDates = append(missingDates, date)
		}
	}
	return InstrumentRepairNeed{
		InstrumentID:         coverage.InstrumentID,
		ProviderKey:          coverage.ProviderKey,
		ProviderSymbol:       coverage.ProviderSymbol,
		Market:               coverage.Market,
		OpeningAnchorDate:    anchor,
		OpeningAnchorMissing: missing,
		FetchRange:           DateRange{Start: MarketDate(fetchStart), End: MarketDate(lastFinalized)},
		MissingRanges:        dateRangesFromDates(missingDates),
	}, nil
}

func dateRangesFromDates(dates []string) []DateRange {
	if len(dates) == 0 {
		return nil
	}
	ranges := make([]DateRange, 0, 1)
	start, prev := dates[0], dates[0]
	for _, date := range dates[1:] {
		parsed, err := time.Parse("2006-01-02", prev)
		if err != nil || date != parsed.AddDate(0, 0, 1).Format("2006-01-02") {
			ranges = append(ranges, DateRange{Start: MarketDate(start), End: MarketDate(prev)})
			start = date
		}
		prev = date
	}
	return append(ranges, DateRange{Start: MarketDate(start), End: MarketDate(prev)})
}

func hasManualFX(preferences []domain.FXPreference) bool {
	for _, preference := range preferences {
		if preference.SourceKind == domain.QuoteSourceManual {
			return true
		}
	}
	return false
}

func previousLocalDate(localDate string) (string, error) {
	parsed, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "localDate", Message: "local date must use YYYY-MM-DD"}
	}
	return parsed.AddDate(0, 0, -1).Format("2006-01-02"), nil
}

// RebuildDirtySnapshots rebuilds closed household days in the dirty range.
// dirty_to is the inclusive upper bound; it is clamped to yesterday.
func (s *Service) RebuildDirtySnapshots(ctx context.Context) (int, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return 0, err
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return 0, err
	}
	if origin == nil {
		return 0, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "start history before rebuilding snapshots"}
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return 0, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	state, err := s.repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		return 0, err
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	yesterday, err := previousLocalDate(s.clock().In(location).Format("2006-01-02"))
	if err != nil {
		return 0, err
	}
	dirtyFrom := ""
	if state.DirtyFrom != nil {
		dirtyFrom = strings.TrimSpace(*state.DirtyFrom)
	}
	if dirtyFrom == "" {
		return 0, nil
	}
	from := originDate
	if dirtyFrom > from {
		from = dirtyFrom
	}
	to := yesterday
	if state.DirtyTo != nil && strings.TrimSpace(*state.DirtyTo) != "" && *state.DirtyTo < to {
		to = *state.DirtyTo
	}
	if from < originDate {
		from = originDate
	}
	if to > yesterday {
		to = yesterday
	}
	if from > to {
		return 0, nil
	}
	generation := state.InputGeneration
	appended, err := s.RebuildHistoricalSnapshots(ctx, from, to)
	if err != nil {
		return appended, err
	}
	after, err := s.repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		return appended, err
	}
	if after.InputGeneration != generation {
		// Inputs advanced during rebuild; keep remaining dirty rather than
		// claiming the published generation is current.
		return appended, nil
	}
	if err := s.CompleteDailySnapshotRange(ctx, household.ID, to); err != nil {
		return appended, err
	}
	return appended, nil
}

// HouseholdCutoffAt is the exclusive end of a closed local day used by
// snapshot eligibility. Quotes are selected with ObservationEligible, never
// by market-date equality.
func HouseholdCutoffAt(localDate, timezone string) (time.Time, error) {
	return domain.HouseholdDayCutoff(localDate, timezone)
}
