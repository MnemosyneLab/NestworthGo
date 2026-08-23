package ui

import (
	"context"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPortfolioViewsBuildForHoldingsAccountsAndExposeManualMode(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "portfolio-ui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := application.NewService(sqlite.NewRepository(database))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "UI", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Brokerage", PrimaryCategory: string(domain.CategoryInvestment), SecondaryCategory: string(domain.SecondaryBrokerageAccount), TrackingMode: string(domain.TrackingHoldings), DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInInvestment: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{Name: "Manual ETF", Type: string(domain.InstrumentETF), QuoteCurrency: "CNY", QuoteSource: string(domain.QuoteSourceManual)})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, application.HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "10", "2026-08-23", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "5", "CNY", "2026-08-23"); err != nil {
		t.Fatal(err)
	}
	bootstrap, err = service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), service, bootstrap, nil)
	if page := NewAccountsPage(controller); page == nil {
		t.Fatal("Accounts page is nil")
	}
	valuation, err := service.AccountValuation(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	holdings, err := service.ListHoldings(ctx, account.Account.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	cashValues, err := service.ListAccountCashValues(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	instruments, err := service.ListInstruments(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := accountValuationSummary(controller, valuation); got == controller.translator.T("accounts.noValue") {
		t.Fatalf("Holdings account rendered without valuation: %q", got)
	}
	if content := accountDetailContent(controller, valuation, holdings, cashValues, instruments, func() {}); content == nil {
		t.Fatal("account detail content is nil")
	}
	options := trackingOptionsForCategory(domain.CategoryInvestment)
	if len(options) != 2 || options[0] != string(domain.TrackingHoldings) || options[1] != string(domain.TrackingManualValue) {
		t.Fatalf("investment tracking options = %v", options)
	}
	if holding.ID == "" {
		t.Fatal("holding was not persisted")
	}
}
