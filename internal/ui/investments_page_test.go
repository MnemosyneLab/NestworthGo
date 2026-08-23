package ui

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
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
	controller := &Controller{page: PageInvestments, refreshPending: true, refreshGeneration: 7}
	ctx, cancel := context.WithCancel(context.Background())
	controller.refreshCancel = cancel
	if controller.finishRefresh(6, refreshRequest{}, application.RefreshResult{}, nil) {
		t.Fatal("stale refresh completion was accepted")
	}
	if !controller.refreshPending {
		t.Fatal("stale completion changed pending state")
	}
	controller.navigate(PageAccounts)
	if ctx.Err() != context.Canceled {
		t.Fatal("leaving Investments did not cancel refresh context")
	}
	if controller.refreshPending {
		t.Fatal("navigation left refresh pending")
	}
}

func TestPartialRefreshIsRetryableAndDisclaimerIsLocalized(t *testing.T) {
	controller := &Controller{translator: i18n.New(settings.LanguageEnglish), refreshResult: &application.RefreshResult{Items: []application.RefreshTargetResult{{Status: application.RefreshFailed}}}, retryRefresh: func() {}}
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
