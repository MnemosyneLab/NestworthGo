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
	HealthKindInitialAnchorMissing     = "initial_anchor_missing"
	HealthKindUnsupportedCoverage      = "unsupported_coverage"
	HealthKindStaleAccountValue        = "stale_account_value"
	HealthKindMissingAccountValue      = "missing_account_value"
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
	AccountID    string
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

// ScanMarketDataHealth inspects local coverage, routing, Tiingo key settings, valuation
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
		valuationIssues, err := s.scanValuationHealth(ctx, map[string]struct{}{}, map[string]struct{}{})
		if err != nil {
			return MarketDataHealthReport{}, err
		}
		return finishHealthReport(MarketDataHealthReport{Issues: append([]HealthIssue{issue}, valuationIssues...)}), nil
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
		if need.LatestOnly {
			continue
		}
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

	// Failed attempts remain in sync details. Current health is derived from
	// current coverage and valuation, not an old operation's errors.

	report := MarketDataHealthReport{
		CoverageThrough:         plan.LastFinalizedMarketDate,
		LastFinalizedMarketDate: plan.LastFinalizedMarketDate,
		SnapshotDays:            estimateDirtyDays(state, plan),
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
	if need.OpeningAnchorExhausted && len(need.MissingRanges) == 0 && len(need.FetchRanges) == 0 {
		base.Kind = HealthKindInitialAnchorMissing
		base.Severity = HealthSeverityBlocking
		base.Code = HealthKindInitialAnchorMissing
		base.Reason = HealthKindInitialAnchorMissing
		base.Action = HealthActionManualEntry
		return base, false
	}
	if len(need.UnavailableRanges) > 0 && len(need.FetchRanges) == 0 {
		base.Kind = HealthKindUnsupportedCoverage
		base.Severity = HealthSeverityWarning
		base.Code = "coingecko_history_limit_365_days"
		base.Reason = base.Code
		base.Action = HealthActionManualEntry
		base.RangeStart = string(need.UnavailableRanges[0].Start)
		base.RangeEnd = string(need.UnavailableRanges[len(need.UnavailableRanges)-1].End)
		return base, false
	}
	if !hasInstrumentGap(need) {
		return HealthIssue{}, false
	}
	span := firstRange(need.MissingRanges, need.FetchRanges)
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
	starts, err := s.instrumentHistoryStarts(ctx, *origin, s.clock())
	if err != nil {
		return nil, err
	}
	var issues []HealthIssue
	for _, instrument := range instruments {
		if instrument.QuoteSource != domain.QuoteSourceManual {
			continue
		}
		originDate := starts[instrument.ID]
		if originDate == "" {
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
			RangeEnd:     max(originDate, plan.LastFinalizedMarketDate),
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
	providerKey := strings.ToLower(strings.TrimSpace(s.FXProviderKey()))
	for _, item := range coverage {
		if !strings.EqualFold(strings.TrimSpace(item.ProviderKey), providerKey) || item.SourcePolicyVersion != fxSourcePolicy(providerKey) {
			continue
		}
		coverageByPair[fxPairKey(item.BaseCurrency, item.QuoteCurrency)] = item
	}
	location := time.UTC
	if origin != nil {
		if loaded, locErr := time.LoadLocation(origin.Timezone); locErr == nil {
			location = loaded
		}
	}
	var issues []HealthIssue
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
		anchorCloses := make([]domain.OracleClose, 0, len(item.DailyReferenceDates))
		for _, date := range item.DailyReferenceDates {
			anchorCloses = append(anchorCloses, domain.OracleClose{MarketDate: date})
		}
		_, anchorMissing := domain.FindOpeningAnchor(plan.OriginLocalDate, anchorCloses)
		_, anchorExhausted, anchorErr := nextOpeningAnchorWindowForDates(item.DailyReferenceDates, item.NoObservationDates, plan.OriginLocalDate)
		if anchorErr != nil {
			return nil, covered, anchorErr
		}
		if len(missing) == 0 && !anchorMissing {
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
		if len(missing) == 0 && anchorMissing && anchorExhausted {
			issue.Kind = HealthKindInitialAnchorMissing
			issue.Severity = HealthSeverityBlocking
			issue.Code = HealthKindInitialAnchorMissing
			issue.Reason = HealthKindInitialAnchorMissing
			issue.Action = HealthActionManualEntry
			issues = append(issues, issue)
			continue
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
			label := domain.InstrumentDisplayLabel(missing.InstrumentName, missing.InstrumentSymbol)
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
		case domain.MissingAccountValue:
			issues = append(issues, HealthIssue{
				ID: "account-value-" + missing.AccountID.String(), Kind: HealthKindMissingAccountValue,
				Severity: HealthSeverityBlocking, GroupKey: "account:" + missing.AccountID.String(),
				TargetKey: "account:" + missing.AccountID.String(), AccountID: missing.AccountID.String(),
				Label: missing.AccountName, Code: string(missing.Kind), Reason: "missing_current_input",
				Action: "account_value", Executable: false,
			})
		default:
			continue
		}
	}
	records, err := s.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if string(record.Account.TrackingMode) != "manual_value" || !record.Account.IncludeInNetWorth || record.LatestValue == nil {
			continue
		}
		// A maintenance reminder, not a valuation expiry or a missing financial input.
		if record.LatestValue.EffectiveAt.AddDate(0, 0, 180).Before(s.clock()) {
			issues = append(issues, HealthIssue{
				ID: "stale-account-value-" + record.Account.ID.String(), Kind: HealthKindStaleAccountValue,
				Severity: HealthSeverityWarning, GroupKey: "account:" + record.Account.ID.String(),
				TargetKey: "account:" + record.Account.ID.String(), AccountID: record.Account.ID.String(),
				Label: record.Account.Name, Reason: "manual_value_older_than_180_days", Action: "account_value",
			})
		}
	}
	return issues, nil
}

func scanSnapshotHealth(state domain.DailySnapshotState, plan HistoryRepairPlan, rootCause bool) []HealthIssue {
	from, to, ok := closedSnapshotRange(state, plan)
	if !ok {
		return nil
	}
	kind, code := HealthKindSnapshotOutdated, "snapshot_outdated"
	if state.DirtyFrom == nil || strings.TrimSpace(*state.DirtyFrom) == "" {
		kind, code = HealthKindSnapshotMissing, "snapshot_missing"
	}
	issue := HealthIssue{
		ID:         strings.ReplaceAll(code, "_", "-"),
		Kind:       kind,
		Severity:   HealthSeverityBlocking,
		GroupKey:   "snapshot",
		TargetKey:  "snapshot",
		RangeStart: from,
		RangeEnd:   to,
		Code:       code,
		Reason:     code,
		Action:     HealthActionRepair,
		Executable: !rootCause,
		Collapsed:  rootCause,
	}
	return []HealthIssue{issue}
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
	// FetchRanges also contains routine correction and expired no-observation
	// checks. Only absent coverage or an absent anchor is a health problem.
	return len(need.MissingRanges) > 0 || need.OpeningAnchorMissing
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
		case HealthKindMissingAccountValue, HealthKindMissingInstrumentHistory, HealthKindMissingFXHistory, HealthKindMissingManualPrice, HealthKindMissingManualFX, HealthKindMissingProviderKey, HealthKindMissingBinding, HealthKindInitialAnchorMissing, HealthKindUnsupportedCoverage, HealthKindIncompleteValuation:
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

func fxHistoryAutoRepairable(providerKey string) bool {
	return strings.ToLower(strings.TrimSpace(providerKey)) == FrankfurterProviderKey
}

func instrumentTargetKeyPtr(id *domain.InstrumentID) string {
	if id == nil {
		return "instrument"
	}
	return instrumentTargetKey(*id)
}
