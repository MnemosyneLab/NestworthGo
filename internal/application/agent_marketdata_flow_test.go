package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAgentLatestSuppressesRoutineProviderRefresh(t *testing.T) {
	t.Parallel()
	tiingo := &countingProvider{inner: &syncFakeProvider{key: TiingoProviderKey}}
	service, _, instrument, _ := newSyncFixture(t, tiingo, &syncFakeProvider{key: YahooFinanceProviderKey})
	ctx := context.Background()
	_, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		InstrumentID: instrument.ID.String(), Currency: "USD", Value: "190", Kind: "latest",
		QuotedAt: "2026-09-09T15:30:00Z", SourceTitle: "Exchange screen",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RefreshMissingOrStale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var cached bool
	for _, item := range result.Items {
		if item.TargetKey == instrumentTargetKey(instrument.ID) {
			cached = item.Status == RefreshCached
		}
	}
	tiingo.mu.Lock()
	latestCalls := tiingo.latestInstrument
	tiingo.mu.Unlock()
	if !cached || latestCalls != 0 {
		t.Fatalf("fresh Agent observation did not suppress routine provider refresh: result=%+v calls=%d", result, latestCalls)
	}
	if _, err := service.RefreshInstrument(ctx, instrument.ID); err != nil {
		t.Fatal(err)
	}
	tiingo.mu.Lock()
	latestCalls = tiingo.latestInstrument
	tiingo.mu.Unlock()
	if latestCalls != 1 {
		t.Fatalf("explicit provider refresh calls=%d, want 1", latestCalls)
	}
}

func TestAgentDailyDatesSkipProviderSyncAndWithdrawalsReopenDates(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requested []DateRange
	tiingo := &syncFakeProvider{key: TiingoProviderKey}
	tiingo.history = func(_ context.Context, _ InstrumentMarketIdentity, span DateRange, _ int) (MappingOutcome[InstrumentDailyObservation], error) {
		mu.Lock()
		requested = append(requested, span)
		mu.Unlock()
		return MappingOutcome[InstrumentDailyObservation]{Status: MappingUnsupported, Reason: "test_no_provider_history"}, nil
	}
	service, _, instrument, _ := newSyncFixture(t, tiingo, &syncFakeProvider{key: YahooFinanceProviderKey})
	ctx := context.Background()
	baseline, err := service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var weekdays []string
	for _, need := range baseline.Instruments {
		if need.InstrumentID != instrument.ID {
			continue
		}
		for _, span := range need.FetchRanges {
			dates, dateErr := InclusiveMarketDates(span)
			if dateErr != nil {
				t.Fatal(dateErr)
			}
			for _, date := range dates {
				parsed, _ := time.Parse("2006-01-02", string(date))
				if parsed.Weekday() != time.Saturday && parsed.Weekday() != time.Sunday {
					weekdays = append(weekdays, string(date))
				}
			}
		}
	}
	if len(weekdays) < 3 {
		t.Fatalf("fixture needs three provider dates, got %v", weekdays)
	}
	d1, d2, d3 := weekdays[len(weekdays)-3], weekdays[len(weekdays)-2], weekdays[len(weekdays)-1]
	closeItem := func(date string) AgentMarketDataItem {
		return AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "USD", Value: "190", Kind: "close", Date: date, Timezone: "America/New_York", SourceTitle: "Exchange close"}
	}
	created, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{closeItem(d1), closeItem(d2)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.QuoteIDs) != 2 {
		t.Fatalf("quote receipt = %+v", created)
	}
	plan, err := service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, instrument.ID, map[string]bool{d1: false, d2: false, d3: true})
	forced, err := service.PlanHistorySync(ctx, HistorySyncOptions{ForceRecheck: true})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, forced, instrument.ID, map[string]bool{d1: true, d2: true})
	preview, err := service.PreviewMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range preview.Items {
		if item.Kind == "history_instrument" && (marketDateInSpan(d1, DateRange{Start: MarketDate(item.StartDate), End: MarketDate(item.EndDate)}) || marketDateInSpan(d2, DateRange{Start: MarketDate(item.StartDate), End: MarketDate(item.EndDate)})) {
			t.Fatalf("sync preview still requests Agent exact date: %+v", item)
		}
	}
	if _, err := service.StartMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String()}); err != nil {
		t.Fatal(err)
	}
	waitSyncTerminal(t, service)
	mu.Lock()
	actual := append([]DateRange(nil), requested...)
	mu.Unlock()
	for _, span := range actual {
		if marketDateInSpan(d1, span) || marketDateInSpan(d2, span) {
			t.Fatalf("provider was called for Agent exact dates: %+v", actual)
		}
	}
	correction := closeItem(d3)
	correction.Operation, correction.TargetQuoteID = "correct", created.QuoteIDs[0]
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
		t.Fatal(err)
	}
	plan, err = service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, instrument.ID, map[string]bool{d1: true, d2: false, d3: false})
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{Operation: "retract", TargetQuoteID: created.QuoteIDs[1], SourceTitle: "Exchange withdrew close"}}}); err != nil {
		t.Fatal(err)
	}
	plan, err = service.PlanHistorySync(ctx, HistorySyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertInstrumentFetchDates(t, plan, instrument.ID, map[string]bool{d1: true, d2: true, d3: false})
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "agent"); err != nil {
		t.Fatal(err)
	}
	preview, err = service.PreviewMarketDataSync(ctx, SyncRequest{Scope: SyncScopeInstrument, InstrumentID: instrument.ID.String(), ForceRecheck: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.InstrumentTargets != 0 || preview.LatestInstrumentCount != 0 || preview.EstimatedRequestCount != 0 {
		t.Fatalf("Agent source still plans provider calls under Force Recheck: %+v", preview)
	}
	refresh, err := service.RefreshInstrument(ctx, instrument.ID)
	if err != nil || len(refresh.Items) != 1 || refresh.Items[0].Status != RefreshSkipped {
		t.Fatalf("Agent source direct refresh = %+v, %v", refresh, err)
	}
}

func TestCompleteAgentFXWithNoProviderKeyHasNoProviderBlocker(t *testing.T) {
	t.Parallel()
	service, _, _, setClock := newOnboardedService(t, "agent-fx-no-provider", []string{"Owner"})
	setClock(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil || bootstrap.Household == nil {
		t.Fatalf("household: %+v, %v", bootstrap, err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "SGD", "provider"); err != nil {
		t.Fatal(err)
	}
	items := []AgentMarketDataItem{}
	for _, date := range []string{"2026-09-27", "2026-09-28"} {
		items = append(items, AgentMarketDataItem{BaseCurrency: "USD", QuoteCurrency: "SGD", Value: "1.4", Kind: "daily_reference", Date: date, Timezone: "UTC", SourceTitle: "Central bank reference"})
	}
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
		t.Fatal(err)
	}
	plan := HistoryRepairPlan{HouseholdID: bootstrap.Household.ID, OriginLocalDate: "2026-09-27", LastFinalizedMarketDate: "2026-09-28"}
	tasks, blockers := service.planFXHistoryRanges(ctx, bootstrap.Household.ID, plan, SyncRequest{Scope: SyncScopeFX, CurrencyA: "USD", CurrencyB: "SGD"})
	if len(tasks) != 0 || len(blockers) != 0 {
		t.Fatalf("complete Agent FX dates still require missing provider: tasks=%+v blockers=%+v", tasks, blockers)
	}
}

func TestAgentFXSuppliesMetalConversionWithoutFXProviderCall(t *testing.T) {
	t.Parallel()
	fxProvider := &countingProvider{inner: &syncFakeProvider{key: FrankfurterProviderKey}}
	metalProvider := &syncFakeProvider{key: YahooFinanceProviderKey}
	service, _, instrument, _ := newSyncFixture(t, metalProvider, fxProvider)
	ctx := context.Background()
	latest, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7.2", Kind: "latest",
		QuotedAt: "2026-09-09T15:30:00Z", SourceTitle: "Bank FX screen",
	}}})
	if err != nil || latest.Inserted != 1 {
		t.Fatalf("Agent FX import: %+v, %v", latest, err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "provider"); err != nil {
		t.Fatal(err)
	}
	metal := domain.Instrument{HouseholdID: instrument.HouseholdID, Type: domain.InstrumentPreciousMetal, QuoteCurrency: "CNY", MetalTemplate: "gold", QuantityUnit: "g"}
	quote, evidence, err := service.latestMetalQuote(withMetalFetchCache(ctx), metal, metalProvider, true)
	if err != nil || quote.Currency != "CNY" || evidence == "" {
		t.Fatalf("latest metal conversion: %+v evidence=%q err=%v", quote, evidence, err)
	}
	fxProvider.mu.Lock()
	latestCalls := fxProvider.latestFX
	fxProvider.mu.Unlock()
	if latestCalls != 0 {
		t.Fatalf("metal conversion fetched FX despite Agent quote: %d", latestCalls)
	}
	if err := service.SetFXProvider(FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return time.Date(2026, 9, 10, 16, 5, 0, 0, time.UTC) })
	if _, _, err := service.latestMetalQuote(withMetalFetchCache(ctx), metal, metalProvider, true); err != nil {
		t.Fatalf("stale Agent FX did not fall back to configured provider: %v", err)
	}
	fxProvider.mu.Lock()
	latestCalls = fxProvider.latestFX
	fxProvider.mu.Unlock()
	if latestCalls != 1 {
		t.Fatalf("stale Agent FX provider calls=%d, want 1", latestCalls)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		BaseCurrency: "USD", QuoteCurrency: "CNY", Value: "7.1", Kind: "daily_reference",
		Date: "2026-09-08", Timezone: "UTC", SourceTitle: "Bank daily FX",
	}}}); err != nil {
		t.Fatal(err)
	}
	outcome := MappingOutcome[InstrumentDailyObservation]{Status: MappingMapped, Batch: HistoryBatch[InstrumentDailyObservation]{Observations: []InstrumentDailyObservation{{
		MarketDate: "2026-09-08", Value: "100", Currency: "USD", ValueEffectiveAt: time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC),
	}}}}
	converted, err := service.convertMetalHistory(ctx, nil, metal, DateRange{Start: "2026-09-08", End: "2026-09-08"}, outcome, nil)
	if err != nil || len(converted.Batch.Observations) != 1 || converted.Batch.Observations[0].Currency != "CNY" {
		t.Fatalf("historical metal conversion: %+v, %v", converted, err)
	}
	fxProvider.mu.Lock()
	historyCalls := fxProvider.historyFX
	fxProvider.mu.Unlock()
	if historyCalls != 0 {
		t.Fatalf("historical conversion fetched FX despite exact Agent date: %d", historyCalls)
	}
	missing := outcome
	missing.Batch.Observations = []InstrumentDailyObservation{{MarketDate: "2026-09-07", Value: "100", Currency: "USD", ValueEffectiveAt: time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC)}}
	if _, err := service.convertMetalHistory(ctx, nil, metal, DateRange{Start: "2026-09-07", End: "2026-09-07"}, missing, nil); !hasDomainCode(err, domain.ErrUnavailable) {
		t.Fatalf("Agent-only missing FX should report a local gap: %v", err)
	}
	fxProvider.mu.Lock()
	historyCalls = fxProvider.historyFX
	fxProvider.mu.Unlock()
	if historyCalls != 0 {
		t.Fatalf("Agent-only missing FX used provider history: %d", historyCalls)
	}
}

func assertInstrumentFetchDates(t *testing.T, plan HistoryRepairPlan, id domain.InstrumentID, expected map[string]bool) {
	t.Helper()
	for _, need := range plan.Instruments {
		if need.InstrumentID != id {
			continue
		}
		for date, want := range expected {
			found := false
			for _, span := range need.FetchRanges {
				found = found || marketDateInSpan(date, span)
			}
			if found != want {
				t.Fatalf("date %s planned=%v want=%v; need=%+v", date, found, want, need)
			}
		}
		return
	}
	t.Fatalf("instrument %s absent from plan: %+v", id, plan.Instruments)
}

func marketDateInSpan(date string, span DateRange) bool {
	return string(span.Start) <= date && date <= string(span.End)
}
