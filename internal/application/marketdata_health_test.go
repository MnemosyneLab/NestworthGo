package application

import (
	"context"
	"sync"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type countingProvider struct {
	inner MarketDataProvider

	mu                sync.Mutex
	latestInstrument  int
	latestFX          int
	historyInstrument int
	historyFX         int
}

func (p *countingProvider) Key() string { return p.inner.Key() }
func (p *countingProvider) Capabilities() MarketDataCapabilities {
	return p.inner.Capabilities()
}
func (p *countingProvider) LatestInstrument(ctx context.Context, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	p.mu.Lock()
	p.latestInstrument++
	p.mu.Unlock()
	return p.inner.LatestInstrument(ctx, identity)
}
func (p *countingProvider) LatestFX(ctx context.Context, identity FXMarketIdentity) (LatestFXQuote, error) {
	p.mu.Lock()
	p.latestFX++
	p.mu.Unlock()
	return p.inner.LatestFX(ctx, identity)
}
func (p *countingProvider) InstrumentDailyHistory(ctx context.Context, identity InstrumentMarketIdentity, rng DateRange) (MappingOutcome[InstrumentDailyObservation], error) {
	p.mu.Lock()
	p.historyInstrument++
	p.mu.Unlock()
	history, ok := p.inner.(InstrumentHistoryProvider)
	if !ok {
		return MappingOutcome[InstrumentDailyObservation]{Status: MappingUnsupported, Reason: "unsupported_price_basis"}, nil
	}
	return history.InstrumentDailyHistory(ctx, identity, rng)
}
func (p *countingProvider) FXDailyHistory(ctx context.Context, identity FXMarketIdentity, rng DateRange) (MappingOutcome[FXDailyObservation], error) {
	p.mu.Lock()
	p.historyFX++
	p.mu.Unlock()
	history, ok := p.inner.(FXHistoryProvider)
	if !ok {
		return MappingOutcome[FXDailyObservation]{Status: MappingUnsupported, Reason: "unsupported"}, nil
	}
	return history.FXDailyHistory(ctx, identity, rng)
}
func (p *countingProvider) networkCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latestInstrument + p.latestFX + p.historyInstrument + p.historyFX
}

type statusProvider struct {
	*countingProvider
	code   string
	reason string
}

func (p *statusProvider) LocalConfigStatus(context.Context) (string, string) {
	return p.code, p.reason
}

func TestScanMarketDataHealthPerformsNoProviderNetwork(t *testing.T) {
	tiingoInner := &syncFakeProvider{key: TiingoProviderKey}
	yahooInner := &syncFakeProvider{key: YahooFinanceProviderKey}
	tiingo := &statusProvider{countingProvider: &countingProvider{inner: tiingoInner}, code: ProviderConfigOK, reason: ""}
	yahoo := &countingProvider{inner: yahooInner}
	service, _, first, second := newSyncFixture(t, tiingo, yahoo)
	report, err := service.ScanMarketDataHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tiingo.networkCalls() != 0 || yahoo.networkCalls() != 0 {
		t.Fatalf("scan contacted providers tiingo=%d yahoo=%d", tiingo.networkCalls(), yahoo.networkCalls())
	}
	if report.LastFinalizedMarketDate == "" {
		t.Fatalf("report = %+v", report)
	}
	if kindCount(report, HealthKindMissingInstrumentHistory) < 1 {
		t.Fatalf("expected executable Tiingo history gap, issues=%+v", report.Issues)
	}
	if !hasIssueForInstrument(report, first.ID.String(), HealthKindMissingInstrumentHistory, true) {
		t.Fatalf("AAPL should be an executable history gap: %+v", report.Issues)
	}
	if !hasIssueForInstrument(report, second.ID.String(), HealthKindUnsupportedCoverage, false) {
		t.Fatalf("Yahoo history must be reported as unsupported, not auto-repairable: %+v", report.Issues)
	}
}

func TestScanMarketDataHealthSeparatesPrerequisitesFromExecutableRepairs(t *testing.T) {
	tiingoInner := &syncFakeProvider{key: TiingoProviderKey}
	yahooInner := &syncFakeProvider{key: YahooFinanceProviderKey}
	tiingo := &statusProvider{countingProvider: &countingProvider{inner: tiingoInner}, code: ProviderConfigMissingKey, reason: "missing"}
	yahoo := &countingProvider{inner: yahooInner}
	service, _, first, _ := newSyncFixture(t, tiingo, yahoo)
	ctx := context.Background()
	accounts, err := service.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil || len(accounts) == 0 {
		t.Fatalf("accounts = %d err=%v", len(accounts), err)
	}
	manual, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ABC Bank Wealth", Type: "bank_investment_product", QuoteCurrency: "SGD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: accounts[0].Account.ID.String(), InstrumentID: manual.ID.String(), Quantity: "1", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}

	report, err := service.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tiingo.networkCalls() != 0 {
		t.Fatal("missing-key scan contacted Tiingo")
	}
	if report.PrerequisiteCount < 1 {
		t.Fatalf("expected prerequisites, report=%+v", report)
	}
	if !hasKind(report, HealthKindMissingProviderKey) {
		t.Fatalf("missing Tiingo key not reported: %+v", report.Issues)
	}
	if hasIssueForInstrument(report, first.ID.String(), HealthKindMissingInstrumentHistory, true) {
		t.Fatal("Tiingo gaps must not be executable while the key is missing")
	}
	if !hasIssueForInstrument(report, first.ID.String(), HealthKindMissingInstrumentHistory, false) {
		t.Fatalf("collapsed Tiingo gap missing: %+v", report.Issues)
	}
	if !hasIssueForInstrument(report, manual.ID.String(), HealthKindMissingManualPrice, false) {
		t.Fatalf("missing manual price not reported: %+v", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Kind == HealthKindSnapshotOutdated && !issue.Collapsed && issue.Executable {
			t.Fatal("snapshot errors must collapse under missing inputs")
		}
	}
}

func TestScanMarketDataHealthVerifiesRepairClearsExecutableGaps(t *testing.T) {
	tiingoInner := &syncFakeProvider{key: TiingoProviderKey, history: func(_ context.Context, identity InstrumentMarketIdentity, rng DateRange, _ int) (MappingOutcome[InstrumentDailyObservation], error) {
		out := mappedTiingoClose("2026-09-04", "185.25")
		out.Batch.VerifiedRanges = []DateRange{rng}
		return out, nil
	}}
	yahooInner := &syncFakeProvider{key: YahooFinanceProviderKey}
	tiingo := &statusProvider{countingProvider: &countingProvider{inner: tiingoInner}, code: ProviderConfigOK, reason: ""}
	yahoo := &countingProvider{inner: yahooInner}
	service, _, first, _ := newSyncFixture(t, tiingo, yahoo)
	before, err := service.ScanMarketDataHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueForInstrument(before, first.ID.String(), HealthKindMissingInstrumentHistory, true) {
		t.Fatalf("expected AAPL gap before repair: %+v", before.Issues)
	}
	started, err := service.StartMarketDataSync(context.Background(), SyncRequest{Scope: SyncScopeRepairAll})
	if err != nil || started.Conflict {
		t.Fatalf("start = %+v err=%v", started, err)
	}
	waitSyncTerminal(t, service)
	after, err := service.ScanMarketDataHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hasIssueForInstrument(after, first.ID.String(), HealthKindMissingInstrumentHistory, true) {
		t.Fatalf("AAPL still executable after repair: %+v", after.Issues)
	}
}

func hasKind(report MarketDataHealthReport, kind string) bool {
	for _, issue := range report.Issues {
		if issue.Kind == kind && !issue.Collapsed {
			return true
		}
	}
	return false
}

func kindCount(report MarketDataHealthReport, kind string) int {
	count := 0
	for _, issue := range report.Issues {
		if issue.Kind == kind && !issue.Collapsed {
			count++
		}
	}
	return count
}

func hasIssueForInstrument(report MarketDataHealthReport, instrumentID, kind string, executable bool) bool {
	for _, issue := range report.Issues {
		if issue.InstrumentID == instrumentID && issue.Kind == kind && issue.Executable == executable && !issue.Collapsed {
			return true
		}
	}
	if !executable {
		for _, issue := range report.Issues {
			if issue.InstrumentID == instrumentID && issue.Kind == kind && issue.Collapsed {
				return true
			}
		}
	}
	return false
}

func TestClassifyInstrumentHealthYahooIsNotAutoRepairable(t *testing.T) {
	need := InstrumentRepairNeed{
		InstrumentID:   domain.InstrumentID("00000000-0000-0000-0000-000000000001"),
		ProviderKey:    YahooFinanceProviderKey,
		ProviderSymbol: "MSFT",
		RouteStatus:    domain.InstrumentRouteOK,
		MissingRanges:  []DateRange{{Start: "2026-09-02", End: "2026-09-04"}},
	}
	issue, _ := classifyInstrumentHealth(need, "Microsoft", nil)
	if issue.Kind != HealthKindUnsupportedCoverage || issue.Executable {
		t.Fatalf("yahoo issue = %+v", issue)
	}
}

func TestClassifyInstrumentHealthBindingOpensEditor(t *testing.T) {
	need := InstrumentRepairNeed{
		InstrumentID: domain.InstrumentID("00000000-0000-0000-0000-000000000002"),
		ProviderKey:  TiingoProviderKey,
		RouteStatus:  domain.InstrumentRouteBindingMissing,
		SkipReason:   "binding_missing",
	}
	issue, _ := classifyInstrumentHealth(need, "AAPL", nil)
	if issue.Kind != HealthKindMissingBinding || issue.Action != HealthActionInstrumentEditor || issue.Executable {
		t.Fatalf("binding issue = %+v", issue)
	}
}
