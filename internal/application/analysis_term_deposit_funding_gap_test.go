package application

import (
	"bytes"
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

	snapshots, err := service.repository.ListDailyValuationSnapshots(ctx, bootstrap.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
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
