package ui

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type uiCountingProvider struct {
	calls atomic.Int32
}

func (p *uiCountingProvider) Key() string { return application.YahooFinanceProviderKey }

func (p *uiCountingProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestInstrument: true, LatestFX: true}
}

func (p *uiCountingProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	p.calls.Add(1)
	return application.LatestInstrumentQuote{}, nil
}

func (p *uiCountingProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	p.calls.Add(1)
	return application.LatestFXQuote{}, nil
}

func TestInvestmentsPageConstructionIsLocalAndProviderFree(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "investments-ui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	provider := &uiCountingProvider{}
	service := application.NewService(sqlite.NewRepository(database), application.NewMarketDataRegistry(provider))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "UI", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), service, bootstrap, nil)
	page := NewInvestmentsPage(controller)
	if page == nil {
		t.Fatal("Investments page is nil")
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("page construction contacted provider %d time(s)", provider.calls.Load())
	}
	if !canvasContainsText(page, controller.translator.T("portfolio.refreshAll")) {
		t.Fatal("Investments page does not expose explicit bulk refresh")
	}
	if canvasContainsText(page, "Search") {
		t.Fatal("Investments page unexpectedly exposes provider symbol search")
	}
	analytics := NewAnalyticsPage(controller)
	if analytics == nil {
		t.Fatal("Analytics page is nil")
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("Analytics page construction contacted provider %d time(s)", provider.calls.Load())
	}
}

func TestProviderControlsRequireCapabilityAndCompleteBinding(t *testing.T) {
	key := application.YahooFinanceProviderKey
	symbol := "QQQ"
	complete := domain.Instrument{QuoteSource: domain.QuoteSourceProvider, ProviderKey: &key, ProviderSymbol: &symbol}
	incomplete := complete
	incomplete.ProviderSymbol = nil
	manual := complete
	manual.QuoteSource = domain.QuoteSourceManual
	if !instrumentProviderBindingComplete(complete) {
		t.Fatal("complete provider binding was rejected")
	}
	if instrumentProviderBindingComplete(incomplete) || instrumentProviderBindingComplete(manual) {
		t.Fatal("incomplete or manual binding was accepted")
	}

	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "provider-controls.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	provider := &uiCountingProvider{}
	service := application.NewService(sqlite.NewRepository(database), application.NewMarketDataRegistry(provider))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "UI", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), service, bootstrap, nil)
	if !instrumentProviderCapabilityAvailable(controller) || !fxProviderCapabilityAvailable(controller) {
		t.Fatal("capable provider was not exposed to the UI")
	}
	if !instrumentProviderRefreshAvailable(controller, complete) {
		t.Fatal("complete capable binding did not expose current-price refresh")
	}

	limited := &uiCapabilityProvider{}
	limitedService := application.NewService(sqlite.NewRepository(database), application.NewMarketDataRegistry(limited))
	limitedController := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), limitedService, bootstrap, nil)
	if instrumentProviderCapabilityAvailable(limitedController) {
		t.Fatal("provider controls were exposed without latest-instrument capability")
	}
}

type uiCapabilityProvider struct{}

func (*uiCapabilityProvider) Key() string { return application.YahooFinanceProviderKey }
func (*uiCapabilityProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
}
func (*uiCapabilityProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	return application.LatestInstrumentQuote{}, nil
}
func (*uiCapabilityProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, nil
}

func TestRefreshGenerationCancelsOnNavigationAndIgnoresLateResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	controller := &Controller{page: PageInvestments, refreshTask: asyncTask[application.RefreshResult]{cancel: cancel, generation: 7, pending: true}}
	if controller.finishRefresh(6, refreshRequest{}, application.RefreshResult{}, nil) {
		t.Fatal("stale refresh completion was accepted")
	}
	if !controller.refreshTask.pending {
		t.Fatal("stale completion changed pending state")
	}
	controller.navigate(PageAccounts)
	if ctx.Err() != context.Canceled {
		t.Fatal("leaving Investments did not cancel refresh context")
	}
	if controller.refreshTask.pending {
		t.Fatal("navigation left refresh pending")
	}
}

func TestPartialRefreshIsRetryableAndDisclaimerIsLocalized(t *testing.T) {
	controller := &Controller{translator: i18n.New(settings.LanguageEnglish), refreshTask: asyncTask[application.RefreshResult]{payload: &application.RefreshResult{Items: []application.RefreshTargetResult{{Status: application.RefreshFailed}}}}, retryRefresh: func() {}}
	if !canvasContainsText(refreshFeedback(controller), controller.translator.T("common.retry")) {
		t.Fatal("partial refresh did not expose retry action")
	}
	if !canvasContainsText(providerDisclaimer(controller), controller.translator.T("portfolio.yahooDisclaimer")) {
		t.Fatal("English Yahoo disclaimer is not visible")
	}
	controller.translator.SetLanguage(settings.LanguageZhCN)
	if !canvasContainsText(providerDisclaimer(controller), "Yahoo Finance 是非官方且无关联关系的数据来源，数据可能变化或变得不可用。") {
		t.Fatal("Simplified Chinese Yahoo disclaimer is not visible")
	}
}

func TestInvestmentIdentityAndSingleRefreshFailuresStayUserFacing(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	controller := &Controller{translator: i18n.New(settings.LanguageEnglish)}
	if got := instrumentIdentityLabel(controller, "Vanguard ETF", "VOO"); got != "Vanguard ETF (VOO)" {
		t.Fatalf("instrument identity label = %q", got)
	}
	id := domain.InstrumentID("00000000-0000-7000-8000-000000000000")
	missing := missingInputText(controller, domain.MissingInputView{Kind: domain.MissingInstrumentPrice, InstrumentID: &id, InstrumentName: "Vanguard ETF", InstrumentSymbol: "VOO"}, nil)
	if missing == "" || missing == id.String() || !strings.Contains(missing, "Vanguard ETF (VOO)") {
		t.Fatalf("missing input text = %q", missing)
	}
	for _, status := range []application.RefreshStatus{application.RefreshSkipped, application.RefreshFailed, application.RefreshRateLimited} {
		if !refreshSingleTargetNeedsAttention(application.RefreshResult{Items: []application.RefreshTargetResult{{Status: status}}}) {
			t.Fatalf("single-target status %q was treated as success", status)
		}
	}
	for _, status := range []application.RefreshStatus{application.RefreshFetched, application.RefreshCached} {
		if refreshSingleTargetNeedsAttention(application.RefreshResult{Items: []application.RefreshTargetResult{{Status: status}}}) {
			t.Fatalf("single-target status %q was treated as failure", status)
		}
	}
}

func TestGainTablesRenderReadModelValuesAndLocalizedUnavailableState(t *testing.T) {
	controller := &Controller{translator: i18n.New(settings.LanguageEnglish), preference: settings.Default()}
	gain := domain.HoldingGainView{
		InstrumentName:   "Fixture ETF",
		InstrumentSymbol: "FIX",
		Quantity:         "2",
		AverageCost:      domain.MoneyView{Amount: "80", Currency: "USD"},
		TotalCost:        domain.MoneyView{Amount: "160", Currency: "USD"},
		CurrentValue:     &domain.MoneyView{Amount: "172.34", Currency: "USD"},
		UnrealizedGain:   &domain.SignedMoneyView{Amount: "12.34", Currency: "USD"},
		Available:        true,
	}
	position := investmentHoldingGainRow(controller, gain)
	for _, want := range []string{
		controller.translator.T("portfolio.averageCost"),
		controller.translator.T("portfolio.totalCost"),
		controller.translator.T("portfolio.unrealizedGain"),
		format.Money(gain.AverageCost.Amount, gain.AverageCost.Currency.String(), controller.preference),
		format.Money(gain.TotalCost.Amount, gain.TotalCost.Currency.String(), controller.preference),
		format.Money(gain.UnrealizedGain.Amount, gain.UnrealizedGain.Currency.String(), controller.preference),
	} {
		if !canvasContainsText(position, want) {
			t.Fatalf("gain table is missing %q", want)
		}
	}

	gain.Available = false
	gain.CurrentValue = nil
	gain.UnrealizedGain = nil
	gain.MissingReason = "current instrument price is unavailable"
	position = investmentHoldingGainRow(controller, gain)
	if !canvasContainsText(position, controller.translator.T("portfolio.unavailable")) {
		t.Fatal("unavailable gain did not use the localized unavailable state")
	}
	if canvasContainsText(position, gain.MissingReason) {
		t.Fatal("internal missing reason leaked into the gain table")
	}

	realized := realizedGainRows(controller, domain.RealizedGainView{
		From: "2026-08-01", To: "2026-08-30", ByInstrument: []domain.GainGroupView{{Label: "Fixture ETF", Gain: domain.SignedMoneyView{Amount: "9.87", Currency: "CNY"}, Available: true}},
	})
	if !canvasContainsText(container.NewVBox(realized...), format.Money("9.87", "CNY", controller.preference)) {
		t.Fatal("realized gain table did not render the service value")
	}
	if !canvasContainsText(container.NewVBox(realized...), "Realized gain: 2026-08-01 to 2026-08-30") {
		t.Fatal("realized gain range did not use the supported text separator")
	}
	if canvasContainsText(container.NewVBox(realized...), "→") {
		t.Fatal("realized gain range still contains the unsupported arrow glyph")
	}

	controller.translator.SetLanguage(settings.LanguageZhCN)
	if !canvasContainsText(investmentHoldingGainRow(controller, gain), controller.translator.T("portfolio.unavailable")) {
		t.Fatal("Simplified Chinese unavailable state was not localized")
	}
}

func canvasContainsText(object fyne.CanvasObject, want string) bool {
	switch value := object.(type) {
	case *widget.Label:
		if value.Text == want {
			return true
		}
	case *widget.Button:
		if value.Text == want {
			return true
		}
	case *canvas.Text:
		if value.Text == want {
			return true
		}
	case *widget.Select:
		for _, option := range value.Options {
			if option == want {
				return true
			}
		}
	}
	if containerObject, ok := object.(*fyne.Container); ok {
		for _, child := range containerObject.Objects {
			if canvasContainsText(child, want) {
				return true
			}
		}
	}
	return false
}
