package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	HealthKindMissingInstrumentHistory = "missing_instrument_history"
	HealthKindMissingFXHistory         = "missing_fx_history"
	HealthKindMissingManualPrice       = "missing_manual_price"
	HealthKindMissingManualFX          = "missing_manual_fx"
	HealthKindMissingProviderKey       = "missing_provider_key"
	HealthKindMissingBinding           = "missing_binding"
	HealthKindUnsupportedCoverage      = "unsupported_coverage"
	HealthKindIncompleteValuation      = "incomplete_valuation"
	HealthKindSnapshotMissing          = "snapshot_missing"
	HealthKindSnapshotOutdated         = "snapshot_outdated"
	HealthKindSyncFailure              = "sync_failure"
	HealthKindHistoryNotStarted        = "history_not_started"

	HealthSeverityBlocking = "blocking"
	HealthSeverityWarning  = "warning"
	HealthSeverityInfo     = "info"

	HealthActionRepair           = "repair"
	HealthActionProviderSettings = "provider_settings"
	HealthActionInstrumentEditor = "instrument_editor"
	HealthActionManualEntry      = "manual_entry"
	HealthActionNone             = "none"
)

// HealthIssue is one locally diagnosed market-data integrity finding.
type HealthIssue struct {
	ID           string
	Kind         string
	Severity     string
	GroupKey     string
	TargetKey    string
	Label        string
	Provider     string
	InstrumentID string
	CurrencyA    string
	CurrencyB    string
	RangeStart   string
	RangeEnd     string
	RangeCount   int
	Code         string
	Reason       string
	Action       string
	Executable   bool
	Collapsed    bool
}

// MarketDataHealthReport is the local Data Health scan result. ScanMarketDataHealth
// never performs provider HTTP.
type MarketDataHealthReport struct {
	Healthy                 bool
	IncompleteSince         string
	CoverageThrough         string
	LastFinalizedMarketDate string
	IssueCount              int
	ExecutableCount         int
	PrerequisiteCount       int
	SnapshotDays            int
	Issues                  []HealthIssue
}

// ScanMarketDataHealth inspects local coverage, routing, secrets, valuation
// missing inputs, dirty snapshots, and the last in-process sync job. It never
// calls LatestInstrument, LatestFX, InstrumentDailyHistory, or FXDailyHistory.
func (s *Service) ScanMarketDataHealth(ctx context.Context) (MarketDataHealthReport, error) {
	household, err := s.requireHousehold(ctx)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	origin, err := s.HistoryOrigin(ctx)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	if origin == nil {
		issue := HealthIssue{
			ID:        HealthKindHistoryNotStarted,
			Kind:      HealthKindHistoryNotStarted,
			Severity:  HealthSeverityInfo,
			GroupKey:  HealthKindHistoryNotStarted,
			TargetKey: "history",
			Code:      string(domain.ErrHistoryNotStarted),
			Reason:    "history_not_started",
			Action:    HealthActionNone,
		}
		return finishHealthReport(MarketDataHealthReport{Issues: []HealthIssue{issue}}), nil
	}

	plan, err := s.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	instruments, err := s.repository.ListInstruments(ctx, household.ID, false)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	names := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range instruments {
		names[instrument.ID] = instrument
	}

	var issues []HealthIssue
	blockedProviders := s.localProviderBlocks(ctx, plan.Instruments)
	coveredInstruments := map[string]struct{}{}

	for _, need := range plan.Instruments {
		instrument := names[need.InstrumentID]
		label := instrument.Name
		if label == "" {
			label = need.ProviderSymbol
		}
		if label == "" {
			label = need.InstrumentID.String()
		}
		issue, blocked := classifyInstrumentHealth(need, label, blockedProviders)
		if issue.Kind == "" {
			continue
		}
		issues = append(issues, issue)
		coveredInstruments[need.InstrumentID.String()] = struct{}{}
		if blocked {
			coveredInstruments["provider:"+strings.ToLower(need.ProviderKey)] = struct{}{}
		}
	}
	for provider, block := range blockedProviders {
		if !block.used {
			continue
		}
		issues = append([]HealthIssue{{
			ID:         "provider-key-" + provider,
			Kind:       HealthKindMissingProviderKey,
			Severity:   HealthSeverityBlocking,
			GroupKey:   "provider_key:" + provider,
			TargetKey:  "provider:" + provider,
			Label:      provider,
			Provider:   provider,
			Code:       ProviderConfigMissingKey,
			Reason:     block.reason,
			Action:     HealthActionProviderSettings,
			Executable: false,
		}}, issues...)
	}

	manualIssues, err := s.scanManualInstrumentHealth(ctx, origin, plan, instruments, coveredInstruments)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	issues = append(issues, manualIssues...)

	fxIssues, fxCovered, err := s.scanFXHealth(ctx, household.ID, origin, plan)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	issues = append(issues, fxIssues...)

	valuationIssues, err := s.scanValuationHealth(ctx, coveredInstruments, fxCovered)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	issues = append(issues, valuationIssues...)

	rootCause := hasUncollapsedRootCause(issues)
	state, err := s.repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		return MarketDataHealthReport{}, err
	}
	snapshotIssues := scanSnapshotHealth(state, plan, rootCause)
	issues = append(issues, snapshotIssues...)

	if job, ok := s.GetCurrentSyncJob(); ok {
		issues = append(issues, scanSyncFailureHealth(job, coveredInstruments, fxCovered)...)
	}

	report := MarketDataHealthReport{
		CoverageThrough:         plan.LastFinalizedMarketDate,
		LastFinalizedMarketDate: plan.LastFinalizedMarketDate,
		SnapshotDays:            estimateDirtyDays(state),
		Issues:                  issues,
	}
	if !rootCause {
		report.SnapshotDays = countedSnapshotDays(snapshotIssues)
	} else {
		report.SnapshotDays = 0
	}
	return finishHealthReport(report), nil
}

type providerBlock struct {
	reason string
	used   bool
}

func (s *Service) localProviderBlocks(ctx context.Context, needs []InstrumentRepairNeed) map[string]providerBlock {
	blocks := map[string]providerBlock{}
	registry := s.MarketDataRegistry()
	if registry == nil {
		return blocks
	}
	seen := map[string]struct{}{}
	for _, need := range needs {
		provider := strings.ToLower(strings.TrimSpace(need.ProviderKey))
		if provider == "" || need.RouteStatus != domain.InstrumentRouteOK {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		resolved, err := registry.Resolve(provider)
		if err != nil || resolved == nil {
			continue
		}
		inspector, ok := resolved.(ProviderLocalStatus)
		if !ok {
			continue
		}
		code, reason := inspector.LocalConfigStatus(ctx)
		if code == "" || code == ProviderConfigOK {
			continue
		}
		blocks[provider] = providerBlock{reason: reason}
	}
	for index := range needs {
		provider := strings.ToLower(strings.TrimSpace(needs[index].ProviderKey))
		if block, ok := blocks[provider]; ok {
			block.used = true
			blocks[provider] = block
		}
	}
	return blocks
}

func classifyInstrumentHealth(need InstrumentRepairNeed, label string, blocked map[string]providerBlock) (HealthIssue, bool) {
	target := instrumentTargetKey(need.InstrumentID)
	provider := strings.ToLower(strings.TrimSpace(need.ProviderKey))
	base := HealthIssue{
		ID:           "instrument-" + need.InstrumentID.String(),
		TargetKey:    target,
		Label:        label,
		Provider:     provider,
		InstrumentID: need.InstrumentID.String(),
		GroupKey:     "instrument:" + need.InstrumentID.String(),
	}
	if need.RouteStatus == domain.InstrumentRouteBindingMissing {
		base.Kind = HealthKindMissingBinding
		base.Severity = HealthSeverityBlocking
		base.Code = domain.InstrumentRouteBindingMissing
		base.Reason = need.SkipReason
		base.Action = HealthActionInstrumentEditor
		return base, false
	}
	if need.RouteStatus == domain.InstrumentRouteUnsupported {
		if !hasInstrumentGap(need) {
			return HealthIssue{}, false
		}
		span := firstRange(need.MissingRanges, need.FetchRanges)
		base.Kind = HealthKindUnsupportedCoverage
		base.Severity = HealthSeverityWarning
		base.Code = domain.InstrumentRouteUnsupported
		base.Reason = need.SkipReason
		base.Action = HealthActionNone
		base.RangeStart = string(span.Start)
		base.RangeEnd = string(span.End)
		base.RangeCount = len(need.MissingRanges)
		return base, false
	}
	if block, ok := blocked[provider]; ok {
		span := firstRange(need.MissingRanges, need.FetchRanges)
		base.Kind = HealthKindMissingInstrumentHistory
		base.Severity = HealthSeverityBlocking
		base.Code = ProviderConfigMissingKey
		base.Reason = block.reason
		base.Action = HealthActionProviderSettings
		base.GroupKey = "provider_key:" + provider
		base.Collapsed = true
		base.RangeStart = string(span.Start)
		base.RangeEnd = string(span.End)
		base.RangeCount = len(need.MissingRanges)
		return base, true
	}
	if !hasInstrumentGap(need) {
		return HealthIssue{}, false
	}
	span := firstRange(need.MissingRanges, need.FetchRanges)
	if !instrumentHistoryAutoRepairable(provider) {
		base.Kind = HealthKindUnsupportedCoverage
		base.Severity = HealthSeverityWarning
		base.Code = "unsupported_price_basis"
		base.Reason = "unsupported_price_basis"
		base.Action = HealthActionNone
		base.RangeStart = string(span.Start)
		base.RangeEnd = string(span.End)
		base.RangeCount = max(len(need.MissingRanges), len(need.FetchRanges))
		return base, false
	}
	base.Kind = HealthKindMissingInstrumentHistory
	base.Severity = HealthSeverityBlocking
	base.Code = "missing_history"
	base.Reason = "missing_history"
	base.Action = HealthActionRepair
	base.Executable = true
	base.RangeStart = string(span.Start)
	base.RangeEnd = string(span.End)
	base.RangeCount = max(len(need.MissingRanges), len(need.FetchRanges))
	return base, false
}

func (s *Service) scanManualInstrumentHealth(ctx context.Context, origin *domain.HistoryOrigin, plan HistoryRepairPlan, instruments []domain.Instrument, covered map[string]struct{}) ([]HealthIssue, error) {
	if origin == nil {
		return nil, nil
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return nil, &domain.Error{Code: domain.ErrHistoryTimezoneRequired, Message: "stored Household timezone is invalid"}
	}
	snapshot, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	held := map[domain.InstrumentID]struct{}{}
	for _, holding := range snapshot.Holdings {
		if holding.ArchivedAt != nil {
			continue
		}
		held[holding.InstrumentID] = struct{}{}
	}
	originDate := origin.StartedAt.In(location).Format("2006-01-02")
	var issues []HealthIssue
	for _, instrument := range instruments {
		if instrument.QuoteSource != domain.QuoteSourceManual {
			continue
		}
		if _, ok := held[instrument.ID]; !ok {
			continue
		}
		if _, ok := covered[instrument.ID.String()]; ok {
			continue
		}
		quotes, err := s.repository.ListInstrumentQuotes(ctx, instrument.ID)
		if err != nil {
			return nil, err
		}
		if hasQuoteOnOrBefore(quotes, originDate, location) {
			continue
		}
		covered[instrument.ID.String()] = struct{}{}
		issues = append(issues, HealthIssue{
			ID:           "manual-" + instrument.ID.String(),
			Kind:         HealthKindMissingManualPrice,
			Severity:     HealthSeverityBlocking,
			GroupKey:     "instrument:" + instrument.ID.String(),
			TargetKey:    instrumentTargetKey(instrument.ID),
			Label:        instrument.Name,
			InstrumentID: instrument.ID.String(),
			RangeStart:   originDate,
			RangeEnd:     plan.LastFinalizedMarketDate,
			Code:         "missing_manual_price",
			Reason:       "opening_anchor_missing",
			Action:       HealthActionManualEntry,
		})
	}
	return issues, nil
}

func (s *Service) scanFXHealth(ctx context.Context, householdID domain.HouseholdID, origin *domain.HistoryOrigin, plan HistoryRepairPlan) ([]HealthIssue, map[string]struct{}, error) {
	covered := map[string]struct{}{}
	prefs, err := s.repository.ListFXPreferences(ctx, householdID)
	if err != nil {
		return nil, covered, err
	}
	if len(prefs) == 0 {
		return nil, covered, nil
	}
	quotes, err := s.repository.ListFXQuotes(ctx, householdID)
	if err != nil {
		return nil, covered, err
	}
	coverage, err := s.repository.ListFXHistoryCoverage(ctx, householdID)
	if err != nil {
		return nil, covered, err
	}
	coverageByPair := map[string]domain.FXHistoryCoverage{}
	for _, item := range coverage {
		coverageByPair[fxPairKey(item.BaseCurrency, item.QuoteCurrency)] = item
	}
	location := time.UTC
	if origin != nil {
		if loaded, locErr := time.LoadLocation(origin.Timezone); locErr == nil {
			location = loaded
		}
	}
	var issues []HealthIssue
	providerKey := strings.ToLower(strings.TrimSpace(s.FXProviderKey()))
	for _, preference := range prefs {
		pair := fxPairKey(preference.CurrencyA, preference.CurrencyB)
		label := pair
		if preference.SourceKind == domain.QuoteSourceManual {
			if hasFXQuoteOnOrBefore(quotes, preference.CurrencyA, preference.CurrencyB, plan.OriginLocalDate, location) {
				continue
			}
			covered[pair] = struct{}{}
			issues = append(issues, HealthIssue{
				ID:         "manual-fx-" + pair,
				Kind:       HealthKindMissingManualFX,
				Severity:   HealthSeverityBlocking,
				GroupKey:   "fx:" + pair,
				TargetKey:  "fx:" + pair,
				Label:      label,
				CurrencyA:  preference.CurrencyA.String(),
				CurrencyB:  preference.CurrencyB.String(),
				RangeStart: plan.OriginLocalDate,
				RangeEnd:   plan.LastFinalizedMarketDate,
				Code:       "missing_manual_fx",
				Reason:     "opening_anchor_missing",
				Action:     HealthActionManualEntry,
			})
			continue
		}
		if providerKey == "" {
			covered[pair] = struct{}{}
			issues = append(issues, HealthIssue{
				ID:        "fx-provider-" + pair,
				Kind:      HealthKindMissingProviderKey,
				Severity:  HealthSeverityBlocking,
				GroupKey:  "fx_provider",
				TargetKey: "fx:" + pair,
				Label:     label,
				CurrencyA: preference.CurrencyA.String(),
				CurrencyB: preference.CurrencyB.String(),
				Code:      string(domain.ErrUnavailable),
				Reason:    "provider_not_configured",
				Action:    HealthActionProviderSettings,
			})
			continue
		}
		item := coverageByPair[pair]
		missing := missingFXDates(plan.OriginLocalDate, plan.LastFinalizedMarketDate, item)
		if len(missing) == 0 {
			continue
		}
		ranges := dateRangesFromDates(missing)
		span := firstRange(ranges, nil)
		covered[pair] = struct{}{}
		issue := HealthIssue{
			ID:         "fx-" + pair,
			TargetKey:  "fx:" + pair,
			Label:      label,
			Provider:   providerKey,
			CurrencyA:  preference.CurrencyA.String(),
			CurrencyB:  preference.CurrencyB.String(),
			GroupKey:   "fx:" + pair,
			RangeStart: string(span.Start),
			RangeEnd:   string(span.End),
			RangeCount: len(ranges),
		}
		if !fxHistoryAutoRepairable(providerKey) {
			issue.Kind = HealthKindUnsupportedCoverage
			issue.Severity = HealthSeverityWarning
			issue.Code = "unsupported_fx_history"
			issue.Reason = "unsupported_fx_history"
			issue.Action = HealthActionNone
			issues = append(issues, issue)
			continue
		}
		issue.Kind = HealthKindMissingFXHistory
		issue.Severity = HealthSeverityBlocking
		issue.Code = "missing_history"
		issue.Reason = "missing_history"
		issue.Action = HealthActionRepair
		issue.Executable = true
		issues = append(issues, issue)
	}
	return issues, covered, nil
}

func (s *Service) scanValuationHealth(ctx context.Context, instruments map[string]struct{}, fxCovered map[string]struct{}) ([]HealthIssue, error) {
	overview, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		return nil, err
	}
	var issues []HealthIssue
	for _, missing := range overview.MissingInputs {
		switch missing.Kind {
		case domain.MissingInstrumentPrice:
			if missing.InstrumentID != nil {
				if _, ok := instruments[missing.InstrumentID.String()]; ok {
					continue
				}
				instruments[missing.InstrumentID.String()] = struct{}{}
			}
			action := HealthActionRepair
			kind := HealthKindIncompleteValuation
			executable := true
			if missing.QuoteSource == domain.QuoteSourceManual {
				action = HealthActionManualEntry
				kind = HealthKindMissingManualPrice
				executable = false
			}
			label := missing.InstrumentName
			id := ""
			if missing.InstrumentID != nil {
				id = missing.InstrumentID.String()
			}
			issues = append(issues, HealthIssue{
				ID:           "valuation-instrument-" + id,
				Kind:         kind,
				Severity:     HealthSeverityBlocking,
				GroupKey:     "instrument:" + id,
				TargetKey:    instrumentTargetKeyPtr(missing.InstrumentID),
				Label:        label,
				InstrumentID: id,
				Code:         string(missing.Kind),
				Reason:       "missing_current_input",
				Action:       action,
				Executable:   executable,
			})
		case domain.MissingFXRate:
			pair := fxPairKey(missing.BaseCurrency, missing.QuoteCurrency)
			if _, ok := fxCovered[pair]; ok {
				continue
			}
			fxCovered[pair] = struct{}{}
			issues = append(issues, HealthIssue{
				ID:         "valuation-fx-" + pair,
				Kind:       HealthKindIncompleteValuation,
				Severity:   HealthSeverityBlocking,
				GroupKey:   "fx:" + pair,
				TargetKey:  "fx:" + pair,
				Label:      pair,
				CurrencyA:  missing.BaseCurrency.String(),
				CurrencyB:  missing.QuoteCurrency.String(),
				Code:       string(missing.Kind),
				Reason:     "missing_current_input",
				Action:     HealthActionRepair,
				Executable: true,
			})
		default:
			continue
		}
	}
	return issues, nil
}

func scanSnapshotHealth(state domain.DailySnapshotState, plan HistoryRepairPlan, rootCause bool) []HealthIssue {
	from := ""
	if state.DirtyFrom != nil {
		from = strings.TrimSpace(*state.DirtyFrom)
	}
	to := ""
	if state.DirtyTo != nil {
		to = strings.TrimSpace(*state.DirtyTo)
	}
	if from == "" && (state.LastCompletedClosedOn == nil || strings.TrimSpace(*state.LastCompletedClosedOn) == "") {
		if plan.OriginLocalDate == "" || plan.YesterdayLocal == "" {
			return nil
		}
		issue := HealthIssue{
			ID:         "snapshot-missing",
			Kind:       HealthKindSnapshotMissing,
			Severity:   HealthSeverityBlocking,
			GroupKey:   "snapshot",
			TargetKey:  "snapshot",
			RangeStart: plan.OriginLocalDate,
			RangeEnd:   plan.YesterdayLocal,
			Code:       "snapshot_missing",
			Reason:     "snapshot_missing",
			Action:     HealthActionRepair,
			Executable: !rootCause,
			Collapsed:  rootCause,
		}
		return []HealthIssue{issue}
	}
	if from == "" {
		return nil
	}
	if to == "" {
		to = from
	}
	issue := HealthIssue{
		ID:         "snapshot-outdated",
		Kind:       HealthKindSnapshotOutdated,
		Severity:   HealthSeverityBlocking,
		GroupKey:   "snapshot",
		TargetKey:  "snapshot",
		RangeStart: from,
		RangeEnd:   to,
		Code:       "snapshot_outdated",
		Reason:     "snapshot_outdated",
		Action:     HealthActionRepair,
		Executable: !rootCause,
		Collapsed:  rootCause,
	}
	return []HealthIssue{issue}
}

func scanSyncFailureHealth(job SyncJobSnapshot, instruments map[string]struct{}, fxCovered map[string]struct{}) []HealthIssue {
	if job.Outcome != SyncOutcomeFailed && job.Outcome != SyncOutcomePartial {
		return nil
	}
	var issues []HealthIssue
	seen := map[string]struct{}{}
	for _, blocker := range job.Blockers {
		key := blocker.TargetKey + ":" + blocker.Code
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if strings.HasPrefix(blocker.TargetKey, "instrument:") {
			id := strings.TrimPrefix(blocker.TargetKey, "instrument:")
			if _, ok := instruments[id]; ok {
				continue
			}
		}
		if strings.HasPrefix(blocker.TargetKey, "fx:") {
			pair := strings.TrimPrefix(blocker.TargetKey, "fx:")
			if _, ok := fxCovered[pair]; ok {
				continue
			}
		}
		issues = append(issues, HealthIssue{
			ID:        "sync-" + key,
			Kind:      HealthKindSyncFailure,
			Severity:  HealthSeverityWarning,
			GroupKey:  "sync",
			TargetKey: blocker.TargetKey,
			Code:      blocker.Code,
			Reason:    blocker.Reason,
			Action:    HealthActionNone,
		})
	}
	return issues
}

func finishHealthReport(report MarketDataHealthReport) MarketDataHealthReport {
	sort.SliceStable(report.Issues, func(i, j int) bool {
		if report.Issues[i].Kind != report.Issues[j].Kind {
			return report.Issues[i].Kind < report.Issues[j].Kind
		}
		return report.Issues[i].TargetKey < report.Issues[j].TargetKey
	})
	since := ""
	for _, issue := range report.Issues {
		if issue.Collapsed || issue.Severity == HealthSeverityInfo {
			continue
		}
		report.IssueCount++
		if issue.Executable {
			report.ExecutableCount++
		} else if issue.Action != HealthActionNone && issue.Action != HealthActionRepair {
			report.PrerequisiteCount++
		}
		if issue.RangeStart != "" && (since == "" || issue.RangeStart < since) {
			since = issue.RangeStart
		}
	}
	report.IncompleteSince = since
	report.Healthy = report.IssueCount == 0
	if report.Healthy && report.CoverageThrough == "" {
		report.CoverageThrough = report.LastFinalizedMarketDate
	}
	return report
}

func hasInstrumentGap(need InstrumentRepairNeed) bool {
	return len(need.MissingRanges) > 0 || len(need.FetchRanges) > 0 || need.OpeningAnchorMissing
}

func firstRange(primary, fallback []DateRange) DateRange {
	if len(primary) > 0 {
		start := primary[0].Start
		end := primary[len(primary)-1].End
		return DateRange{Start: start, End: end}
	}
	if len(fallback) > 0 {
		return DateRange{Start: fallback[0].Start, End: fallback[len(fallback)-1].End}
	}
	return DateRange{}
}

func hasUncollapsedRootCause(issues []HealthIssue) bool {
	for _, issue := range issues {
		if issue.Collapsed {
			continue
		}
		switch issue.Kind {
		case HealthKindMissingInstrumentHistory, HealthKindMissingFXHistory, HealthKindMissingManualPrice, HealthKindMissingManualFX, HealthKindMissingProviderKey, HealthKindMissingBinding, HealthKindUnsupportedCoverage, HealthKindIncompleteValuation:
			return true
		}
	}
	return false
}

func countedSnapshotDays(issues []HealthIssue) int {
	for _, issue := range issues {
		if issue.Collapsed || (issue.Kind != HealthKindSnapshotMissing && issue.Kind != HealthKindSnapshotOutdated) {
			continue
		}
		if issue.RangeStart == "" {
			return 0
		}
		to := issue.RangeEnd
		if to == "" {
			to = issue.RangeStart
		}
		dates, err := domain.InclusiveMarketDates(issue.RangeStart, to)
		if err != nil {
			return 0
		}
		return len(dates)
	}
	return 0
}

func hasQuoteOnOrBefore(quotes []domain.InstrumentQuote, originDate string, location *time.Location) bool {
	if location == nil {
		location = time.UTC
	}
	for _, quote := range quotes {
		if quote.QuotedAt.IsZero() {
			continue
		}
		if quote.QuotedAt.In(location).Format("2006-01-02") <= originDate {
			return true
		}
	}
	return false
}

func hasFXQuoteOnOrBefore(quotes []domain.FXQuote, currencyA, currencyB domain.CurrencyCode, originDate string, location *time.Location) bool {
	if location == nil {
		location = time.UTC
	}
	want := fxPairKey(currencyA, currencyB)
	for _, quote := range quotes {
		if fxPairKey(quote.BaseCurrency, quote.QuoteCurrency) != want {
			continue
		}
		if quote.QuotedAt.IsZero() {
			continue
		}
		if quote.QuotedAt.In(location).Format("2006-01-02") <= originDate {
			return true
		}
	}
	return false
}

func missingFXDates(origin, lastFinalized string, coverage domain.FXHistoryCoverage) []string {
	if origin == "" || lastFinalized == "" || origin > lastFinalized {
		return nil
	}
	required, err := domain.InclusiveMarketDates(origin, lastFinalized)
	if err != nil {
		return nil
	}
	covered := indexStrings(coverage.DailyReferenceDates)
	for _, date := range coverage.NoObservationDates {
		covered[date] = struct{}{}
	}
	missing := make([]string, 0)
	for _, date := range required {
		if _, ok := covered[date]; !ok {
			missing = append(missing, date)
		}
	}
	return missing
}

func instrumentHistoryAutoRepairable(providerKey string) bool {
	return strings.ToLower(strings.TrimSpace(providerKey)) == TiingoProviderKey
}

func fxHistoryAutoRepairable(providerKey string) bool {
	return strings.ToLower(strings.TrimSpace(providerKey)) == FrankfurterProviderKey
}

func instrumentTargetKeyPtr(id *domain.InstrumentID) string {
	if id == nil {
		return "instrument"
	}
	return instrumentTargetKey(*id)
}
