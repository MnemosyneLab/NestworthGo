package application

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestFundingGapAttributionAcrossPurchasesAndResolution(t *testing.T) {
	s, _, owner, now := overviewFixture(t)
	ctx := t.Context()
	account := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "0", "CNY", ""); err != nil {
		t.Fatal(err)
	}
	instruments := make([]domain.Instrument, 3)
	holdings := make([]domain.Holding, 3)
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Older ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	instruments[0] = instrument
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	holdings[0] = holding
	if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}

	origin, err := s.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{holdings[0].ID: "10"})
	if err != nil {
		t.Fatal(err)
	}
	*now = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	// The current ledger can fund both buys, while closed days before August 6
	// cannot. No real data or provider is involved.
	if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "300", "CNY", "2026-08-06T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var firstActivity domain.ActivityID
	for i, name := range []string{"First purchase", "Second purchase"} {
		date := time.Date(2026, 8, i+2, 12, 0, 0, 0, time.UTC)
		command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.Account.ID.String(), Currency: "CNY", Principal: "100", EffectiveAt: date.Format(time.RFC3339), Terms: ProductTermsInput{Kind: "term_deposit", Name: name, StartOn: date.Format("2006-01-02"), MaturityOn: strPtr("2026-12-20"), InterestMode: "simple_act_365", AnnualRatePercent: strPtr("3"), InterestPaidThroughOn: strPtr(date.Format("2006-01-02"))}, Policy: depositPolicy()}}
		preview, err := s.PreviewProductOperation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := s.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstActivity = receipt.ActivityIDs[0]
		}
		product, err := s.Product(ctx, receipt.ProductIDs[0])
		if err != nil {
			t.Fatal(err)
		}
		portfolio, err := s.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, holding := range portfolio.Holdings {
			if holding.ID == product.Contract.HoldingID {
				holdings[i+1] = holding
			}
		}
		for _, instrument := range portfolio.Instruments {
			if instrument.ID == product.Contract.InstrumentID {
				instruments[i+1] = instrument
			}
		}
	}

	for day := 2; day <= 8; day++ {
		if _, err := s.AppendManualInstrumentQuote(ctx, instruments[0].ID, "11", time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), false); err != nil {
			t.Fatal(err)
		}
	}

	portfolio, err := s.historicalPortfolioSnapshot(ctx, &origin, time.Date(2026, 8, 4, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(portfolio.ReplayFundingGaps) != 1 {
		t.Fatalf("gaps=%+v", portfolio.ReplayFundingGaps)
	}
	gap := portfolio.ReplayFundingGaps[0]
	if gap.ActivityID != firstActivity || len(gap.HoldingIDs) != 2 || !slices.Contains(gap.HoldingIDs, holdings[1].ID) || !slices.Contains(gap.HoldingIDs, holdings[2].ID) || slices.Contains(gap.HoldingIDs, holdings[0].ID) {
		t.Fatalf("attribution=%+v", gap)
	}
	if _, err := s.RebuildHistoricalSnapshots(ctx, "2026-08-01", "2026-08-08"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range snapshots {
		inGap := snapshot.LocalDate >= "2026-08-02" && snapshot.LocalDate < "2026-08-06"
		if snapshot.Complete == inGap {
			t.Fatalf("%s complete=%v", snapshot.LocalDate, snapshot.Complete)
		}
		missingCash := false
		for _, item := range snapshot.Items {
			if item.HoldingID == nil {
				missingCash = !item.Complete && item.MissingReason != nil && *item.MissingReason == fundingCashAfterProductStart
				continue
			}
			affected := *item.HoldingID != holdings[0].ID && inGap
			if item.MissingReason != nil != affected {
				t.Fatalf("%s holding=%s reason=%v want affected=%v", snapshot.LocalDate, *item.HoldingID, item.MissingReason, affected)
			}
			if !item.Complete {
				t.Fatalf("known holding value lost: %+v", item)
			}
		}
		if missingCash != inGap {
			t.Fatalf("%s synthetic missing cash=%v", snapshot.LocalDate, missingCash)
		}
	}
	for _, includeCash := range []bool{true, false} {
		for i, instrument := range instruments {
			query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrument.ID.String()}, From: "2026-08-04", To: "2026-08-08", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: includeCash}
			calendar, err := s.ReturnCalendar(ctx, query, "", "day")
			if err != nil {
				t.Fatal(err)
			}
			if len(calendar.Days) != 5 {
				t.Fatalf("days=%d", len(calendar.Days))
			}
			for _, day := range calendar.Days {
				// The resolution day still has an unknown opening endpoint; the next
				// day is fully funded. The unrelated ETF is known throughout.
				affected := i > 0 && day.Date <= "2026-08-06"
				if affected {
					if day.ReturnRate != nil || day.Status == domain.CompletenessOK || day.MissingReason != fundingCashAfterProductStart {
						t.Fatalf("affected purchase cash=%v: %+v", includeCash, day)
					}
				} else if day.ReturnRate == nil || day.Status != domain.CompletenessOK || day.MissingReason != "" {
					t.Fatalf("healthy holding %s cash=%v: %+v", instrument.Name, includeCash, day)
				}
			}
		}
		query := domain.AnalysisQuery{Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-08-04", To: "2026-08-05", Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: includeCash}
		calendar, err := s.ReturnCalendar(ctx, query, "", "day")
		if err != nil {
			t.Fatal(err)
		}
		for _, day := range calendar.Days {
			if day.ReturnRate != nil || day.Status == domain.CompletenessOK {
				t.Fatalf("household gap weakened cash=%v: %+v", includeCash, day)
			}
		}
	}
	overview, err := s.HistoricalOverview(ctx, "2026-08-04", "2026-08-06")
	if err != nil {
		t.Fatal(err)
	}
	if overview.Left.Complete || overview.Left.Assets != nil || overview.Left.NetWorth != nil || overview.Right == nil || !overview.Right.Complete {
		t.Fatalf("overview totals: %+v / %+v", overview.Left, overview.Right)
	}
	parent := overviewRow(t, overview, account.Account.ID.String())
	if parent.Left.Complete || parent.Left.BaseAmount != nil {
		t.Fatalf("account total=%+v", parent.Left)
	}

	t.Run("persisted stale attribution rebuilds only requested gap once", func(t *testing.T) {
		stale := snapshots[2]
		if stale.LocalDate != "2026-08-03" {
			t.Fatalf("fixture order: %s", stale.LocalDate)
		}
		stale.ID = domain.NewDailyValuationSnapshotID()
		stale.Revision++
		stale.ContentHash = snapshotContentHashVersion + ":synthetic-old-attribution"
		for i := range stale.Items {
			stale.Items[i].ID = domain.NewDailyValuationSnapshotItemID()
			if stale.Items[i].HoldingID != nil && *stale.Items[i].HoldingID == holdings[0].ID {
				stale.Items[i].MissingReason = strPtr(fundingCashAfterProductStart)
			}
		}
		if _, err := s.repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, stale, *now); err != nil {
			t.Fatal(err)
		}
		if !snapshotNeedsRebuild(stale) || snapshotNeedsRebuild(snapshots[0]) {
			t.Fatal("targeted invalidation failed")
		}
		issues, err := s.incompleteSnapshotHealth(ctx, &origin, HistoryRepairPlan{OriginLocalDate: "2026-08-01", YesterdayLocal: "2026-08-08"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		repairable := slices.ContainsFunc(issues, func(issue HealthIssue) bool {
			return issue.RangeStart == stale.LocalDate && issue.Code == "snapshot_outdated" && issue.Executable
		})
		if !repairable {
			t.Fatal("stale gap lacks existing health repair action")
		}
		if err := s.ensureSnapshotCoverage(ctx, origin.HouseholdID, "2026-08-03", "2026-08-03"); err != nil {
			t.Fatal(err)
		}
		first, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ensureSnapshotCoverage(ctx, origin.HouseholdID, "2026-08-03", "2026-08-03"); err != nil {
			t.Fatal(err)
		}
		second, err := s.repository.ListDailyValuationSnapshots(ctx, origin.HouseholdID, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		for i, snapshot := range second {
			if snapshot.ID != first[i].ID {
				t.Fatalf("repeat rebuild: %s", snapshot.LocalDate)
			}
			if snapshot.LocalDate != stale.LocalDate {
				if snapshot.ID != snapshots[i].ID {
					t.Fatalf("unrequested day rebuilt: %s", snapshot.LocalDate)
				}
				continue
			}
			if snapshot.Revision != stale.Revision+1 || !strings.HasPrefix(snapshot.ContentHash, fundingGapSnapshotHashPrefix) {
				t.Fatalf("rebuild revision/hash=%d/%s", snapshot.Revision, snapshot.ContentHash)
			}
			for _, item := range snapshot.Items {
				if item.HoldingID != nil && *item.HoldingID == holdings[0].ID && item.MissingReason != nil {
					t.Fatal("stale unrelated annotation survived rebuild")
				}
			}
		}
	})
}

func TestFundingGapAnnotationsPreserveOtherMissingInputs(t *testing.T) {
	account := domain.NewAccountID()
	affected, unrelated := domain.NewHoldingID(), domain.NewHoldingID()
	for _, reason := range []string{"holding amount exceeds the supported range", "missing instrument price or FX rate", "historical market-data coverage is unverified"} {
		items := []domain.DailyValuationSnapshotItem{
			{AccountID: account, HoldingID: &affected, NativeCurrency: "CNY", Complete: false, MissingReason: &reason},
			{AccountID: account, HoldingID: &unrelated, NativeCurrency: "CNY", Complete: true},
		}
		items, missing := annotateReplayFundingGaps(items, []domain.ReplayFundingGap{{AccountID: account, Currency: "CNY", HoldingIDs: []domain.HoldingID{affected}}})
		if missing != 1 || len(items) != 3 || items[0].Complete || items[0].MissingReason == nil || *items[0].MissingReason != reason || items[1].MissingReason != nil || items[2].Complete || items[2].HoldingID != nil {
			t.Fatalf("missing input %q changed: %+v", reason, items)
		}
	}
}
