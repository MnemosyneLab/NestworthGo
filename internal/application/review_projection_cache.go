package application

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

type cachedReviewPosition struct {
	timeline ReviewPositionTimeline
	epochIDs []string
}

// reviewProjectionCache shares expensive historical snapshots, daily input
// projections, and position timelines across every rule in one publication.
// The cache never outlives the immutable capture that supplied its facts.
type reviewProjectionCache struct {
	capture            domain.ReviewProjectionCapture
	reviewHistory      *domain.HistoricalSnapshotBatch
	providerKey        string
	snapshots          map[string]domain.PortfolioSnapshot
	inputs             map[string]cachedReviewInput
	positions          map[domain.InstrumentID]cachedReviewPosition
	positionActivities map[domain.InstrumentID][]domain.Activity
	historicalQuotes   *historicalQuoteCache
	quoteSeries        map[domain.InstrumentID]domain.ReviewDailyQuoteSeries
	dailyEvidence      map[reviewDailyEvidenceScopeKey]reviewDailyEvidenceScope
	suffixEvidence     map[domain.ReviewDailyEvidenceKey]int
	snapshotReads      int
	inputProjections   int
	positionReplays    int
}

type cachedReviewInput struct {
	input    domain.ReviewDailyInput
	coverage ReviewHistoricalCoverage
}

type reviewDailyEvidenceScopeKey struct {
	ruleID           domain.ReviewRuleID
	conditionVersion int
	instrumentID     domain.InstrumentID
	timezoneVersion  string
}

type reviewDailyEvidenceScope struct {
	latestOn          string
	count             int
	complete          bool
	trailingTrueCount int
}

func newReviewProjectionCache(capture domain.ReviewProjectionCapture, providerKey string) *reviewProjectionCache {
	history := capture.Historical.History
	// The review-specific repository capture selects position-changing
	// activities and loads cost events separately. Replaying those selected
	// position effects is sufficient for daily quantities; cash-only activity
	// effects cannot change the review input. Mixed activities remain available
	// in full whenever they carry a holding quantity effect.
	reviewHistory := history
	reviewHistory.Activities = reviewSnapshotActivities(history.Activities)
	quotes := &historicalQuoteCache{instrumentQuotes: make(map[domain.InstrumentID][]domain.InstrumentQuote), fxQuotes: history.FXQuoteFacts}
	for _, quote := range history.InstrumentQuoteFacts {
		quotes.instrumentQuotes[quote.InstrumentID] = append(quotes.instrumentQuotes[quote.InstrumentID], quote)
	}
	quoteSeries := make(map[domain.InstrumentID]domain.ReviewDailyQuoteSeries, len(quotes.instrumentQuotes))
	for instrumentID, instrumentQuotes := range quotes.instrumentQuotes {
		quoteSeries[instrumentID] = domain.NewReviewDailyQuoteSeries(instrumentID, instrumentQuotes, history.Portfolio.InstrumentClosures...)
	}
	cache := &reviewProjectionCache{
		capture: capture, reviewHistory: &reviewHistory, providerKey: providerKey,
		snapshots:          make(map[string]domain.PortfolioSnapshot),
		inputs:             make(map[string]cachedReviewInput),
		positions:          make(map[domain.InstrumentID]cachedReviewPosition),
		positionActivities: make(map[domain.InstrumentID][]domain.Activity),
		historicalQuotes:   quotes,
		quoteSeries:        quoteSeries,
		dailyEvidence:      make(map[reviewDailyEvidenceScopeKey]reviewDailyEvidenceScope),
	}
	for key, item := range capture.ReusableDailyEvidence {
		scopeKey := reviewDailyEvidenceScopeKey{ruleID: key.RuleID, conditionVersion: key.ConditionVersion, instrumentID: key.InstrumentID, timezoneVersion: key.TimezoneVersion}
		scope, ok := cache.dailyEvidence[scopeKey]
		if !ok {
			scope.complete = true
		}
		scope.count++
		if key.On >= scope.latestOn {
			scope.latestOn = key.On
			scope.trailingTrueCount = item.PriorConsecutiveCount
		}
		if !item.HasResult || item.Result.On != key.On || item.Input.On != key.On || item.Input.InstrumentID != key.InstrumentID || item.InputFingerprint == "" || item.Input.FXProviderKey != providerKey {
			scope.complete = false
		}
		cache.dailyEvidence[scopeKey] = scope
	}
	if capture.Suffix != nil {
		cache.suffixEvidence = make(map[domain.ReviewDailyEvidenceKey]int, len(capture.Suffix.Prefix))
		for index := range capture.Suffix.Prefix {
			cache.suffixEvidence[capture.Suffix.Prefix[index].Key] = index
		}
	}
	cache.indexPositionActivities()
	return cache
}

func reviewSnapshotActivities(activities []domain.Activity) []domain.Activity {
	result := make([]domain.Activity, 0, len(activities))
	for _, activity := range activities {
		for _, effect := range activity.Effects {
			if effect.Quantity != nil && effect.HoldingID != nil {
				result = append(result, activity)
				break
			}
		}
	}
	return result
}

// reusableLatestOn reports the final contiguous date already cached for one
// rule/holding. The caller checks the bounded trailing interval itself.
func (c *reviewProjectionCache) reusableLatestOn(rule domain.ReviewRule, instrumentID domain.InstrumentID, timezoneVersion, historyStartOn, latestOn string, width int) (string, bool) {
	if c.capture.Control.ProjectionStatus != "complete" || c.capture.Control.PublishedFingerprint != c.capture.HistoricalFingerprint {
		return "", false
	}
	// Older episode projections kept only the latest rolling window. Rebuild
	// that history once so the episode's original trigger window and first hit
	// can be reconstructed before allowing bounded incremental replay again.
	for _, episode := range c.capture.OpenEpisodes {
		if episode.RuleID == rule.ID && episode.InstrumentID == instrumentID && episode.TriggerWindow.WindowStart == "" {
			return "", false
		}
	}
	key := reviewDailyEvidenceScopeKey{ruleID: rule.ID, conditionVersion: rule.CurrentConditionVersion, instrumentID: instrumentID, timezoneVersion: timezoneVersion}
	scope, ok := c.dailyEvidence[key]
	if !ok || !scope.complete || scope.count == 0 || width < 1 || scope.latestOn > latestOn {
		return "", false
	}
	latest, err := time.Parse(time.DateOnly, scope.latestOn)
	if err != nil {
		return "", false
	}
	start, err := time.Parse(time.DateOnly, historyStartOn)
	if err != nil {
		return "", false
	}
	first := latest.AddDate(0, 0, 1-width)
	if first.Before(start) {
		first = start
	}
	expected := int(latest.Sub(first).Hours()/24) + 1
	if expected != scope.count {
		return "", false
	}
	for day := first; !day.After(latest); day = day.AddDate(0, 0, 1) {
		key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: timezoneVersion, On: day.Format(time.DateOnly)}
		if _, ok := c.capture.ReusableDailyEvidence[key]; !ok {
			return "", false
		}
	}
	return scope.latestOn, true
}

func (c *reviewProjectionCache) reusableConsecutiveCount(rule domain.ReviewRule, instrumentID domain.InstrumentID, timezoneVersion, latestOn string) int {
	key := reviewDailyEvidenceScopeKey{ruleID: rule.ID, conditionVersion: rule.CurrentConditionVersion, instrumentID: instrumentID, timezoneVersion: timezoneVersion}
	scope, ok := c.dailyEvidence[key]
	if !ok || scope.latestOn != latestOn || !scope.complete {
		return 0
	}
	return scope.trailingTrueCount
}

// Position replay only needs quantity events that touch the instrument. Index
// them once per immutable capture so scanning one instrument does not sort all
// household activities for every target.
func (c *reviewProjectionCache) indexPositionActivities() {
	holdingInstruments := make(map[domain.HoldingID]domain.InstrumentID, len(c.capture.Historical.History.Portfolio.Holdings))
	for _, holding := range c.capture.Historical.History.Portfolio.Holdings {
		holdingInstruments[holding.ID] = holding.InstrumentID
	}
	for _, component := range c.capture.Historical.History.OriginData.Components {
		if component.Kind == domain.HistoryOriginHoldingQuantity && component.HoldingID != nil && component.InstrumentID != nil {
			holdingInstruments[*component.HoldingID] = *component.InstrumentID
		}
	}
	for _, activity := range c.capture.Historical.History.Activities {
		var touched map[domain.InstrumentID]struct{}
		for _, effect := range activity.Effects {
			if effect.Quantity == nil || effect.HoldingID == nil {
				continue
			}
			if owner := holdingInstruments[*effect.HoldingID]; owner != "" {
				if touched == nil {
					touched = make(map[domain.InstrumentID]struct{}, 2)
				}
				touched[owner] = struct{}{}
			}
			if effect.InstrumentID != nil {
				if touched == nil {
					touched = make(map[domain.InstrumentID]struct{}, 2)
				}
				touched[*effect.InstrumentID] = struct{}{}
			}
		}
		for instrumentID := range touched {
			c.positionActivities[instrumentID] = append(c.positionActivities[instrumentID], activity)
		}
	}
}

func (c *reviewProjectionCache) dailyInput(ctx context.Context, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string) (domain.ReviewDailyInput, ReviewHistoricalCoverage, error) {
	cutoff, err := domain.ReviewCivilCutoff(on, spec.Schedule.Timezone)
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	history := c.capture.Historical
	if history.History.Origin.StartedAt.IsZero() || !history.History.Origin.StartedAt.Before(cutoff) {
		return domain.ReviewDailyInput{On: on, InstrumentID: instrumentID, FXProviderKey: c.providerKey}, ReviewHistoricalCoverage{On: on, Missing: []string{"history_origin"}}, nil
	}
	needsCost, lookbacks, currencies := reviewRequiredInputs(spec.Predicate)
	policy, err := jsoncodec.Marshal(struct {
		Timezone   string
		NeedsCost  bool
		Lookbacks  []int
		Currencies []domain.CurrencyCode
	}{spec.Schedule.Timezone, needsCost, lookbacks, currencies})
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	inputKey := string(instrumentID) + "\x00" + on + "\x00" + string(policy)
	if cached, ok := c.inputs[inputKey]; ok {
		return cached.input, cached.coverage, nil
	}
	portfolio, err := c.portfolioForDay(ctx, spec.Schedule.Timezone, on, cutoff)
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	// Daily evidence for one target only consults that instrument's quotes.
	// Keep the immutable indexed quote arrays shared across dates instead of
	// copying every quote into each cached day snapshot.
	portfolio.InstrumentQuotes = c.historicalQuotes.instrumentQuotes[instrumentID]
	portfolio.FXQuotes = c.historicalQuotes.fxQuotes
	input, coverage, err := projectReviewHistoricalDayFromPortfolioWithQuoteSeries(ctx, history, c.providerKey, spec, instrumentID, on, cutoff, portfolio, c.quoteSeries[instrumentID])
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, err
	}
	c.inputs[inputKey] = cachedReviewInput{input: input, coverage: coverage}
	c.inputProjections++
	return input, coverage, nil
}

func (c *reviewProjectionCache) portfolioForDay(ctx context.Context, timezone, on string, cutoff time.Time) (domain.PortfolioSnapshot, error) {
	key := timezone + "\x00" + on
	if snapshot, ok := c.snapshots[key]; ok {
		return snapshot, nil
	}
	if err := ctx.Err(); err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	batch := c.reviewHistory
	snapshot, err := (HistoricalReplay{batch: batch, quotes: c.historicalQuotes, omitQuoteProjection: true}).Snapshot(ctx, &batch.Origin, cutoff.Add(-time.Nanosecond))
	if err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	c.snapshots[key] = snapshot
	c.snapshotReads++
	return snapshot, nil
}

func (c *reviewProjectionCache) position(ctx context.Context, instrumentID domain.InstrumentID, cutoff time.Time) (ReviewPositionTimeline, []string, error) {
	if cached, ok := c.positions[instrumentID]; ok {
		return cached.timeline, cached.epochIDs, nil
	}
	history := c.capture.Historical.History
	history.Activities = c.positionActivities[instrumentID]
	timeline, err := ProjectReviewPositionTimeline(ctx, history, instrumentID, cutoff)
	if err != nil {
		return ReviewPositionTimeline{}, nil, err
	}
	originID := c.capture.Historical.History.Origin.ID
	epochIDs := make([]string, 0, len(timeline.Epochs))
	for _, epoch := range timeline.Epochs {
		id, err := ReviewPositionEpochID(c.capture.HouseholdID, instrumentID, originID, epoch)
		if err != nil {
			return ReviewPositionTimeline{}, nil, err
		}
		epochIDs = append(epochIDs, id)
	}
	c.positions[instrumentID] = cachedReviewPosition{timeline: timeline, epochIDs: epochIDs}
	c.positionReplays++
	return timeline, epochIDs, nil
}

func evaluateReviewHistoricalDateCached(ctx context.Context, cache *reviewProjectionCache, rule domain.ReviewRule, spec domain.ReviewRuleSpec, instrumentID domain.InstrumentID, on string) (domain.ReviewDailyInput, ReviewHistoricalCoverage, domain.ReviewDailyResult, error) {
	if _, err := domain.ReviewCivilCutoff(on, spec.Schedule.Timezone); err != nil {
		if !domain.ReviewNonexistentCivilDate(err) {
			return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, domain.ReviewDailyResult{}, err
		}
		return domain.ReviewDailyInput{On: on, InstrumentID: instrumentID}, ReviewHistoricalCoverage{On: on, Missing: []string{"nonexistent_local_date"}}, domain.ReviewDailyResult{On: on, Truth: domain.ReviewUnknown, Leaves: []domain.ReviewDailyLeafEvidence{}}, nil
	}
	timezoneVersion := spec.Schedule.Timezone + "/" + spec.Schedule.CutoffVersion
	key := domain.ReviewDailyEvidenceKey{RuleID: rule.ID, ConditionVersion: rule.CurrentConditionVersion, InstrumentID: instrumentID, TimezoneVersion: timezoneVersion, On: on}
	if cached, ok := cache.capture.ReusableDailyEvidence[key]; ok && cached.Input.FXProviderKey == cache.providerKey {
		coverage := ReviewHistoricalCoverage{On: on, Missing: append([]string{}, cached.CoverageMissing...)}
		if cached.HasResult {
			return cached.Input, coverage, cached.Result, nil
		}
		result, err := domain.EvaluateReviewDaily(spec, cached.Input)
		return cached.Input, coverage, result, err
	}
	input, coverage, err := cache.dailyInput(ctx, spec, instrumentID, on)
	if err != nil {
		return domain.ReviewDailyInput{}, ReviewHistoricalCoverage{}, domain.ReviewDailyResult{}, err
	}
	result, err := domain.EvaluateReviewDaily(spec, input)
	return input, coverage, result, err
}
