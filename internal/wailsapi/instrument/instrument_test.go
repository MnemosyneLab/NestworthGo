package instrument_test

import (
	"context"
	"testing"

	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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

func TestUpdateInstrumentQuoteSourceInvalid(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	created, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "NVIDIA", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatalf("CreateInstrument: %v", err)
	}
	_, err = service.UpdateInstrument(ctx, created.ID, instrument.InstrumentRequest{QuoteSource: "not-a-source"})
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
	encoded, err := jsoncodec.Marshal(created)
	if err != nil {
		t.Fatalf("jsoncodec.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := jsoncodec.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("jsoncodec.Unmarshal: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("round-trip produced an empty object")
	}
}

func TestOptionalShortNameWirePatchClearAndDefault(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	symbol := "000001"
	alias := " 家庭 🌿 "
	i, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Fund class A", Symbol: &symbol, Type: "mutual_fund", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if i.ShortName != "" {
		t.Fatal("new record has nonempty alias")
	}
	updated, err := service.UpdateInstrument(ctx, i.ID, instrument.InstrumentRequest{ShortName: &alias})
	if err != nil || updated.ShortName != "家庭 🌿" {
		t.Fatalf("update %+v %v", updated, err)
	}
	retained, err := service.UpdateInstrument(ctx, i.ID, instrument.InstrumentRequest{Name: i.Name})
	if err != nil || retained.ShortName != updated.ShortName {
		t.Fatalf("omit %+v %v", retained, err)
	}
	empty := " "
	cleared, err := service.UpdateInstrument(ctx, i.ID, instrument.InstrumentRequest{ShortName: &empty})
	if err != nil || cleared.ShortName != "" || cleared.ID != i.ID || cleared.Name != i.Name || cleared.Symbol == nil || *cleared.Symbol != symbol {
		t.Fatalf("clear %+v %v", cleared, err)
	}
}

func TestOldReplaceRequestWithoutShortNamePreservesAlias(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	alias := "Alias"
	i, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Full name", ShortName: &alias, Type: "etf", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateInstrument(ctx, i.ID, instrument.InstrumentRequest{Replace: true, Name: "New full name", Type: i.Type, QuoteCurrency: i.QuoteCurrency})
	if err != nil || updated.ShortName != alias {
		t.Fatalf("old complete-form request %+v %v", updated, err)
	}
}

func TestDuplicateAliasesRetainShareClassAndArchivedIdentity(t *testing.T) {
	service := onboardedService(t)
	ctx := context.Background()
	alias := "Family fund"
	firstCode, secondCode := "000001", "000002"
	first, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Fund class A", ShortName: &alias, Symbol: &firstCode, Type: "mutual_fund", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Fund class B", ShortName: &alias, Symbol: &secondCode, Type: "mutual_fund", QuoteCurrency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("duplicate alias collapsed identities")
	}
	if err := service.ArchiveInstrument(ctx, first.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err := service.ListInstruments(ctx, false)
	if err != nil || len(active) != 1 || active[0].ID != second.ID || active[0].ShortName != alias {
		t.Fatalf("active %+v %v", active, err)
	}
	all, err := service.ListInstruments(ctx, true)
	if err != nil || len(all) != 2 {
		t.Fatalf("all %+v %v", all, err)
	}
	for _, value := range all {
		if value.ShortName != alias || value.Symbol == nil {
			t.Fatalf("identity %+v", value)
		}
	}
}
