package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestDebtPaymentAppliesPrincipalAndFeeCumulatively(t *testing.T) {
	for _, mode := range []string{"balance", "holdings"} {
		t.Run(mode, func(t *testing.T) {
			database, err := sqlite.Open(t.TempDir() + "/debt.db")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			service := NewService(sqlite.NewRepository(database))
			clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
			service.setClock(func() time.Time { return clock })
			ctx := context.Background()
			if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Debt", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := service.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			owner := bootstrap.Members[0].ID
			cashInput := AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: mode, DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{owner}}
			if mode == "balance" {
				cashInput.InitialAmount = "1000"
			} else {
				cashInput.AccountType = "brokerage"
				cashInput.BalanceSheetRole = "asset"
			}
			cash, err := service.CreateAccount(ctx, cashInput)
			if err != nil {
				t.Fatal(err)
			}
			debt, err := service.CreateAccount(ctx, AccountInput{Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "200", OwnerIDs: []domain.MemberID{owner}})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "holdings" {
				if _, err := service.AppendAccountCashValue(ctx, cash.Account.ID, "1000", "CNY", "2026-08-01"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			clock = clock.Add(72 * time.Hour)
			principal, _ := domain.ParseMoney("100", "CNY")
			fee, _ := domain.ParseMoney("10", "CNY")
			preview, err := service.RecordChange(ctx, domain.DebtPaymentInput{HouseholdID: bootstrap.Household.ID, DebtAccountID: debt.Account.ID, CashAccountID: cash.Account.ID, Principal: principal, InterestOrFee: &fee, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Resulting) != 2 || preview.Resulting[1].Amount != "890" {
				t.Fatalf("preview resulting = %+v", preview.Resulting)
			}
			reloaded, err := service.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range reloaded.Accounts {
				if record.Account.ID == debt.Account.ID && (record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "100") {
					t.Fatalf("debt projection = %+v", record.LatestValue)
				}
			}
			if mode == "balance" {
				for _, record := range reloaded.Accounts {
					if record.Account.ID == cash.Account.ID && (record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != "890") {
						t.Fatalf("cash projection = %+v", record.LatestValue)
					}
				}
			} else {
				values, err := service.repository.ListAccountCashValues(ctx, cash.Account.ID)
				if err != nil || len(values) == 0 || values[0].Amount.CanonicalAmount() != "890" {
					t.Fatalf("cash projection = %+v, err=%v", values, err)
				}
			}
		})
	}
}

func TestDebtPaymentRejectsInsufficientCombinedCashWithoutWrites(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/debt-insufficient.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Debt", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	owner := bootstrap.Members[0].ID
	cash, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	debt, err := service.CreateAccount(ctx, AccountInput{Name: "Debt", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "200", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	principal, _ := domain.ParseMoney("100", "CNY")
	fee, _ := domain.ParseMoney("1", "CNY")
	if _, err := service.RecordChange(ctx, domain.DebtPaymentInput{HouseholdID: bootstrap.Household.ID, DebtAccountID: debt.Account.ID, CashAccountID: cash.Account.ID, Principal: principal, InterestOrFee: &fee, EffectiveAt: clock.Add(24 * time.Hour)}); err == nil {
		t.Fatal("insufficient combined cash was accepted")
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 0 {
		t.Fatalf("activities after rejected payment = %d", activities)
	}
}

func TestPostHistoryCreationRecordsReconciliationActivities(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/creation.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Creation", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{Name: "New cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "500", OwnerIDs: []domain.MemberID{owner}})
	if err != nil || account.LatestValue == nil || account.LatestValue.Amount.CanonicalAmount() != "500" {
		t.Fatalf("post-history account = %+v, err=%v", account, err)
	}
	holdingsAccount, err := service.CreateAccount(ctx, AccountInput{Name: "New broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: holdingsAccount.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"})
	if err != nil || holding.Quantity.Canonical() != "3" {
		t.Fatalf("post-history holding = %+v, err=%v", holding, err)
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 2 {
		t.Fatalf("reconciliation activities = %d, want 2", activities)
	}
	var adjustmentCost string
	if err := database.SQL.QueryRow("SELECT COALESCE(cost_unit_price, '') FROM activity_effects WHERE holding_id = ? AND cost_unit_price IS NOT NULL", holding.ID.String()).Scan(&adjustmentCost); err != nil {
		t.Fatal(err)
	}
	if adjustmentCost != "10" {
		t.Fatalf("persisted reconciliation unit cost = %q, want 10", adjustmentCost)
	}
	events, err := repository.ListCostBasisEvents(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != domain.CostBasisAdjustmentIn || events[0].UnitCost == nil || events[0].UnitCost.Canonical() != "10" {
		t.Fatalf("reconciliation cost-basis events = %+v", events)
	}
}

func TestSnapshotCursorPreservesEarlierDirtyDates(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/cursor.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Cursor", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	amount, _ := domain.ParseMoney("1", "CNY")
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if err := service.repository.MarkDailySnapshotCompleted(ctx, bootstrap.Household.ID, "2026-08-02", clock); err != nil {
		t.Fatal(err)
	}
	state, err := service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" {
		t.Fatalf("cursor after first date = %+v, err=%v", state, err)
	}
	if err := service.repository.MarkDailySnapshotCompleted(ctx, bootstrap.Household.ID, "2026-08-04", clock); err != nil {
		t.Fatal(err)
	}
	state, err = service.DailySnapshotState(ctx, bootstrap.Household.ID)
	if err != nil || state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-03" {
		t.Fatalf("earlier dirty date was lost = %+v, err=%v", state, err)
	}
}

func TestArchivedChangeTargetsAreRejectedAtPreviewAndCommit(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/archived-change.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Archive", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	owner := bootstrap.Members[0].ID
	cash, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	brokerTwo, err := service.CreateAccount(ctx, AccountInput{Name: "Broker two", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	from, err := service.CreateHolding(ctx, HoldingInput{AccountID: broker.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := service.CreateHolding(ctx, HoldingInput{AccountID: brokerTwo.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveAccount(ctx, cash.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("1", "CNY")
	if _, err := service.PreviewChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: cash.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock}); err == nil {
		t.Fatal("archived Account was accepted by PreviewChange")
	}
	if err := service.ArchiveHolding(ctx, from.ID, true); err != nil {
		t.Fatal(err)
	}
	quantity, _ := domain.ParseQuantity("1")
	if _, err := service.PreviewChange(ctx, domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: from.ID, ToHoldingID: to.ID, Quantity: quantity, EffectiveAt: clock}); err == nil {
		t.Fatal("archived Holding was accepted by PreviewChange")
	}
	// Build a valid preview first, then archive the target before the repository
	// commit to cover the preview-to-commit race boundary.
	if err := service.ArchiveAccount(ctx, cash.Account.ID, false); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: cash.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveAccount(ctx, cash.Account.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.repository.CommitActivity(ctx, preview.Activity, preview.Effects, preview.Resulting, clock); err == nil {
		t.Fatal("commit accepted an Account archived after preview")
	}
	var activities int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if activities != 0 {
		t.Fatalf("activities after archived writes = %d", activities)
	}
}

func TestNormalMetadataAndBackdatedQuotesAppendEvidenceAndDirtyHistory(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/observed-mutations.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Observed", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, _ := service.Bootstrap(ctx)
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{owner}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	clock = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := service.UpdateAccount(ctx, account.Account.ID, AccountInput{IncludeInNetWorth: false, IncludeInNetWorthSet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-02", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-08-02"); err != nil {
		t.Fatal(err)
	}
	var accountObservations, instrumentObservations, fxObservations int
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM account_state_observations").Scan(&accountObservations); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM instrument_preference_observations").Scan(&instrumentObservations); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRow("SELECT COUNT(*) FROM fx_preference_observations").Scan(&fxObservations); err != nil {
		t.Fatal(err)
	}
	if accountObservations != 1 || instrumentObservations != 1 || fxObservations != 1 {
		t.Fatalf("observation counts = account=%d instrument=%d fx=%d", accountObservations, instrumentObservations, fxObservations)
	}
	var dirty string
	if err := database.SQL.QueryRow("SELECT dirty_from FROM history_snapshot_state WHERE household_id = ?", bootstrap.Household.ID.String()).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != "2026-08-02" {
		t.Fatalf("dirty_from = %q, want 2026-08-02", dirty)
	}
}
