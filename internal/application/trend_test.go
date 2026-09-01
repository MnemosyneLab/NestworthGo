package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestClosedDayRebuildFromStartsAfterLastCompleted(t *testing.T) {
	lastCompleted := "2026-07-31"
	dirty := "2026-07-10"
	cases := []struct {
		name          string
		origin        string
		yesterday     string
		lastCompleted *string
		dirtyFrom     *string
		wantFrom      string
		wantSkip      bool
	}{
		{name: "new day after completed history", origin: "2026-06-20", yesterday: "2026-08-01", lastCompleted: &lastCompleted, wantFrom: "2026-08-01"},
		{name: "already complete", origin: "2026-06-20", yesterday: "2026-07-31", lastCompleted: &lastCompleted, wantSkip: true},
		{name: "dirty range wins over last completed", origin: "2026-06-20", yesterday: "2026-08-01", lastCompleted: &lastCompleted, dirtyFrom: &dirty, wantFrom: "2026-07-10"},
		{name: "no cursor rebuilds from origin", origin: "2026-06-20", yesterday: "2026-07-01", wantFrom: "2026-06-20"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			from, skip, err := closedDayRebuildFrom(test.origin, test.yesterday, domain.DailySnapshotState{
				LastCompletedClosedOn: test.lastCompleted,
				DirtyFrom:             test.dirtyFrom,
			})
			if err != nil {
				t.Fatalf("closedDayRebuildFrom: %v", err)
			}
			if skip != test.wantSkip || from != test.wantFrom {
				t.Fatalf("from=%q skip=%v, want from=%q skip=%v", from, skip, test.wantFrom, test.wantSkip)
			}
		})
	}
}

func TestTrendChartsIncludeTodayWhenHistoryStartsToday(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "trend-today", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	setClock(time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	brokerage, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatalf("brokerage: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "AAPL", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerage.Account.ID.String(), InstrumentID: stock.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatalf("holding: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "50", "2026-08-15", false); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holding.ID: "50"}); err != nil {
		t.Fatalf("StartHistoryWithCosts: %v", err)
	}

	wealth, err := service.NetWorthTrend(ctx, domain.Trend30Days)
	if err != nil {
		t.Fatalf("NetWorthTrend: %v", err)
	}
	if len(wealth.Points) != 1 || wealth.Points[0].LocalDate != "2026-08-15" {
		t.Fatalf("today wealth = %+v, want a single 2026-08-15 point", wealth.Points)
	}
	if wealth.Points[0].NetWorth == nil || wealth.Points[0].NetWorth.CanonicalAmount() != "100" {
		t.Fatalf("today net worth = %+v, want 100", wealth.Points[0].NetWorth)
	}

	portfolio, err := service.PortfolioTrend(ctx, domain.Trend30Days)
	if err != nil {
		t.Fatalf("PortfolioTrend: %v", err)
	}
	if len(portfolio.Points) != 1 || portfolio.Points[0].LocalDate != "2026-08-15" {
		t.Fatalf("today portfolio = %+v, want a single 2026-08-15 point", portfolio.Points)
	}
	if portfolio.Points[0].ValuedSubtotal == nil || portfolio.Points[0].ValuedSubtotal.CanonicalAmount() != "100" {
		t.Fatalf("holdings-only today point = %+v, want 100", portfolio.Points[0].ValuedSubtotal)
	}
}

func TestNetWorthTrendRebuildsHistoryLongerThan31DaysFromLastCompleted(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "trend-long-history", []string{"Owner"})
	owner := bootstrap.Members[0].ID
	setClock(time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "CNY", IncludeInNetWorth: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "100",
	}); err != nil {
		t.Fatalf("account: %v", err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatalf("StartHistory: %v", err)
	}
	if _, err := service.RebuildHistoricalSnapshots(ctx, "2026-06-20", "2026-07-30"); err == nil {
		t.Fatal("RebuildHistoricalSnapshots accepted more than 31 days")
	}

	setClock(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	trend, err := service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("NetWorthTrend over 31 days: %v", err)
	}
	if len(trend.Points) < 40 {
		t.Fatalf("points = %d, want closed days plus today", len(trend.Points))
	}
	state, err := service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-07-31" {
		t.Fatalf("last completed = %+v err=%v, want 2026-07-31", state, err)
	}

	from, skip, err := closedDayRebuildFrom("2026-06-20", "2026-08-01", state)
	if err != nil || skip || from != "2026-08-01" {
		t.Fatalf("incremental rebuild from = %q skip=%v err=%v, want 2026-08-01", from, skip, err)
	}

	setClock(time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	trend, err = service.NetWorthTrend(ctx, domain.TrendAllTime)
	if err != nil {
		t.Fatalf("new-day NetWorthTrend: %v", err)
	}
	if trend.Points[len(trend.Points)-1].LocalDate != "2026-08-02" {
		t.Fatalf("latest point = %+v, want 2026-08-02", trend.Points[len(trend.Points)-1])
	}
	state, err = service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != "2026-08-01" {
		t.Fatalf("last completed after new day = %+v err=%v, want 2026-08-01", state, err)
	}
}
