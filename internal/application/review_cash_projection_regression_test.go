package application

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// The review timeline is a holding projection. It must not replay a buy's
// cash debit after omitting the contribution that funded that buy.
func TestReviewPreviewFundedBuyUsesConsistentProjection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		asOf  bool
		truth domain.ReviewTruth
	}{
		{"late_metadata_unknown", false, domain.ReviewUnknown},
		{"known_metadata_true", true, domain.ReviewTrue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s := NewService(sqlite.NewRepository(db))
			now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			if tc.asOf {
				now = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			}
			s.SetClock(func() time.Time { return now })
			ctx := t.Context()
			if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic review", BaseCurrency: "USD", Timezone: "UTC", HistoryStartDate: "2026-10-01", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := s.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.asOf {
				now = time.Date(2026, 10, 1, 0, 5, 0, 0, time.UTC)
			}
			opened := "2026-10-01"
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true, OpenedOn: &opened, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			funding, _ := domain.ParseMoney("2000", "USD")
			if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: funding, Reason: domain.ReasonContribution, EffectiveAt: time.Date(2026, 10, 1, 0, 10, 0, 0, time.UTC)}); err != nil {
				t.Fatal(err)
			}
			quantity, _ := domain.ParseQuantity("10")
			gross, _ := domain.ParseMoney("1000", "USD")
			fee, _ := domain.ParseMoney("5", "USD")
			if _, err := s.RecordChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: quantity, Gross: gross, Fee: &fee, EffectiveAt: time.Date(2026, 10, 1, 0, 20, 0, 0, time.UTC)}); err != nil {
				t.Fatal(err)
			}
			items := make([]AgentMarketDataItem, 0, 17)
			for day := 1; day <= 17; day++ {
				items = append(items, AgentMarketDataItem{InstrumentID: instrument.ID.String(), Currency: "USD", Value: "100", Kind: "nav", Date: time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic NAV"})
			}
			if _, err := s.ImportAgentMarketData(ctx, uuid.NewString(), AgentMarketDataInput{Items: items}); err != nil {
				t.Fatal(err)
			}
			batch, err := s.repository.(ReviewHistoricalBatchReader).LoadReviewHistoricalBatch(ctx, bootstrap.Household.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			for _, activity := range batch.History.Activities {
				for _, effect := range activity.Effects {
					if effect.Target != domain.EffectTargetHoldingQuantity {
						t.Fatalf("review activity retained unrelated effect %s", effect.Target)
					}
				}
			}
			spec := syntheticHistoricalReviewSpec()
			spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
				{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "900", Currency: "USD", FXMode: "native"}},
				{Leaf: &domain.ReviewRuleLeaf{Metric: "cost", Operator: ">", Value: "1002", Currency: "USD", FXMode: "native"}},
			}}
			for _, policy := range []domain.ReviewTriggerPolicy{{Kind: "single"}, {Kind: "consecutive", Count: 2}, {Kind: "rolling", MinHits: 2, WindowDays: 3}} {
				spec.TriggerPolicy = policy
				preview, err := s.PreviewReviewRuleV2(ctx, spec, instrument.ID.String(), "2026-10-17", "")
				if err != nil || preview.Window.Truth != tc.truth || len(preview.Days) == 0 {
					t.Fatalf("policy %s: preview=%+v days=%+v err=%v", policy.Kind, preview.Window, preview.Days, err)
				}
				if tc.asOf {
					latest := preview.Days[len(preview.Days)-1]
					if latest.Result.Truth != domain.ReviewTrue || len(latest.Result.Leaves) != 2 || latest.Result.Leaves[0].Actual != "1000.0000" || latest.Result.Leaves[1].Actual != "1005.0000" {
						t.Fatalf("policy %s lost funded-trade values: %+v", policy.Kind, latest)
					}
				}
			}
		})
	}
}
