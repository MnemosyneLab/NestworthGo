package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Reproduce entering yesterday's deposit and first buy into an account created
// today. Later metadata edits must not erase those economic facts.
func TestBackdatedFirstTradeRebuildAndOpeningAnalysis(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/backdated.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	service := NewService(repo)
	ctx := context.Background()
	clock := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Backdated", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-09-14T02:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "1000", "CNY"), Reason: domain.ReasonContribution, EffectiveAt: time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "20"), Gross: mustMoney(t, "200", "CNY"), EffectiveAt: time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonContribution, EffectiveAt: time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	if _, err := service.RebuildHistoricalSnapshots(ctx, "2026-09-14", "2026-09-15"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := repo.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots=%d", len(snapshots))
	}
	for i, want := range []string{"1000", "1100"} {
		if !snapshots[i].Complete || snapshots[i].AssetsAmount.CanonicalAmount() != want {
			t.Fatalf("day %d: %+v", i, snapshots[i])
		}
	}
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-09-14", To: "2026-09-15", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := service.Analyze(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.RatedDays != 2 || result.Status != domain.CompletenessOK {
		t.Fatalf("coverage=%+v status=%s days=%+v", result.Coverage, result.Status, result.DailyReturns)
	}
	for _, day := range result.Days {
		if day.Status != domain.CompletenessOK || day.Residual != nil {
			t.Fatalf("unreconciled day: %+v", day)
		}
	}
	if result.InvestedCapital == nil || !result.InvestedCapital.Amount().IsZero() {
		t.Fatalf("opening=%+v", result.InvestedCapital)
	}
	// Rebuilding is idempotent, including the original multi-day batch.
	if n, err := service.RebuildHistoricalSnapshots(ctx, "2026-09-14", "2026-09-15"); err != nil || n != 0 {
		t.Fatalf("repeat=%d %v", n, err)
	}
	// Only the immutable creation state can be extended back, not this edit.
	if _, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{IncludeInNetWorth: false, IncludeInNetWorthSet: true}); err != nil {
		t.Fatal(err)
	}
	snap, _, err := service.BuildDailyValuationSnapshot(ctx, "2026-09-14")
	if err != nil || !snap.Complete || snap.AssetsAmount.CanonicalAmount() != "1000" {
		t.Fatalf("later metadata leaked: %+v %v", snap, err)
	}
}

func TestHealthDoesNotReportRoutineRechecksAsMissing(t *testing.T) {
	need := InstrumentRepairNeed{InstrumentID: domain.NewInstrumentID(), ProviderKey: YahooFinanceProviderKey, RouteStatus: domain.InstrumentRouteOK, FetchRanges: []DateRange{{Start: "2026-09-12", End: "2026-09-16"}}}
	if issue, _ := classifyInstrumentHealth(need, "Fund", nil); issue.Kind != "" {
		t.Fatalf("routine recheck reported as missing: %+v", issue)
	}
	need.MissingRanges = []DateRange{{Start: "2026-09-15", End: "2026-09-15"}}
	if issue, _ := classifyInstrumentHealth(need, "Fund", nil); issue.Kind != HealthKindMissingInstrumentHistory || !issue.Executable {
		t.Fatalf("real gap hidden: %+v", issue)
	}
}

func TestBackdatedHistoryRouteRequiresEquivalentMappingAndVerifiedClose(t *testing.T) {
	id := domain.NewInstrumentID()
	provider, symbol, market := domain.WorkerProviderKey, "QQQM", "NASDAQ"
	instrument := domain.Instrument{ID: id, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderSymbol: &symbol, MarketCode: &market, ProviderBindingRevision: 1}
	cutoff := time.Date(2026, 9, 14, 15, 59, 59, 0, time.UTC)
	binding := domain.InstrumentProviderBindingRevision{InstrumentID: id, ProviderKey: domain.TiingoProviderKey, ProviderSymbol: symbol, Market: market, Currency: "USD", BindingRevision: 1, Enabled: true, EffectiveFrom: cutoff.Add(24 * time.Hour)}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: id, UnitPrice: mustUnitPrice(t, "100"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: domain.TiingoProviderKey, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-14", ValueEffectiveAt: cutoff.Add(6 * time.Hour), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose), Revision: 1}
	coverage := []domain.InstrumentHistoryCoverage{{InstrumentID: id, ProviderKey: domain.TiingoProviderKey, BindingRevision: 1, SourcePolicyVersion: string(PriceBasisTiingoRawClose), CloseMarketDates: []string{"2026-09-14"}}}
	for _, tc := range []struct {
		name   string
		change func(*domain.InstrumentProviderBindingRevision, *domain.InstrumentQuote)
		want   string
	}{
		{"equivalent", func(_ *domain.InstrumentProviderBindingRevision, _ *domain.InstrumentQuote) {}, domain.TiingoProviderKey},
		{"different symbol", func(b *domain.InstrumentProviderBindingRevision, _ *domain.InstrumentQuote) {
			b.ProviderSymbol = "OTHER"
		}, provider},
		{"different market", func(b *domain.InstrumentProviderBindingRevision, _ *domain.InstrumentQuote) { b.Market = "SGX" }, provider},
		{"different currency", func(b *domain.InstrumentProviderBindingRevision, _ *domain.InstrumentQuote) { b.Currency = "SGD" }, provider},
		{"disabled", func(b *domain.InstrumentProviderBindingRevision, _ *domain.InstrumentQuote) { b.Enabled = false }, provider},
		{"realtime", func(_ *domain.InstrumentProviderBindingRevision, q *domain.InstrumentQuote) {
			q.ObservationKind = "realtime"
		}, provider},
		{"later date", func(_ *domain.InstrumentProviderBindingRevision, q *domain.InstrumentQuote) {
			q.EffectiveDate = "2026-09-15"
		}, provider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, q := binding, quote
			tc.change(&b, &q)
			got := backdatedInstrumentHistoryRoute(instrument, []domain.InstrumentQuote{q}, coverage, []domain.InstrumentProviderBindingRevision{b}, cutoff, "Asia/Singapore")
			if got.ProviderKey == nil || *got.ProviderKey != tc.want {
				t.Fatalf("route=%+v want %s", got, tc.want)
			}
		})
	}
}

func TestAnalysisValuesStartingPointWithoutPersistingPreviousDay(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/opening.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(sqlite.NewRepository(db))
	ctx := context.Background()
	clock := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Opening", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	b, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Savings", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Hour)
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: b.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "10", "CNY"), Reason: domain.ReasonInterest, EffectiveAt: clock}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(24 * time.Hour)
	query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-09-14", To: "2026-09-14", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true}
	result, err := service.Analyze(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.CompletenessOK || result.InvestedCapital == nil || result.InvestedCapital.Amount().String() != "1000" || result.ReturnAmount == nil || result.ReturnAmount.Amount().String() != "10" || result.ReturnRate == nil || result.ReturnRate.String() != "0.01" {
		t.Fatalf("starting point analysis=%+v", result)
	}
	var count int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM daily_valuation_snapshots WHERE local_date < '2026-09-14'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invented previous close: %d %v", count, err)
	}
}

func TestReturnGroupsKeepCompleteZeroCapitalDays(t *testing.T) {
	accountID := domain.NewAccountID()
	zero := testReturnMoney(t, "0")
	amount := testReturnMoney(t, "1")
	capital := testReturnMoney(t, "100")
	days := []domain.ComponentDay{
		{Date: "2026-09-14", Component: domain.ComponentID{AccountID: accountID, Currency: "CNY", Cash: true}, ReturnAmount: &zero, InvestedCapital: &zero, Status: domain.CompletenessOK},
		{Date: "2026-09-15", Component: domain.ComponentID{AccountID: accountID, Currency: "CNY", Cash: true}, ReturnAmount: &amount, InvestedCapital: &capital, Status: domain.CompletenessOK},
	}
	for _, ordered := range []bool{true, false} {
		copyDays := append([]domain.ComponentDay(nil), days...)
		if !ordered {
			copyDays[0], copyDays[1] = copyDays[1], copyDays[0]
		}
		result := domain.PeriodAnalysisResult{Query: domain.AnalysisQuery{IncludeCash: true}, Days: copyDays, Coverage: domain.RateCoverage{TotalDays: 2}}
		groups, err := FoldReturnGroups(result, AnalysisGroupBy("account"))
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].Status != domain.CompletenessOK || groups[0].Coverage.RatedDays != 1 || groups[0].ReturnAmount.String() != "1" {
			t.Fatalf("ordered=%v groups=%+v", ordered, groups)
		}
	}
}
