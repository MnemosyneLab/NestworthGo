package ui

import (
	"context"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type settingsProvider struct {
	key string
}

func (p settingsProvider) Key() string { return p.key }

func (settingsProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
}

func (settingsProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	return application.LatestInstrumentQuote{}, nil
}

func (settingsProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	return application.LatestFXQuote{}, nil
}

func TestSettingsPageExposesCapabilityAwareFXProviderSelection(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "settings-provider-ui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := application.NewService(sqlite.NewRepository(database), application.NewMarketDataRegistry(
		settingsProvider{key: settings.FXProviderYahoo},
		settingsProvider{key: settings.FXProviderFrankfurter},
	))
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Settings", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	controller := NewControllerWithBackend(fyneApplication, window, nil, store, settings.Default(), service, bootstrap, nil)
	page := NewSettingsPage(controller)
	if !canvasContainsText(page, controller.translator.T("settings.providers.title")) {
		t.Fatal("settings page omitted provider section")
	}
	if !canvasContainsText(page, controller.translator.T("settings.provider.frankfurter")) {
		t.Fatal("settings page omitted Frankfurter option")
	}

	controller.updatePreference(func(next *settings.Settings) {
		next.FXProvider = settings.FXProviderFrankfurter
	})
	if got := controller.Preferences().FXProvider; got != settings.FXProviderFrankfurter {
		t.Fatalf("selected FX provider = %q, want Frankfurter", got)
	}
	if got := service.FXProviderKey(); got != settings.FXProviderFrankfurter {
		t.Fatalf("service FX provider = %q, want Frankfurter", got)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.FXProvider != settings.FXProviderFrankfurter {
		t.Fatalf("saved FX provider = %q, want Frankfurter", saved.FXProvider)
	}
	controller.updatePreference(func(next *settings.Settings) {
		next.FXProvider = "missing-provider"
	})
	if got := controller.Preferences().FXProvider; got != settings.FXProviderFrankfurter {
		t.Fatalf("unsupported FX provider changed preference to %q", got)
	}
	saved, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.FXProvider != settings.FXProviderFrankfurter {
		t.Fatalf("unsupported FX provider was persisted as %q", saved.FXProvider)
	}
}

// Regression test for docs/development/code-review-2026-08-24.md §4: selects
// must expose stable non-translated values so business logic never compares
// translated labels (a language switch or missing key must not flip a sell
// into a buy).
func TestValueSelectMapsStableValuesBehindTranslatedLabels(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	options := []selectOption{
		{value: historySideBuy, label: "买入"},
		{value: historySideSell, label: "卖出"},
	}
	vs := newValueSelect(options)
	vs.Select(historySideSell)
	if got := vs.Value(); got != historySideSell {
		t.Fatalf("Value() after Select(sell) = %q, want %q", got, historySideSell)
	}

	changed := ""
	vs.widget.OnChanged = func(string) { changed = vs.Value() }
	vs.widget.SetSelected("买入")
	if changed != historySideBuy {
		t.Fatalf("OnChanged reported %q, want %q", changed, historySideBuy)
	}

	// Replacing the option set clears a no-longer-valid selection instead of
	// letting its label masquerade as a domain value.
	vs.SetOptions([]selectOption{{value: historySideDraw, label: "取出"}})
	if got := vs.Value(); got != "" {
		t.Fatalf("Value() after option set swap = %q, want empty", got)
	}

	// An unknown label never leaks into domain space.
	vs.widget.SetSelected("未知")
	if got := vs.Value(); got == "未知" {
		t.Fatal("unknown label was returned as a domain value")
	}
}
