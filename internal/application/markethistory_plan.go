package application

import (
	"context"
	"errors"
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
	FetchRanges          []DateRange
	RouteStatus          string
	SkipReason           string
}

type HistoryRepairPlan struct {
	HouseholdID             domain.HouseholdID
	OriginLocalDate         string
	YesterdayLocal          string
	LastFinalizedMarketDate string
	ResolverPolicyVersion   string
	Instruments             []InstrumentRepairNeed
	ManualFX                bool
	ForceRecheck            bool
}

type HistorySyncOptions struct {
	ForceRecheck bool
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

// PlanHistorySync overlays historical routing, negative-cache expiry,
// recent-correction checks, and optional Force Recheck on the coverage plan.
// Current manual instruments are already omitted from coverage. It does not
// perform provider HTTP.
func (s *Service) PlanHistorySync(ctx context.Context, opts HistorySyncOptions) (HistoryRepairPlan, error) {
	plan, err := s.PlanMarketDataRepair(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	plan.ForceRecheck = opts.ForceRecheck
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	coverage, err := s.repository.ListInstrumentHistoryCoverage(ctx, household.ID)
	if err != nil {
		return HistoryRepairPlan{}, err
	}
	byID := make(map[domain.InstrumentID]domain.InstrumentHistoryCoverage, len(coverage))
	for _, item := range coverage {
		byID[item.InstrumentID] = item
	}
	now := s.clock()
	for index, need := range plan.Instruments {
		item := byID[need.InstrumentID]
		enriched, enrichErr := applyHistorySyncPolicy(need, item, plan.LastFinalizedMarketDate, now, opts.ForceRecheck)
		if enrichErr != nil {
			return HistoryRepairPlan{}, enrichErr
		}
		plan.Instruments[index] = enriched
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
	if !missing && anchor != "" {
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
		RouteStatus:          domain.InstrumentRouteOK,
	}, nil
}

func applyHistorySyncPolicy(need InstrumentRepairNeed, coverage domain.InstrumentHistoryCoverage, lastFinalized string, now time.Time, force bool) (InstrumentRepairNeed, error) {
	route := domain.ResolveInstrumentRoute(coverage.Market, coverage.ProviderKey, coverage.ProviderSymbol)
	need.RouteStatus = route.Status
	need.SkipReason = route.Reason
	if route.Status != domain.InstrumentRouteOK {
		need.FetchRanges = nil
		return need, nil
	}
	dates, err := InclusiveMarketDates(need.FetchRange)
	if err != nil {
		return InstrumentRepairNeed{}, err
	}
	fetchDates := make([]string, 0)
	for _, date := range dates {
		label := string(date)
		_, hasClose := indexStrings(coverage.CloseMarketDates)[label]
		_, hasNoObs := indexStrings(coverage.NoObservationDates)[label]
		expires, hasExpiry := coverage.NoObservationExpiresAt[label]
		decision := domain.DecideHistoryFetch(domain.HistoryFetchInput{
			Date:                   label,
			LastFinalized:          lastFinalized,
			Now:                    now,
			ForceRecheck:           force,
			HasClose:               hasClose,
			CloseFetchedAt:         coverage.CloseFetchedAt[label],
			HasNoObservation:       hasNoObs,
			NoObservationExpiresAt: expires,
			NoObservationHasExpiry: hasExpiry,
			NoObservationCheckedAt: coverage.NoObservationCheckedAt[label],
		})
		if decision.Action == domain.HistoryFetch {
			fetchDates = append(fetchDates, label)
		}
	}
	need.FetchRanges = dateRangesFromDates(fetchDates)
	return need, nil
}

func indexStrings(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
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
	appended := 0
	for chunkStart := from; chunkStart <= to; {
		end := chunkStart
		var err error
		for step := 0; step < 30 && end < to; step++ {
			end, err = nextRebuildDate(end)
			if err != nil {
				return appended, err
			}
		}
		if end > to {
			end = to
		}
		var chunkDone bool
		for attempt := 0; attempt < 3 && !chunkDone; attempt++ {
			chunkState, stateErr := s.repository.DailySnapshotState(ctx, household.ID)
			if stateErr != nil {
				return appended, stateErr
			}
			count, rebuildErr := s.RebuildHistoricalSnapshots(ctx, chunkStart, end)
			if rebuildErr != nil {
				if isSnapshotGenerationChanged(rebuildErr) {
					continue
				}
				return appended + count, rebuildErr
			}
			if generationRepo, ok := s.repository.(GenerationAwareSnapshotRepository); ok {
				completeErr := s.WithWrite(ctx, func(writeCtx context.Context) error {
					return generationRepo.CompleteDailySnapshotRangeAtGeneration(writeCtx, household.ID, end, s.clock(), chunkState.InputGeneration)
				})
				if isSnapshotGenerationChanged(completeErr) {
					continue
				}
				if completeErr != nil {
					return appended, completeErr
				}
			} else {
				after, stateErr := s.repository.DailySnapshotState(ctx, household.ID)
				if stateErr != nil {
					return appended, stateErr
				}
				if after.InputGeneration != chunkState.InputGeneration {
					continue
				}
				if err := s.CompleteDailySnapshotRange(ctx, household.ID, end); err != nil {
					return appended, err
				}
			}
			appended += count
			chunkDone = true
		}
		if !chunkDone {
			// The dirty cursor is intentionally left in place. The next sync can
			// retry with a stable input generation rather than publishing a
			// result whose provenance is already stale.
			return appended, nil
		}
		if end == to {
			break
		}
		chunkStart, err = nextRebuildDate(end)
		if err != nil {
			return appended, err
		}
	}
	return appended, nil
}

func isSnapshotGenerationChanged(err error) bool {
	var domainErr *domain.Error
	return err != nil && errors.As(err, &domainErr) && domainErr != nil && domainErr.Field == "inputGeneration"
}

func nextRebuildDate(value string) (string, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "snapshot date range is invalid"}
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02"), nil
}

// HouseholdCutoffAt is the exclusive end of a closed local day used by
// snapshot eligibility. Quotes are selected with ObservationEligible, never
// by market-date equality.
func HouseholdCutoffAt(localDate, timezone string) (time.Time, error) {
	return domain.HouseholdDayCutoff(localDate, timezone)
}
