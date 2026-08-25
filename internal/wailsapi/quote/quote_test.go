package quote_test

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestSaveAndReadManualInstrumentQuote(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	created, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	service := quote.NewService(app)
	saved, err := service.SaveManualInstrumentQuote(ctx, created.ID, "131.70", "2026-01-15T00:00:00.000Z")
	if err != nil {
		t.Fatalf("SaveManualInstrumentQuote: %v", err)
	}
	if saved.UnitPrice != "131.7" {
		t.Fatalf("UnitPrice = %q, want 131.7", saved.UnitPrice)
	}
	current, err := service.CurrentInstrumentQuote(ctx, created.ID)
	if err != nil {
		t.Fatalf("CurrentInstrumentQuote: %v", err)
	}
	if current == nil || current.UnitPrice != "131.7" {
		t.Fatalf("current = %+v, want 131.7", current)
	}
	history, err := service.InstrumentQuoteHistory(ctx, created.ID)
	if err != nil {
		t.Fatalf("InstrumentQuoteHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %+v, want one entry", history)
	}
}

func TestCurrentInstrumentQuoteBeforeAnyQuote(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	created, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	service := quote.NewService(app)
	current, err := service.CurrentInstrumentQuote(ctx, created.ID)
	if err != nil {
		t.Fatalf("CurrentInstrumentQuote: %v", err)
	}
	if current != nil {
		t.Fatalf("current = %+v, want nil before any quote is captured", current)
	}
}

func TestFXPreferenceAndManualFXQuote(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	service := quote.NewService(app)
	preference, err := service.SetFXPreference(ctx, "SGD", "USD", "manual")
	if err != nil {
		t.Fatalf("SetFXPreference: %v", err)
	}
	if preference.SourceKind != "manual" {
		t.Fatalf("preference = %+v", preference)
	}
	saved, err := service.SaveManualFXQuote(ctx, "SGD", "USD", "0.74", "2026-01-15T00:00:00.000Z")
	if err != nil {
		t.Fatalf("SaveManualFXQuote: %v", err)
	}
	if saved.Rate != "0.74" {
		t.Fatalf("Rate = %q, want 0.74", saved.Rate)
	}
	current, err := service.CurrentFXQuote(ctx, "SGD", "USD")
	if err != nil {
		t.Fatalf("CurrentFXQuote: %v", err)
	}
	if current == nil || current.Rate != "0.74" {
		t.Fatalf("current = %+v", current)
	}
	list, err := service.ListFXPreferences(ctx)
	if err != nil {
		t.Fatalf("ListFXPreferences: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v, want one preference", list)
	}
}

func TestSetFXPreferenceRequiresBaseCurrency(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	service := quote.NewService(app)
	_, err := service.SetFXPreference(ctx, "SGD", "EUR", "manual")
	if err == nil {
		t.Fatal("want a validation error when neither currency is the household base")
	}
}
