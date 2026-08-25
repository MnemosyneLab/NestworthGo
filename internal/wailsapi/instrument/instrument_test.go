package instrument_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func onboardedService(t *testing.T) *instrument.Service {
	t.Helper()
	app := wailstest.NewService(t)
	if err := household.NewService(app).CompleteOnboarding(context.Background(), household.CompleteOnboardingRequest{
		HouseholdName: "H", BaseCurrency: "USD", MemberNames: []string{"Alice"},
	}); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	return instrument.NewService(app)
}

func TestCreateAndListInstrument(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	created, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	if created.Name != "NVIDIA" || created.QuoteSource != "manual" {
		t.Fatalf("created = %+v", created)
	}
	list, err := service.ListInstruments(ctx, false)
	if err != nil {
		t.Fatalf("ListInstruments: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want one instrument matching created", list)
	}
}

func TestCreateInstrumentRequiresProviderBindingWhenSourceIsProvider(t *testing.T) {
	service := onboardedService(t)
	_, err := service.CreateInstrument(context.Background(), instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider",
	})
	if err == nil {
		t.Fatal("want a validation error when provider source lacks a provider key/symbol")
	}
	wireErr, ok := apierror.Parse(err.Error())
	if !ok || wireErr.Code != "validation" {
		t.Fatalf("err = %v, want validation code", err)
	}
}

func TestUpdateInstrumentPartialMerge(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	created, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	symbol := "NVDA"
	updated, err := service.UpdateInstrument(ctx, created.ID, instrument.InstrumentRequest{
		Name: "NVIDIA Corp", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual", Symbol: &symbol,
	})
	if err != nil {
		t.Fatalf("UpdateInstrument: %v", err)
	}
	if updated.Name != "NVIDIA Corp" || updated.Symbol == nil || *updated.Symbol != "NVDA" {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestArchiveInstrument(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	created, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	if err := service.ArchiveInstrument(ctx, created.ID, true); err != nil {
		t.Fatalf("ArchiveInstrument: %v", err)
	}
	active, err := service.ListInstruments(ctx, false)
	if err != nil {
		t.Fatalf("ListInstruments: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active = %+v, want empty after archive", active)
	}
}

func TestSetInstrumentQuoteSourceInvalid(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	created, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	err = service.SetInstrumentQuoteSource(ctx, created.ID, "not-a-source")
	if err == nil {
		t.Fatal("want a validation error for an unsupported quote source")
	}
}

func TestInstrumentDTORoundTripsAsJSON(t *testing.T) {
	service := onboardedService(t)
	created, err := service.CreateInstrument(context.Background(), instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("round-trip produced an empty object")
	}
}
