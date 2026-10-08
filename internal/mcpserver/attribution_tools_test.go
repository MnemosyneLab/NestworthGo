package mcpserver

import (
	"context"
	"reflect"
	"testing"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
)

func addDividendAttribution(t *testing.T, app *application.Service, suffix, dividend string) string {
	t.Helper()
	ctx := context.Background()
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v, %v", members, err)
	}
	record, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Brokerage " + suffix, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
		OwnerIDs: []string{members[0].ID.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{
		Name: "Fund " + suffix, Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	assetID, err := domain.ParseInstrumentID(asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2025-03-02", "2025-03-03"} {
		if _, err := app.AppendManualInstrumentQuote(ctx, assetID, "50", date, false); err != nil {
			t.Fatal(err)
		}
	}
	h := history.NewService(app)
	if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeMoneyAdded, AccountID: record.Account.ID, Amount: "1000", Currency: "USD",
		Reason: "contribution", EffectiveAt: "2025-03-02T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	trade, err := h.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeTrade, SettlementAccountID: record.Account.ID, InstrumentID: asset.ID,
		Side: "buy", Quantity: "10", Gross: "500", GrossCurrency: "USD", Fee: "0", FeeCurrency: "USD",
		EffectiveAt: "2025-03-02T11:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if trade.Activity.TradeDetail == nil {
		t.Fatal("trade lacks holding ID")
	}
	if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
		Kind: history.ChangeCashDividend, HoldingID: trade.Activity.TradeDetail.HoldingID,
		Amount: dividend, Currency: "USD", EffectiveAt: "2025-03-03T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	return record.Account.ID
}

func TestAttributionToolsHTTPParityAndValidation(t *testing.T) {
	t.Parallel()
	s, app, _, _ := historyFixture(t)
	t.Cleanup(s.Close)
	brokerageID := addDividendAttribution(t, app, "A", "5")
	secondBrokerageID := addDividendAttribution(t, app, "B", "7")
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"list_contributions", "get_contribution_item", "get_return_day", "get_asset_driver_detail"} {
		found := false
		for _, tool := range listed.Tools {
			if tool.Name == name {
				found = true
				if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
					t.Fatalf("%s must disclose non-destructive derived writes", name)
				}
				if name == "list_contributions" {
					schema := jsonObject(t, tool.InputSchema)
					properties := schema["properties"].(map[string]any)
					for _, field := range []string{"returnType", "groupBy", "ordering"} {
						if len(properties[field].(map[string]any)["enum"].([]any)) == 0 {
							t.Fatalf("%s schema has no %s enum", name, field)
						}
					}
					queryProperties := properties["query"].(map[string]any)["properties"].(map[string]any)
					for _, field := range []string{"scopeKind", "valuation", "basis"} {
						if len(queryProperties[field].(map[string]any)["enum"].([]any)) == 0 {
							t.Fatalf("%s query schema has no %s enum", name, field)
						}
					}
				}
			}
		}
		if !found {
			t.Fatalf("missing tool %s", name)
		}
	}
	ctx := context.Background()
	query := map[string]any{"from": "2025-03-02", "to": "2025-03-03", "includeCash": true}
	request := analysis.AnalysisQueryRequest{From: "2025-03-02", To: "2025-03-03", IncludeCash: true}
	adapter := analysis.NewService(app)

	contribution := call(t, client, "list_contributions", map[string]any{
		"query": query, "returnType": "dividend_interest", "groupBy": "account", "limit": 1,
	}, false)["data"].(map[string]any)
	wantContribution, err := adapter.Contribution(ctx, request, "dividend_interest", "account", "amount_desc")
	if err != nil {
		t.Fatal(err)
	}
	if len(wantContribution.Rows) != 2 {
		t.Fatalf("fixture must produce two contribution rows, got %+v", wantContribution.Rows)
	}
	if contribution["returnType"] != "dividend_interest" || contribution["groupBy"] != "account" || contribution["status"] != wantContribution.Status || contribution["ratedDays"] != float64(wantContribution.RatedDays) || contribution["totalDays"] != float64(wantContribution.TotalDays) {
		t.Fatalf("contribution metadata: %+v", contribution)
	}
	if contribution["limit"] != float64(1) || contribution["totalRows"] != float64(len(wantContribution.Rows)) {
		t.Fatalf("contribution page metadata: %+v", contribution)
	}
	rows := contribution["rows"].([]any)
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], jsonObject(t, wantContribution.Rows[0])) {
		t.Fatalf("contribution rows: %+v, want %+v", rows, wantContribution.Rows)
	}
	if contribution["hasMore"] != true || contribution["nextOffset"] != float64(1) {
		t.Fatalf("first contribution page: %+v", contribution)
	}
	second := call(t, client, "list_contributions", map[string]any{
		"query": query, "returnType": "dividend_interest", "groupBy": "account", "limit": 1, "offset": 1,
	}, false)["data"].(map[string]any)
	if second["hasMore"] != false || second["nextOffset"] != nil || !reflect.DeepEqual(second["rows"].([]any)[0], jsonObject(t, wantContribution.Rows[1])) {
		t.Fatalf("second contribution page: %+v", second)
	}
	if len(wantContribution.Rows) > 0 {
		key := wantContribution.Rows[0].Key
		if key != brokerageID && key != secondBrokerageID {
			t.Fatalf("contribution key = %q, want brokerage IDs %q or %q", key, brokerageID, secondBrokerageID)
		}
		item := call(t, client, "get_contribution_item", map[string]any{
			"query": query, "returnType": "dividend_interest", "groupBy": "account", "groupKey": key,
		}, false)["data"].(map[string]any)
		wantItem, err := adapter.ContributionItem(ctx, request, "dividend_interest", "account", key)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(item, jsonObject(t, wantItem)) {
			t.Fatalf("contribution item = %+v, want %+v", item, wantItem)
		}
	}

	day := call(t, client, "get_return_day", map[string]any{"query": query, "date": "2025-03-03"}, false)["data"]
	wantDay, err := adapter.ReturnDay(ctx, request, "2025-03-03")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(day, jsonObject(t, wantDay)) {
		t.Fatalf("return day = %+v, want %+v", day, wantDay)
	}
	driver := call(t, client, "get_asset_driver_detail", map[string]any{"query": query, "driverKey": "income"}, false)["data"]
	wantDriver, err := adapter.AssetDriverDetail(ctx, request, "income")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(driver, jsonObject(t, wantDriver)) {
		t.Fatalf("asset driver = %+v, want %+v", driver, wantDriver)
	}

	for _, args := range []map[string]any{
		{"query": query, "returnType": "total_return", "groupBy": "account", "limit": 101},
		{"query": query, "returnType": "total_return", "groupBy": "account", "offset": -1},
		{"query": query, "returnType": "bad", "groupBy": "account"},
		{"query": query, "returnType": "total_return", "groupBy": "bad"},
		{"query": query, "returnType": "total_return", "groupBy": "account", "ordering": "bad"},
		{"query": map[string]any{"from": "2025-03-02", "to": "2025-03-05"}, "returnType": "total_return", "groupBy": "account"},
	} {
		call(t, client, "list_contributions", args, true)
	}
	call(t, client, "get_contribution_item", map[string]any{"query": query, "returnType": "total_return", "groupBy": "account", "groupKey": ""}, true)
	call(t, client, "get_return_day", map[string]any{"query": query, "date": "2025-03-04"}, true)
	call(t, client, "get_return_day", map[string]any{"query": query, "date": "2025-03-02T00:00:00Z"}, true)
	call(t, client, "get_asset_driver_detail", map[string]any{"query": query, "driverKey": "unknown"}, true)
}
