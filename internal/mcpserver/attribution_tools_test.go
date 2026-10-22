package mcpserver

import (
	"context"
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
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

func assertContributionPageMatches(t *testing.T, got map[string]any, want any) {
	t.Helper()
	expected := jsonObject(t, want)
	for _, field := range []string{"returnType", "groupBy", "available", "status", "missingReason"} {
		if got[field] != expected[field] {
			t.Fatalf("contribution page %s=%v, want %v; page=%+v", field, got[field], expected[field], got)
		}
	}
	if !reflect.DeepEqual(got["rows"], expected["rows"]) {
		t.Fatalf("contribution page rows=%+v, want %+v", got["rows"], expected["rows"])
	}
	if got["totalRows"] != float64(len(expected["rows"].([]any))) {
		t.Fatalf("contribution page totalRows=%v, want %d", got["totalRows"], len(expected["rows"].([]any)))
	}
}

func addRealizedContributionTrades(t *testing.T, app *application.Service) (accountA, accountB string, stock, etf domain.Instrument) {
	t.Helper()
	ctx := context.Background()
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v, %v", members, err)
	}
	createAccount := func(name string) string {
		t.Helper()
		record, createErr := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
			Name: name, AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
			DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true,
			OwnerIDs: []string{members[0].ID.String()},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return record.Account.ID
	}
	accountA, accountB = createAccount("Synthetic realized A"), createAccount("Synthetic realized B")
	stock, err = app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	etf, err = app.CreateInstrument(ctx, application.InstrumentInput{Name: "Synthetic ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetFXPreference(ctx, "CNY", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualFXQuote(ctx, "CNY", "USD", "0.1", "2025-03-02T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendManualFXQuote(ctx, "CNY", "USD", "0.2", "2025-03-03T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	h := history.NewService(app)
	for _, accountID := range []string{accountA, accountB} {
		for _, currency := range []string{"USD", "CNY"} {
			if _, err := h.RecordChange(ctx, history.ChangeCommandRequest{
				Kind: history.ChangeMoneyAdded, AccountID: accountID, Amount: "1000", Currency: currency,
				Reason: "income", EffectiveAt: "2025-03-02T09:30:00Z",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	trade := func(accountID string, instrument domain.Instrument, side, holdingID, quantity, gross, currency, fee, date string) string {
		t.Helper()
		result, recordErr := h.RecordChange(ctx, history.ChangeCommandRequest{
			Kind: history.ChangeTrade, SettlementAccountID: accountID, InstrumentID: instrument.ID.String(),
			Side: side, HoldingID: holdingID, Quantity: quantity, Gross: gross, GrossCurrency: currency,
			Fee: fee, FeeCurrency: currency, EffectiveAt: date,
		})
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		if result.Activity.TradeDetail == nil {
			t.Fatal("trade has no persisted trade detail")
		}
		return result.Activity.TradeDetail.HoldingID
	}
	// The buy fees raise the USD stock basis to 11/share and the CNY ETF basis
	// to 102 CNY/share. Each sale fee reduces proceeds in the realized result.
	stockAHolding := trade(accountA, stock, "buy", "", "2", "20", "USD", "2", "2025-03-02T10:00:00Z")
	trade(accountA, stock, "sell", stockAHolding, "1", "15", "USD", "1", "2025-03-02T11:00:00Z")
	trade(accountA, stock, "sell", stockAHolding, "1", "5", "USD", "0", "2025-03-02T12:00:00Z")
	stockBHolding := trade(accountB, stock, "buy", "", "1", "10", "USD", "0", "2025-03-02T10:00:00Z")
	trade(accountB, stock, "sell", stockBHolding, "1", "8", "USD", "0", "2025-03-03T10:00:00Z")
	etfAHolding := trade(accountA, etf, "buy", "", "1", "100", "CNY", "2", "2025-03-02T10:00:00Z")
	trade(accountA, etf, "sell", etfAHolding, "1", "150", "CNY", "3", "2025-03-02T14:00:00Z")
	etfBHolding := trade(accountB, etf, "buy", "", "1", "200", "CNY", "0", "2025-03-02T10:00:00Z")
	trade(accountB, etf, "sell", etfBHolding, "1", "250", "CNY", "0", "2025-03-03T14:00:00Z")
	return accountA, accountB, stock, etf
}

func TestRealizedContributionFiltersHTTPAndWails(t *testing.T) {
	s, app, _, _ := historyFixture(t)
	t.Cleanup(s.Close)
	accountA, accountB, stock, etf := addRealizedContributionTrades(t, app)
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	client := connect(t, s)
	adapter := analysis.NewService(app)
	request := analysis.AnalysisQueryRequest{From: "2025-03-02", To: "2025-03-03", Valuation: "base", Basis: "investment"}
	query := map[string]any{"from": request.From, "to": request.To, "valuation": request.Valuation, "basis": request.Basis}
	baseList, err := adapter.Contribution(t.Context(), request, "realized", "instrument", "amount_desc")
	if err != nil || len(baseList.Rows) != 2 {
		t.Fatalf("base realized list rows=%+v err=%v", baseList.Rows, err)
	}
	baseMCP := call(t, client, "list_contributions", map[string]any{"query": query, "returnType": "realized", "groupBy": "instrument", "ordering": "amount_desc", "limit": 1}, false)["data"].(map[string]any)
	if baseMCP["totalRows"] != float64(2) || baseMCP["hasMore"] != true || baseMCP["nextOffset"] != float64(1) {
		t.Fatalf("realized pagination metadata = %+v", baseMCP)
	}
	baseRows := baseMCP["rows"].([]any)
	if len(baseRows) != 1 || !reflect.DeepEqual(baseRows[0], jsonObject(t, baseList.Rows[0])) {
		t.Fatalf("realized first page = %+v, want %+v", baseRows, baseList.Rows[0])
	}
	secondPage := call(t, client, "list_contributions", map[string]any{"query": query, "returnType": "realized", "groupBy": "instrument", "ordering": "amount_desc", "limit": 1, "offset": 1}, false)["data"].(map[string]any)
	if secondPage["totalRows"] != float64(2) || secondPage["hasMore"] != false || secondPage["nextOffset"] != nil {
		t.Fatalf("realized second page metadata = %+v", secondPage)
	}
	accountList, err := adapter.Contribution(t.Context(), request, "realized", "account", "amount_desc")
	if err != nil || len(accountList.Rows) != 2 || accountList.Rows[0].Amount.Amount != "8" || accountList.Rows[1].Amount.Amount != "1.5" {
		t.Fatalf("account realized totals = %+v err=%v, want account B 8 and A 1.5 USD", accountList.Rows, err)
	}
	if baseList.Rows[0].Amount.Currency != "USD" || baseList.Rows[0].Amount.Amount != "14.5" || baseList.Rows[1].Amount.Amount != "-5" {
		t.Fatalf("instrument realized totals = %+v, want ETF 14.5 and stock -5 USD", baseList.Rows)
	}

	// Currency and class scope each match the equivalent household filter and
	// keep base-currency output even when the query asks for native valuation.
	for _, tc := range []struct {
		kind, id, field, value string
		want                   string
	}{
		{kind: "currency", id: "USD", field: "currency", value: "USD", want: "-5"},
		{kind: "currency", id: "CNY", field: "currency", value: "CNY", want: "14.5"},
		{kind: "asset_class", id: "stock", field: "assetClass", value: "stock", want: "-5"},
		{kind: "asset_class", id: "etf", field: "assetClass", value: "etf", want: "14.5"},
	} {
		scopedRequest := request
		scopedRequest.ScopeKind, scopedRequest.ScopeID, scopedRequest.Valuation = tc.kind, tc.id, "native"
		scopedWant, err := adapter.Contribution(t.Context(), scopedRequest, "realized", "account", "amount_desc")
		if err != nil {
			t.Fatal(err)
		}
		scoped := call(t, client, "list_contributions", map[string]any{
			"query":      map[string]any{"from": request.From, "to": request.To, "scopeKind": tc.kind, "scopeId": tc.id, "valuation": "native", "basis": "investment"},
			"returnType": "realized", "groupBy": "account", "ordering": "amount_desc", "limit": 100,
		}, false)["data"].(map[string]any)
		filterRequest := request
		filterRequest.Valuation = "native"
		if tc.field == "currency" {
			filterRequest.Currency = &tc.value
		} else {
			filterRequest.AssetClass = tc.value
		}
		filterOnly := call(t, client, "list_contributions", map[string]any{
			"query":      map[string]any{"from": request.From, "to": request.To, "valuation": "native", "basis": "investment", tc.field: tc.value},
			"returnType": "realized", "groupBy": "account", "ordering": "amount_desc", "limit": 100,
		}, false)["data"].(map[string]any)
		want, err := adapter.Contribution(t.Context(), filterRequest, "realized", "account", "amount_desc")
		if err != nil {
			t.Fatal(err)
		}
		assertContributionPageMatches(t, scoped, scopedWant)
		assertContributionPageMatches(t, filterOnly, want)
		if !reflect.DeepEqual(scoped["rows"], filterOnly["rows"]) || scoped["status"] != filterOnly["status"] {
			t.Fatalf("%s scope/filter mismatch: scope=%+v filter=%+v", tc.kind, scoped, filterOnly)
		}
		amounts := decimal.Zero
		for _, row := range filterOnly["rows"].([]any) {
			amount, parseErr := decimal.NewFromString(row.(map[string]any)["amount"].(map[string]any)["amount"].(string))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			amounts = amounts.Add(amount)
		}
		if filterOnly["available"] != true || len(filterOnly["rows"].([]any)) != 2 || amounts.String() != tc.want {
			t.Fatalf("%s %s result = %+v, want account row sum %s household base USD", tc.kind, tc.id, filterOnly, tc.want)
		}
	}
	usdFilter := map[string]any{"from": request.From, "to": request.To, "currency": "USD"}
	filteredPage := call(t, client, "list_contributions", map[string]any{
		"query": usdFilter, "returnType": "realized", "groupBy": "account", "ordering": "amount_desc", "limit": 1,
	}, false)["data"].(map[string]any)
	usdValue := "USD"
	filteredWant, err := adapter.Contribution(t.Context(), analysis.AnalysisQueryRequest{
		From: request.From, To: request.To, Currency: &usdValue,
	}, "realized", "account", "amount_desc")
	if err != nil {
		t.Fatal(err)
	}
	if filteredPage["totalRows"] != float64(2) || filteredPage["hasMore"] != true || filteredPage["nextOffset"] != float64(1) ||
		!reflect.DeepEqual(filteredPage["rows"].([]any)[0], jsonObject(t, filteredWant.Rows[0])) {
		t.Fatalf("filtered first page = %+v, want rows from filtered Wails projection %+v", filteredPage, filteredWant.Rows)
	}
	filteredSecond := call(t, client, "list_contributions", map[string]any{
		"query": usdFilter, "returnType": "realized", "groupBy": "account", "ordering": "amount_desc", "limit": 1, "offset": 1,
	}, false)["data"].(map[string]any)
	if filteredSecond["totalRows"] != float64(2) || filteredSecond["hasMore"] != false || filteredSecond["nextOffset"] != nil ||
		!reflect.DeepEqual(filteredSecond["rows"].([]any)[0], jsonObject(t, filteredWant.Rows[1])) {
		t.Fatalf("filtered second page = %+v, want rows from filtered Wails projection %+v", filteredSecond, filteredWant.Rows)
	}

	// All four dimensions intersect and contradictions cannot widen to the
	// household. Currency and class remain filters, not grouping dimensions.
	combinedQuery := map[string]any{"from": request.From, "to": request.To, "accountId": accountA, "instrumentId": stock.ID.String(), "currency": "USD", "assetClass": "stock"}
	combined := call(t, client, "list_contributions", map[string]any{"query": combinedQuery, "returnType": "realized", "groupBy": "instrument", "limit": 100}, false)["data"].(map[string]any)
	if rows := combined["rows"].([]any); len(rows) != 1 || rows[0].(map[string]any)["key"] != stock.ID.String() || rows[0].(map[string]any)["amount"].(map[string]any)["amount"] != "-3" {
		t.Fatalf("combined intersection = %+v, want account A stock -3 USD", combined)
	}
	for _, contradiction := range []map[string]any{
		{"from": request.From, "to": request.To, "scopeKind": "account", "scopeId": accountA, "accountId": accountB},
		{"from": request.From, "to": request.To, "scopeKind": "instrument", "scopeId": stock.ID.String(), "instrumentId": etf.ID.String()},
		{"from": request.From, "to": request.To, "scopeKind": "currency", "scopeId": "USD", "currency": "CNY"},
		{"from": request.From, "to": request.To, "scopeKind": "asset_class", "scopeId": "stock", "assetClass": "etf"},
	} {
		result := call(t, client, "list_contributions", map[string]any{"query": contradiction, "returnType": "realized", "groupBy": "instrument", "limit": 1}, false)["data"].(map[string]any)
		if result["available"] != true || len(result["rows"].([]any)) != 0 || result["totalRows"] != float64(0) || result["hasMore"] != false {
			t.Fatalf("contradictory query widened: %+v", result)
		}
	}
	memberResult := call(t, client, "list_contributions", map[string]any{"query": map[string]any{"from": request.From, "to": request.To, "memberId": "00000000-0000-4000-8000-000000000001"}, "returnType": "realized", "groupBy": "instrument"}, false)["data"].(map[string]any)
	if memberResult["status"] != "unavailable" || len(memberResult["rows"].([]any)) != 0 {
		t.Fatalf("member filter should remain unsupported: %+v", memberResult)
	}
	groupResult := call(t, client, "list_contributions", map[string]any{"query": query, "returnType": "realized", "groupBy": "currency"}, false)["data"].(map[string]any)
	if groupResult["status"] != "unavailable" || len(groupResult["rows"].([]any)) != 0 {
		t.Fatalf("realized currency grouping should remain unsupported: %+v", groupResult)
	}
	itemKey := stock.ID.String()
	stockIDString := stock.ID.String()
	usdCurrency := "USD"
	item := sectionEvidence(t, client, "get_contribution_item", map[string]any{"query": combinedQuery, "returnType": "realized", "groupBy": "instrument", "groupKey": itemKey})
	wantItem, err := adapter.ContributionItem(t.Context(), analysis.AnalysisQueryRequest{
		From: request.From, To: request.To, AccountID: &accountA, InstrumentID: &stockIDString, Currency: &usdCurrency, AssetClass: "stock",
	}, "realized", "instrument", itemKey)
	if err != nil || !reflect.DeepEqual(item, jsonObject(t, wantItem)) {
		t.Fatalf("realized item MCP=%+v Wails=%+v err=%v", item, wantItem, err)
	}
	call(t, client, "get_contribution_item", map[string]any{"query": combinedQuery, "returnType": "realized", "groupBy": "instrument", "groupKey": etf.ID.String()}, true)
	call(t, client, "get_contribution_item", map[string]any{"query": query, "returnType": "realized", "groupBy": "instrument", "groupKey": "synthetic-instrument"}, true)

	// Current metadata determines class selection, including for older sale
	// events. The synthetic history remains isolated to the test database.
	if _, err := app.UpdateInstrument(t.Context(), stock.ID, application.InstrumentInput{Type: "other"}); err != nil {
		t.Fatal(err)
	}
	changed, err := adapter.Contribution(t.Context(), analysis.AnalysisQueryRequest{
		From: request.From, To: request.To, ScopeKind: "asset_class", ScopeID: "other", Valuation: "base", Basis: "investment",
	}, "realized", "instrument", "amount_desc")
	if err != nil || len(changed.Rows) != 1 || changed.Rows[0].Key != stock.ID.String() {
		t.Fatalf("current classification after metadata change = %+v err=%v", changed, err)
	}
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
		item := sectionEvidence(t, client, "get_contribution_item", map[string]any{
			"query": query, "returnType": "dividend_interest", "groupBy": "account", "groupKey": key,
		})
		wantItem, err := adapter.ContributionItem(ctx, request, "dividend_interest", "account", key)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(item, jsonObject(t, wantItem)) {
			t.Fatalf("contribution item = %+v, want %+v", item, wantItem)
		}
	}

	day := sectionEvidence(t, client, "get_return_day", map[string]any{"query": query, "date": "2025-03-03"})
	wantDay, err := adapter.ReturnDay(ctx, request, "2025-03-03")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(day, jsonObject(t, wantDay)) {
		t.Fatalf("return day = %+v, want %+v", day, wantDay)
	}
	driver := sectionEvidence(t, client, "get_asset_driver_detail", map[string]any{"query": query, "driverKey": "income"})
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
