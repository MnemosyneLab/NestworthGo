package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestExtremePersistedHoldingKeepsOverviewAndUpcomingActionsReadable(t *testing.T) {
	t.Parallel()
	service, ctx, bootstrap, setClock := newOnboardedService(t, "extreme-holding-read-flows", []string{"Owner"})
	setClock(time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC))
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Synthetic brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}

	addHolding := func(name, quantity, unitPrice string) domain.Holding {
		t.Helper()
		instrument, createErr := service.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		holding, createErr := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: quantity})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr = service.AppendManualInstrumentQuote(ctx, instrument.ID, unitPrice, "2026-08-01", false); createErr != nil {
			t.Fatal(createErr)
		}
		if createErr = service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); createErr != nil {
			t.Fatal(createErr)
		}
		return holding
	}
	saveScheduledPolicy := func(holding domain.Holding) {
		t.Helper()
		days, calendar, onRequest := 0, "calendar", "on_request"
		if _, policyErr := service.SaveLiquidityPolicy(ctx, SavePolicyInput{
			Source: domain.HoldingSourceRef(account.Account.ID, holding.ID),
			Policy: ProductPolicyInput{
				AccessKind: onRequest, UnlockOn: strPtr("2026-08-01"), SettlementDays: &days, DayBasis: &calendar,
				NormalExitFee: strPtr("0"), EarlyKind: "not_allowed",
			},
		}); policyErr != nil {
			t.Fatal(policyErr)
		}
	}
	findAction := func(actions domain.UpcomingActions, holding domain.Holding) *domain.UpcomingAction {
		t.Helper()
		key := domain.HoldingSourceRef(account.Account.ID, holding.ID).Key()
		for index := range actions.Items {
			if actions.Items[index].SourceKey == key {
				return &actions.Items[index]
			}
		}
		t.Fatalf("upcoming action for %s is missing: %+v", key, actions)
		return nil
	}

	ordinary := addHolding("Ordinary synthetic holding", "2", "2")
	saveScheduledPolicy(ordinary)
	controlOverview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("ordinary overview: %v", err)
	}
	if !controlOverview.Complete || controlOverview.Assets.String() != "4" || len(controlOverview.MissingInputs) != 0 {
		t.Fatalf("ordinary control overview = %+v", controlOverview)
	}
	controlActions, err := service.UpcomingActions(ctx)
	if err != nil {
		t.Fatalf("ordinary upcoming actions: %v", err)
	}
	ordinaryAction := findAction(controlActions, ordinary)
	if ordinaryAction.Amount == nil || ordinaryAction.Amount.CanonicalAmount() != "4" || ordinaryAction.AmountStatus != "known" {
		t.Fatalf("ordinary control action = %+v", ordinaryAction)
	}
	controlHealth, err := service.ScanMarketDataHealth(ctx)
	if err != nil || !controlHealth.Healthy {
		t.Fatalf("ordinary health = %+v, err=%v", controlHealth, err)
	}

	extreme := addHolding("Extreme synthetic holding", "1000000000000", "1")
	saveScheduledPolicy(extreme)
	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatalf("overview with persisted extreme holding: %v", err)
	}
	if overview.Complete || overview.Assets.String() != "4" || len(overview.MissingInputs) != 1 || overview.MissingInputs[0].Kind != domain.MissingHoldingAmount || overview.MissingInputs[0].InstrumentID == nil {
		t.Fatalf("extreme holding was not retained as an unknown partial value: %+v", overview)
	}
	assets := overview.DisplayValuation.Assets
	if assets.Total != nil || assets.KnownSubtotal == nil || assets.KnownSubtotal.Amount != "4" || assets.Quality.UnpricedCount != 1 {
		t.Fatalf("display valuation presented an incomplete total: %+v", assets)
	}

	actions, err := service.UpcomingActions(ctx)
	if err != nil {
		t.Fatalf("upcoming actions with persisted extreme holding: %v", err)
	}
	if actions.Completeness != domain.StatusPartial {
		t.Fatalf("upcoming actions completeness = %s, want partial", actions.Completeness)
	}
	ordinaryAction = findAction(actions, ordinary)
	if ordinaryAction.Amount == nil || ordinaryAction.Amount.CanonicalAmount() != "4" || ordinaryAction.AmountStatus != "known" {
		t.Fatalf("ordinary holding changed in upcoming actions: %+v", ordinaryAction)
	}
	extremeAction := findAction(actions, extreme)
	if extremeAction.Amount != nil || extremeAction.AmountStatus != "unknown" || len(extremeAction.ReasonCodes) != 1 || extremeAction.ReasonCodes[0] != "unknown_amount" {
		t.Fatalf("extreme holding was represented as an amount: %+v", extremeAction)
	}
	health, err := service.ScanMarketDataHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.Healthy || health.IssueCount != 1 || health.ExecutableCount != 0 {
		t.Fatalf("extreme holding health = %+v", health)
	}
	var found bool
	for _, issue := range health.Issues {
		if issue.Code != string(domain.MissingHoldingAmount) {
			continue
		}
		found = true
		if issue.Kind != HealthKindIncompleteValuation || issue.Severity != HealthSeverityBlocking || issue.InstrumentID != extreme.InstrumentID.String() || issue.AccountID != account.Account.ID.String() || issue.Label != "Extreme synthetic holding" || issue.Action != HealthActionInstrumentEditor || issue.Executable {
			t.Fatalf("holding amount issue = %+v", issue)
		}
	}
	if !found {
		t.Fatalf("holding amount issue is missing: %+v", health)
	}
}
