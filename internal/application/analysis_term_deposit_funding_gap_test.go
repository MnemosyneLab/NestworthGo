package application

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// A term deposit may record its contract start before the cash that funds it.
// Return Analysis must keep the computable panel instead of failing closed.
func TestReturnAnalysisSurvivesTermDepositStartBeforeFundingCash(t *testing.T) {
	started := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	service, ctx := newProductTestService(t, started)
	now := time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accountRecord, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	account := accountRecord.Account.ID
	if _, err := service.AppendAccountCashValue(ctx, account, "150000", "USD", "2026-10-08T04:00:00Z"); err != nil {
		t.Fatalf("funding cash: %v", err)
	}
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-10-01T04:00:00Z",
		Terms: ProductTermsInput{
			Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-10-01", MaturityOn: strPtr("2026-12-20"),
			InterestMode: "simple_act_365", AnnualRatePercent: strPtr("3.525"), InterestPaidThroughOn: strPtr("2026-10-01"),
		},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatalf("preview open: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, openCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatalf("record open: %v", err)
	}
	activities, err := service.ListActivities(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	var openActivity string
	for _, activity := range activities {
		if activity.EffectiveLocalDate == "2026-10-01" {
			openActivity = activity.ID.String()
		}
	}
	if openActivity == "" {
		t.Fatal("product open activity was not recorded")
	}

	var logs bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))

	query := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-10-01", To: "2026-10-18",
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	}
	calendar, err := service.ReturnCalendar(ctx, query, "", "day")
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if !calendar.Available || calendar.Status == domain.CompletenessUnavailable {
		t.Fatalf("calendar unavailable status=%s reason=%q days=%d", calendar.Status, calendar.MissingReason, len(calendar.Days))
	}
	if calendar.MissingReason != fundingCashAfterProductStart {
		t.Fatalf("calendar reason = %q", calendar.MissingReason)
	}
	if calendar.Summary.EndingInvestedValue == nil || !calendar.Summary.EndingInvestedValue.Amount().Equal(decimal.RequireFromString("150000")) {
		t.Fatalf("ending invested = %v, want 150000", calendar.Summary.EndingInvestedValue)
	}
	gapIssue := false
	for _, day := range calendar.Days {
		switch day.Date {
		case "2026-10-02":
			if day.EndingInvestedValue == nil || !day.EndingInvestedValue.Amount().Equal(decimal.RequireFromString("100000")) {
				t.Fatalf("gap-day ending = %v, want the deposit principal", day.EndingInvestedValue)
			}
			if day.Status != domain.CompletenessPartial || day.MissingReason != fundingCashAfterProductStart {
				t.Fatalf("gap day status=%s reason=%q", day.Status, day.MissingReason)
			}
		case "2026-10-09":
			if day.EndingInvestedValue == nil || !day.EndingInvestedValue.Amount().Equal(decimal.RequireFromString("150000")) {
				t.Fatalf("funded-day ending = %v, want cash 50000 plus principal 100000", day.EndingInvestedValue)
			}
			if day.Status != domain.CompletenessOK || day.MissingReason != "" {
				t.Fatalf("funded day status=%s reason=%q", day.Status, day.MissingReason)
			}
		}
		for _, issue := range day.Issues {
			if issue.MissingReason == fundingCashAfterProductStart && issue.Date >= "2026-10-01" && issue.Date <= "2026-10-07" {
				gapIssue = true
			}
		}
	}
	if !gapIssue {
		t.Fatal("gap days are missing the funding reason")
	}

	trend, err := service.ReturnTrend(ctx, query, ReturnTrendPeriodReturnAmount)
	if err != nil || !trend.Available {
		t.Fatalf("trend err=%v available=%v reason=%q", err, trend.Available, trend.MissingReason)
	}
	contribution, err := service.Contribution(ctx, query, ContributionTotalReturn, ContributionGroupInstrument, ContributionSortAmountDesc)
	if err != nil || !contribution.Available {
		t.Fatalf("contribution err=%v available=%v reason=%q", err, contribution.Available, contribution.MissingReason)
	}
	if contribution.MissingReason != fundingCashAfterProductStart {
		t.Fatalf("contribution reason = %q", contribution.MissingReason)
	}
	for _, row := range contribution.Rows {
		if strings.HasPrefix(row.Key, "cash:") || row.Status == domain.CompletenessOK {
			continue
		}
		if row.MissingReason != fundingCashAfterProductStart {
			t.Fatalf("product contribution %s reason=%q", row.Key, row.MissingReason)
		}
	}
	change, err := service.AssetChange(ctx, query)
	if err != nil || !change.Available {
		t.Fatalf("asset change err=%v available=%v reason=%q", err, change.Available, change.MissingReason)
	}
	assertFundingGapHistoricalTotals(t, service, ctx)
	assertFundingGapContributionUnknown(t, service, ctx)

	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	markedProduct := false
	for _, snapshot := range snapshots {
		if snapshot.LocalDate < "2026-10-01" || snapshot.LocalDate > "2026-10-18" {
			continue
		}
		var holding, cashItem *domain.DailyValuationSnapshotItem
		for index := range snapshot.Items {
			item := &snapshot.Items[index]
			if item.AccountID != account {
				continue
			}
			if item.HoldingID != nil {
				holding = item
				continue
			}
			if item.NativeCurrency == "USD" {
				cashItem = item
			}
		}
		if holding == nil {
			t.Fatalf("%s is missing the term deposit", snapshot.LocalDate)
		}
		if !holding.Complete || !decimalAmountEqual(holding.NativeAmount, "100000") {
			t.Fatalf("%s deposit = complete:%v native:%q", snapshot.LocalDate, holding.Complete, holding.NativeAmount)
		}
		if snapshot.LocalDate < "2026-10-08" {
			if holding.MissingReason == nil || *holding.MissingReason != fundingCashAfterProductStart {
				t.Fatalf("%s deposit reason = %v", snapshot.LocalDate, holding.MissingReason)
			}
			if cashItem == nil || cashItem.Complete || cashItem.NativeAmount != "" || cashItem.MissingReason == nil || *cashItem.MissingReason != fundingCashAfterProductStart {
				t.Fatalf("%s cash = %#v", snapshot.LocalDate, cashItem)
			}
			markedProduct = true
			continue
		}
		if holding.MissingReason != nil {
			t.Fatalf("%s deposit still marked after funding: %s", snapshot.LocalDate, *holding.MissingReason)
		}
		if cashItem == nil || !cashItem.Complete || !decimalAmountEqual(cashItem.NativeAmount, "50000") || cashItem.MissingReason != nil {
			native := ""
			if cashItem != nil {
				native = cashItem.NativeAmount
			}
			t.Fatalf("%s cash = %q complete=%v", snapshot.LocalDate, native, cashItem != nil && cashItem.Complete)
		}
	}
	if !markedProduct {
		t.Fatal("term deposit was not marked on a gap day or contribution row")
	}
	logged := logs.String()
	for _, part := range []string{
		"historical replay reached a negative balance",
		account.String(),
		"USD",
		openActivity,
		"2026-10-01",
	} {
		if !strings.Contains(logged, part) {
			t.Fatalf("log missing %q:\n%s", part, logged)
		}
	}
}

func assertFundingGapHistoricalTotals(t *testing.T, service *Service, ctx context.Context) {
	t.Helper()
	overview, err := service.HistoricalOverview(ctx, "2026-10-02", "2026-10-09")
	if err != nil {
		t.Fatalf("historical overview: %v", err)
	}
	if overview.Left.Complete || overview.Left.Assets != nil || overview.Left.NetWorth != nil {
		t.Fatalf("2026-10-02 presented as complete: complete=%v assets=%v net=%v", overview.Left.Complete, overview.Left.Assets, overview.Left.NetWorth)
	}
	if !decimalAmountEqual(overview.Left.KnownAssets, "100000") {
		t.Fatalf("known assets = %s, want the deposit principal", overview.Left.KnownAssets)
	}
	if overview.Left.DisplayValuation.Assets.Total != nil || overview.Left.DisplayValuation.NetWorth.Total != nil || overview.Left.DisplayValuation.Assets.Quality.Availability == domain.ValuationFull {
		t.Fatalf("gap display totals = assets:%v net:%v availability:%s", overview.Left.DisplayValuation.Assets.Total, overview.Left.DisplayValuation.NetWorth.Total, overview.Left.DisplayValuation.Assets.Quality.Availability)
	}
	if overview.Right == nil || !overview.Right.Complete || overview.Right.Assets == nil || !decimalAmountEqual(*overview.Right.Assets, "150000") {
		assets := ""
		if overview.Right != nil && overview.Right.Assets != nil {
			assets = *overview.Right.Assets
		}
		t.Fatalf("funded day assets=%q complete=%v", assets, overview.Right != nil && overview.Right.Complete)
	}
	if overview.Change == nil || overview.Change.NetWorth != nil {
		t.Fatal("net-worth change treated the gap day as a complete endpoint")
	}
	gapCash := false
	for _, row := range overview.Rows {
		if row.Kind != "cash" || row.Left == nil {
			continue
		}
		if row.Left.NativeAmount != nil || row.Left.BaseAmount != nil || row.Left.Complete {
			t.Fatalf("gap cash still has a complete amount: %#v", row.Left)
		}
		for _, reason := range row.Left.Missing {
			if reason == historicalFundingGapReason {
				gapCash = true
			}
		}
	}
	if !gapCash {
		t.Fatal("historical overview cash row is missing the funding-gap reason")
	}
	partialCategory := false
	for _, category := range overview.Categories {
		if category.Left.Status == "partial" {
			partialCategory = true
			if category.Left.Amount != nil {
				t.Fatalf("partial category %s still publishes a complete amount %s", category.Key, *category.Left.Amount)
			}
		}
	}
	if !partialCategory {
		t.Fatal("category comparison treated the funding gap as complete")
	}
	series, err := service.HistoricalOverviewSeries(ctx, "2026-10-02", "2026-10-09")
	if err != nil {
		t.Fatalf("historical series: %v", err)
	}
	for _, point := range series.Points {
		switch point.Date {
		case "2026-10-02":
			if point.AssetsComplete || point.Assets != nil || point.NetWorth != nil || point.DisplayValuation.Assets.Total != nil {
				t.Fatalf("trend point presented as complete: %+v", point)
			}
			found := false
			for _, missing := range point.Missing {
				for _, reason := range missing.Reasons {
					if reason == historicalFundingGapReason {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("trend point missing reasons = %#v", point.Missing)
			}
		case "2026-10-09":
			if !point.AssetsComplete || point.Assets == nil || !decimalAmountEqual(*point.Assets, "150000") {
				t.Fatalf("funded trend point = complete:%v assets:%v", point.AssetsComplete, point.Assets)
			}
		}
	}
}

func assertFundingGapContributionUnknown(t *testing.T, service *Service, ctx context.Context) {
	t.Helper()
	query := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-10-02", To: "2026-10-07",
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	}
	for _, groupBy := range []ContributionGroupBy{ContributionGroupInstrument, ContributionGroupAccount, ContributionGroupCurrency, ContributionGroupAssetClass} {
		contribution, err := service.Contribution(ctx, query, ContributionTotalReturn, groupBy, ContributionSortAmountDesc)
		if err != nil {
			t.Fatalf("contribution %s: %v", groupBy, err)
		}
		for _, row := range contribution.Rows {
			if row.Status == domain.CompletenessUnavailable && row.Amount != nil {
				t.Fatalf("%s contribution %s published an amount for an unavailable return: %v", groupBy, row.Key, row.Amount)
			}
		}
	}
	contribution, err := service.Contribution(ctx, query, ContributionTotalReturn, ContributionGroupInstrument, ContributionSortAmountDesc)
	if err != nil {
		t.Fatal(err)
	}
	var cash *ContributionRow
	for index := range contribution.Rows {
		row := &contribution.Rows[index]
		if strings.HasPrefix(row.Key, "cash:") {
			cash = row
		}
	}
	if cash == nil || cash.Amount != nil || cash.Status != domain.CompletenessUnavailable || cash.MissingReason != fundingCashAfterProductStart {
		t.Fatalf("cash contribution = %#v rows=%d", cash, len(contribution.Rows))
	}
	item, err := service.ContributionItem(ctx, query, ContributionTotalReturn, ContributionGroupInstrument, cash.Key)
	if err != nil {
		t.Fatal(err)
	}
	if item.Amount != nil || item.Status != domain.CompletenessUnavailable || item.MissingReason != fundingCashAfterProductStart {
		t.Fatalf("cash contribution item = amount:%v status:%s reason:%q", item.Amount, item.Status, item.MissingReason)
	}
}

func decimalAmountEqual(got, want string) bool {
	left, err := decimal.NewFromString(got)
	if err != nil {
		return false
	}
	right, err := decimal.NewFromString(want)
	if err != nil {
		return false
	}
	return left.Equal(right)
}
