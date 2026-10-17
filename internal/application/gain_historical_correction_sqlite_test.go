package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestAccountGainCorrectionKeepsOriginalEconomicSaleFXTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/gain-correction.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	service := NewService(repo)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic correction", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Synthetic brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "100", "USD", "2026-09-25"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic USD holding", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "6", "2026-09-25T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}

	buyAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	now = buyAt
	buy, err := service.RecordChange(ctx, domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"),
		Gross: mustMoney(t, "2", "USD"), EffectiveAt: buyAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	saleAt := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	now = saleAt.Add(time.Hour)
	saleCommand := domain.TradeInput{
		HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account.Account.ID,
		HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "2"),
		Gross: mustMoney(t, "24", "USD"), EffectiveAt: saleAt,
	}
	sale, err := service.RecordChange(ctx, saleCommand)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Activity.TradeDetail == nil || buy.Activity.TradeDetail == nil {
		t.Fatal("synthetic trade details were not persisted")
	}

	// These quotes were appended after the sale was recorded. The sale-time
	// quote is 7; a correction entered on September 29 would see rate 9 if its
	// audit/entry time were incorrectly used as the realized-gain FX cutoff.
	now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "7", "2026-09-27T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "9", "2026-09-29T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	saleCommand.Gross = mustMoney(t, "26", "USD")
	saleCommand.EffectiveAt = now // The correction request date is not the economic sale date.
	fixed, err := service.FixChange(ctx, sale.Activity.ID, saleCommand)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.Activity.EffectiveAt.Equal(saleAt) || fixed.Activity.CreatedAt.Equal(saleAt) {
		t.Fatalf("correction timestamps=%+v, want original economic time %s and later audit time", fixed.Activity, saleAt)
	}

	accountGain, err := service.AccountGain(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, accountGain.RealizedGain, "168", "CNY")
	if len(accountGain.Holdings) != 1 || accountGain.Holdings[0].RealizedGain.Amount != "24" {
		t.Fatalf("corrected native holding gain=%+v, want USD 24", accountGain.Holdings)
	}
	period, err := service.RealizedGainInRange(ctx, domain.GainScope{AccountID: &account.Account.ID}, "2026-09-27", "2026-09-27")
	if err != nil || !period.Available || period.Total == nil {
		t.Fatalf("corrected sale period=%+v err=%v", period, err)
	}
	assertSignedGain(t, period.Total, "168", "CNY")
}
