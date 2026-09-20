package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func decodeExport(t *testing.T, service *Service) ExportDocument {
	t.Helper()
	data, err := service.ExportJSONBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var doc ExportDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != ExportFormat || doc.FormatVersion != 1 || !doc.CurrentState.Derived {
		t.Fatalf("metadata = %+v", doc)
	}
	for _, forbidden := range []string{"apiKey", "database.sqlite", "mutationKeys", "dailySnapshots", "projectionKind", "iconKey"} {
		if strings.Contains(string(data), `"`+forbidden+`"`) {
			t.Fatalf("internal field %s was exported", forbidden)
		}
	}
	return doc
}

func TestJSONExportIncludesArchivedHistoryAndTransferCosts(t *testing.T) {
	service, ctx, bootstrap, setClock := newOnboardedService(t, "export-transfers", []string{"Owner"})
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Source", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := service.CreateAccount(ctx, AccountInput{Name: "Destination", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := service.CreateHolding(ctx, HoldingInput{AccountID: destination.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "720", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistoryWithCosts(ctx, "UTC", map[domain.HoldingID]string{source.ID: "720"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	setClock(at)
	if _, err := service.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: source.ID, ToHoldingID: target.ID, Quantity: mustQuantity(t, "2"), EffectiveAt: at}); err != nil {
		t.Fatal(err)
	}
	doc := decodeExport(t, service)
	if doc.Timezone == nil || *doc.Timezone != "UTC" {
		t.Fatalf("timezone = %v", doc.Timezone)
	}
	if len(doc.Facts.History["activities"]) == 0 || len(doc.Facts.History["effects"]) == 0 || len(doc.Facts.History["openingPositions"]) == 0 {
		t.Fatal("lost ledger facts")
	}
	targetID := target.ID.String()
	found := false
	for _, holding := range doc.CurrentState.Holdings {
		if holding.HoldingID == targetID {
			found = true
			if holding.CostStatus != "complete" || holding.AverageUnitCost == nil || holding.AverageUnitCost.Amount != "720" {
				t.Fatalf("transfer cost = %+v", holding)
			}
		}
	}
	if !found {
		t.Fatal("missing transfer target")
	}
	at = at.Add(time.Hour)
	setClock(at)
	if _, err := service.RecordChange(ctx, domain.PositionTransferInput{HouseholdID: bootstrap.Household.ID, FromHoldingID: target.ID, ToHoldingID: source.ID, Quantity: mustQuantity(t, "2"), EffectiveAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveHolding(ctx, target.ID, true); err != nil {
		t.Fatal(err)
	}
	archived := decodeExport(t, service)
	for _, holding := range archived.CurrentState.Holdings {
		if holding.HoldingID == targetID && !holding.Archived {
			t.Fatal("archive state missing")
		}
	}
	if len(archived.Facts.Directory["holdings"]) != len(doc.Facts.Directory["holdings"]) || len(archived.Facts.History["activities"]) != len(doc.Facts.History["activities"])+1 {
		t.Fatal("archive filtering dropped history")
	}
	for _, holding := range doc.Facts.Directory["holdings"] {
		if _, ok := holding["quantity"]; ok {
			t.Fatal("derived quantity mixed into holding definition")
		}
	}
}

func TestJSONExportPreservesMissingValuesAndDecimalStrings(t *testing.T) {
	service, ctx, bootstrap, _ := newOnboardedService(t, "export", []string{"Owner"})
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Unpriced", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1.12345678"})
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeExport(t, service)
	state := doc.CurrentState.Holdings[0]
	if state.HoldingID != holding.ID.String() || state.Quantity != "1.12345678" || state.NativeValue != nil || state.BaseValue != nil || state.ValuationComplete || len(state.MissingInputs) == 0 {
		t.Fatalf("missing quote state = %+v", state)
	}
	if state.CostBasis != nil || state.AverageUnitCost != nil || state.CostStatus != "unavailable" {
		t.Fatalf("missing cost became zero: %+v", state)
	}
	if doc.CurrentState.Accounts[0].Complete || doc.CurrentState.Accounts[0].ValuedSubtotal != nil {
		t.Fatal("missing valuation became zero")
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "123.12345678", "2026-08-01", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "124.12345678", "2026-08-01T01:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	doc = decodeExport(t, service)
	state = doc.CurrentState.Holdings[0]
	if state.NativeValue == nil || state.BaseValue != nil || state.ValuationComplete {
		t.Fatalf("missing FX state = %+v", state)
	}
	if len(doc.Facts.MarketData["instrumentQuotes"]) != 2 {
		t.Fatal("export did not retain all quote history")
	}
	for _, quote := range doc.Facts.MarketData["instrumentQuotes"] {
		if _, ok := quote["unitPrice"].(string); !ok {
			t.Fatal("price must be a decimal string")
		}
		if _, ok := quote["delayed"].(bool); !ok {
			t.Fatal("flag must be boolean")
		}
	}
}

func TestJSONExportStableEmptyCollectionsAndNoWrites(t *testing.T) {
	service, ctx, _, _ := newOnboardedService(t, "export-empty", []string{"Owner"})
	before, err := service.CurrentDatabasePreview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ExportJSONBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ExportJSONBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("same state and clock should produce identical exports")
	}
	var doc ExportDocument
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.CurrentState.Accounts == nil || doc.CurrentState.Holdings == nil || doc.Facts.History["activities"] == nil || doc.Facts.MarketData["instrumentQuotes"] == nil {
		t.Fatal("empty collection encoded as null")
	}
	after, err := service.CurrentDatabasePreview(ctx)
	if err != nil || before != after {
		t.Fatalf("export changed database: %+v -> %+v, %v", before, after, err)
	}
}
