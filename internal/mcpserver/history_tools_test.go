package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func historyFixture(t *testing.T) (*Service, *application.Service, string, []string) {
	t.Helper()
	ctx := context.Background()
	app := wailstest.NewService(t)
	app.SetClock(func() time.Time { return time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC) })
	if err := app.CompleteOnboarding(ctx, application.OnboardingInput{
		HouseholdName: "History", BaseCurrency: "USD", MemberNames: []string{"Alice"},
		Timezone: "UTC", HistoryStartDate: "2025-03-01",
	}); err != nil {
		t.Fatal(err)
	}
	members, err := app.ListMembers(ctx, false)
	if err != nil || len(members) != 1 {
		t.Fatalf("members: %v, %v", members, err)
	}
	record, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Checking", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []string{members[0].ID.String()}, InitialAmount: "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	app.SetClock(func() time.Time { return time.Date(2025, 3, 5, 12, 0, 0, 0, time.UTC) })
	h := history.NewService(app)
	ids := make([]string, 0, 2)
	for _, entry := range []struct {
		kind               history.ChangeCommandKind
		amount, reason, at string
	}{
		{history.ChangeMoneyAdded, "100", "income", "2025-03-02T12:00:00Z"},
		{history.ChangeMoneyRemoved, "25", "expense", "2025-03-03T12:00:00Z"},
	} {
		result, recordErr := h.RecordChange(ctx, history.ChangeCommandRequest{
			Kind: entry.kind, AccountID: record.Account.ID, Amount: entry.amount,
			Currency: "USD", Reason: entry.reason, EffectiveAt: entry.at,
		})
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		ids = append(ids, result.Activity.ID)
	}
	return New(app, t.TempDir(), nil), app, record.Account.ID, ids
}

func connectHistoryTools(t *testing.T, service *Service) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "history-test", Version: "1"}, nil)
	service.historyTools(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "history-test-client", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHistoryToolsPageFilterAndDetail(t *testing.T) {
	s, _, accountID, ids := historyFixture(t)
	c := connectHistoryTools(t, s)
	listed, err := c.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 3 {
		t.Fatalf("history tool count = %d", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("%s is not marked read-only", tool.Name)
		}
	}
	first := call(t, c, "list_activities", map[string]any{"limit": 1, "accountId": accountID}, false)["data"].(map[string]any)
	if first["hasMore"] != true || len(first["activities"].([]any)) != 1 {
		t.Fatalf("first page: %+v", first)
	}
	if first["activities"].([]any)[0].(map[string]any)["id"] != ids[1] {
		t.Fatalf("unexpected first activity: %+v", first)
	}
	cursor := first["next"].(map[string]any)
	second := call(t, c, "list_activities", map[string]any{
		"limit": 1, "accountId": accountID,
		"afterEffectiveAt": cursor["effectiveAt"], "afterCreatedAt": cursor["createdAt"], "afterId": cursor["id"],
	}, false)["data"].(map[string]any)
	if second["hasMore"] != false || second["activities"].([]any)[0].(map[string]any)["id"] != ids[0] {
		t.Fatalf("second page: %+v", second)
	}
	filtered := call(t, c, "list_activities", map[string]any{
		"kinds": []string{"cash_in"}, "fromLocalDate": "2025-03-02", "toLocalDate": "2025-03-03",
	}, false)["data"].(map[string]any)
	if got := filtered["activities"].([]any); len(got) != 1 || got[0].(map[string]any)["id"] != ids[0] {
		t.Fatalf("filtered activities: %+v", filtered)
	}
	activity := call(t, c, "get_activity", IDInput{ID: ids[0]}, false)["data"].(map[string]any)
	if activity["id"] != ids[0] || len(activity["effects"].([]any)) == 0 {
		t.Fatalf("activity detail: %+v", activity)
	}
	call(t, c, "get_activity", IDInput{ID: "invalid"}, true)
	call(t, c, "list_activities", map[string]any{"fromLocalDate": "bad-date"}, true)
	call(t, c, "list_activities", map[string]any{"limit": 101}, true)
	call(t, c, "list_activities", map[string]any{"afterId": ids[0]}, true)
}

func TestAnalyzePeriodReturnsExistingDTOsAndCoverage(t *testing.T) {
	s, app, _, _ := historyFixture(t)
	c := connectHistoryTools(t, s)
	query := analysis.AnalysisQueryRequest{From: "2025-03-02", To: "2025-03-03", IncludeCash: true}
	actual := call(t, c, "analyze_period", map[string]any{"query": map[string]any{
		"from": query.From, "to": query.To, "includeCash": true,
	}}, false)["data"].(map[string]any)
	adapter := analysis.NewService(app)
	ctx := context.Background()
	income, err := adapter.Categories(ctx, query, "income")
	if err != nil {
		t.Fatal(err)
	}
	expenses, err := adapter.Categories(ctx, query, "spending")
	if err != nil {
		t.Fatal(err)
	}
	assetChange, err := adapter.AssetChange(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	returns, err := adapter.ReturnTrend(ctx, query, "period_return_amount")
	if err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]any{
		"income": income, "expenses": expenses, "assetChange": assetChange, "investmentReturns": returns,
	} {
		got := actual[key].(map[string]any)
		if _, ok := got["status"]; !ok {
			t.Fatalf("%s has no quality status: %+v", key, got)
		}
		if !reflect.DeepEqual(got, jsonObject(t, expected)) {
			t.Fatalf("%s differs from Wails analysis DTO: got=%+v want=%+v", key, got, expected)
		}
	}
	if result := actual["investmentReturns"].(map[string]any); result["ratedDays"] == nil || result["totalDays"] == nil {
		t.Fatalf("return coverage missing: %+v", result)
	}
	call(t, c, "analyze_period", map[string]any{"query": map[string]string{
		"from": "2025-03-02", "to": "2025-03-03",
	}}, false)
	call(t, c, "analyze_period", AnalyzePeriodInput{Query: PeriodAnalysisQuery{From: "invalid", To: "2025-03-03"}}, true)
	call(t, c, "analyze_period", AnalyzePeriodInput{Query: PeriodAnalysisQuery{From: "2025-03-03", To: "2025-03-05"}}, true)
}
