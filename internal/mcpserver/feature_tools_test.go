package mcpserver

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

type memoryInvestmentReviewStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *memoryInvestmentReviewStore) LoadConfiguration(key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return append([]byte(nil), value...), ok, nil
}

func (s *memoryInvestmentReviewStore) SaveConfiguration(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func featureJSONEqual(t *testing.T, actual, expected any) {
	t.Helper()
	encode := func(value any) any {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	if !reflect.DeepEqual(encode(actual), encode(expected)) {
		t.Fatalf("feature MCP/GUI mismatch:\nMCP: %v\nGUI: %v", encode(actual), encode(expected))
	}
}

func featureToolList(t *testing.T, c *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	listed, err := c.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func TestFeatureToolDiscoverySchemasAndPermissions(t *testing.T) {
	t.Parallel()
	readTools := []string{"get_investment_reviews", "get_upcoming_actions", "get_historical_overview", "get_historical_overview_series"}
	writeTools := []string{"save_investment_review_policy", "acknowledge_investment_review", "set_investment_review_enrollment", "set_investment_review_snooze"}
	if len(featureHTTPTools) != len(readTools)+len(writeTools) {
		t.Fatalf("HTTP guard coverage has %d feature tools; discovery has %d", len(featureHTTPTools), len(readTools)+len(writeTools))
	}
	for _, name := range append(append([]string{}, readTools...), writeTools...) {
		if !isFeatureTool(name) {
			t.Fatalf("registered feature %s is missing from the HTTP guard allowlist", name)
		}
	}
	for _, mode := range []string{ReadOnly, DirectoryWrite, LedgerWrite} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := fixture(t)
			if _, err := s.Enable(mode); err != nil {
				t.Fatal(err)
			}
			tools := featureToolList(t, connect(t, s))
			for _, name := range readTools {
				tool, ok := tools[name]
				if !ok || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
					t.Fatalf("%s missing or not marked read-only in %s", name, mode)
				}
				raw, _ := json.Marshal(tool.InputSchema)
				if !strings.Contains(string(raw), `"additionalProperties":false`) {
					t.Fatalf("%s schema is not closed: %s", name, raw)
				}
			}
			for _, name := range writeTools {
				tool, ok := tools[name]
				if ok != (mode == LedgerWrite) {
					t.Fatalf("%s availability in %s = %t", name, mode, ok)
				}
				if ok {
					raw, _ := json.Marshal(tool.InputSchema)
					if !strings.Contains(string(raw), `"operationId"`) || !strings.Contains(string(raw), `"input"`) || tool.Annotations == nil || !tool.Annotations.IdempotentHint {
						t.Fatalf("%s missing operation receipt schema/hint: %s %+v", name, raw, tool.Annotations)
					}
				}
			}
			if mode == LedgerWrite {
				checks := map[string][]string{
					"save_investment_review_policy":    {"policy", "expectedRevision"},
					"acknowledge_investment_review":    {"episodeId"},
					"set_investment_review_enrollment": {"instrumentId", "enabled", "expectedRevision"},
					"set_investment_review_snooze":     {"instrumentId", "days", "expectedRevision"},
				}
				for name, required := range checks {
					raw, _ := json.Marshal(tools[name].InputSchema)
					var schema map[string]any
					if err := json.Unmarshal(raw, &schema); err != nil {
						t.Fatal(err)
					}
					input := schema["properties"].(map[string]any)["input"].(map[string]any)
					properties := input["properties"].(map[string]any)
					for _, field := range required {
						if _, ok := properties[field]; !ok {
							t.Fatalf("%s schema lacks input.%s: %s", name, field, raw)
						}
					}
				}
			}
		})
	}
}

func TestFeatureReadToolsMatchGUIServices(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	store := &memoryInvestmentReviewStore{}
	fx.app.SetInvestmentReviewConfiguration(store)
	if _, err := fx.app.AppendAccountValue(t.Context(), domain.AccountID(fx.checking), "100", "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.AppendAccountValue(t.Context(), domain.AccountID(fx.checking), "150", "2026-10-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	c := connect(t, fx.service)

	upcoming := call(t, c, "get_upcoming_actions", skillExample(t, "products", "upcoming-actions", nil), false)["data"]
	guiUpcoming, err := liquidity.NewService(fx.app).UpcomingActions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	featureJSONEqual(t, upcoming, guiUpcoming)

	review := call(t, c, "get_investment_reviews", skillExample(t, "analysis", "investment-review-read", nil), false)["data"]
	guiReview, err := liquidity.NewService(fx.app).InvestmentReviews(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	featureJSONEqual(t, review, guiReview)

	historyArgs := skillExample(t, "analysis", "historical-overview", nil)
	historyView := call(t, c, "get_historical_overview", historyArgs, false)["data"]
	guiHistory, err := analytics.NewService(fx.app).HistoricalOverview(t.Context(), historyArgs["date"].(string), historyArgs["compareTo"].(string))
	if err != nil {
		t.Fatal(err)
	}
	featureJSONEqual(t, historyView, guiHistory)
	view := historyView.(map[string]any)
	if view["change"].(map[string]any)["netWorth"] != "50" {
		t.Fatalf("historical exact endpoint delta lost: %+v", view["change"])
	}
	reconciliation := view["reconciliation"].(map[string]any)
	if !strings.Contains(reconciliation["basis"].(string), "endpoint_bridge/1") {
		t.Fatalf("historical endpoint bridge contract missing: %+v", reconciliation)
	}

	seriesArgs := skillExample(t, "analysis", "historical-overview-series", nil)
	expanded, err := c.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_historical_overview_series", Arguments: seriesArgs})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(expanded)
	if !expanded.IsError || !strings.Contains(string(encoded), "too_large") {
		t.Fatalf("oversized quality capture must fail explicitly, never truncate: %+v", expanded)
	}
	// Added quality increases payload cost. Preserve the wire budget; parity
	// uses an explicitly narrower capture rather than truncating returned dates.
	seriesArgs["to"] = "2026-10-03"
	series := call(t, c, "get_historical_overview_series", seriesArgs, false)["data"]
	guiSeries, err := analytics.NewService(fx.app).HistoricalOverviewSeries(t.Context(), seriesArgs["from"].(string), seriesArgs["to"].(string))
	if err != nil {
		t.Fatal(err)
	}
	featureJSONEqual(t, series, guiSeries)
	if len(series.(map[string]any)["points"].([]any)) != 3 {
		t.Fatal("historical series did not preserve the requested closed dates")
	}
}

func TestInvestmentReviewMutationToolsHonorRevisionEpisodesAndReceipts(t *testing.T) {
	t.Parallel()
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	fx.app.SetClock(func() time.Time { return now })
	store := &memoryInvestmentReviewStore{}
	fx.app.SetInvestmentReviewConfiguration(store)
	fund, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic NAV fund", Type: "mutual_fund", QuoteCurrency: "USD", QuoteSource: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: fund.ID.String(), Quantity: "10", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	quotes := make([]application.AgentMarketDataItem, 0, 43)
	first := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 43; i++ {
		quotes = append(quotes, application.AgentMarketDataItem{InstrumentID: fund.ID.String(), Currency: "USD", Value: "1", Kind: "nav", Date: first.AddDate(0, 0, i).Format(time.DateOnly), Timezone: "UTC", SourceTitle: "Synthetic test data"})
	}
	if _, err := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: quotes}); err != nil {
		t.Fatal(err)
	}
	activitiesBefore, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.Enable(LedgerWrite); err != nil {
		t.Fatal(err)
	}
	c := connect(t, fx.service)
	initial := call(t, c, "get_investment_reviews", map[string]any{}, false)["data"].(map[string]any)
	items := initial["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["instrumentId"] != fund.ID.String() {
		t.Fatalf("synthetic review projection missing eligible fund: %+v", initial)
	}

	policyArgs := skillExample(t, "analysis", "investment-review-policy-save", map[string]string{"operationId": uuid.NewString()})
	opID := policyArgs["operationId"].(string)
	firstReceipt := call(t, c, "save_investment_review_policy", policyArgs, false)["data"].(map[string]any)
	if firstReceipt["status"] != "succeeded" || firstReceipt["result"].(map[string]any)["revision"] != float64(1) {
		t.Fatalf("policy save receipt: %+v", firstReceipt)
	}
	secondReceipt := call(t, c, "save_investment_review_policy", policyArgs, false)["data"].(map[string]any)
	if secondReceipt["id"] != firstReceipt["id"] || secondReceipt["result"].(map[string]any)["revision"] != float64(1) {
		t.Fatal("same operation retry did not return the original durable receipt")
	}
	path, err := fx.service.operationPath(opID)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := fx.service.GetOperation(opID)
	if err != nil {
		t.Fatal(err)
	}
	prior.Status, prior.Result = "pending", nil
	if err = writePrivateJSON(path, prior); err != nil {
		t.Fatal(err)
	}
	call(t, c, "save_investment_review_policy", policyArgs, true)
	current := call(t, c, "get_investment_reviews", map[string]any{}, false)["data"].(map[string]any)
	if current["revision"] != float64(1) {
		t.Fatal("unknown operation receipt replayed a settings write")
	}

	stalePolicy := policyArgs["input"].(map[string]any)
	stalePolicy["expectedRevision"] = float64(0)
	stalePolicyArgs := map[string]any{"operationId": uuid.NewString(), "input": stalePolicy}
	call(t, c, "save_investment_review_policy", stalePolicyArgs, true)
	invalidPolicy := map[string]any{"enabled": true, "windowDays": 8, "lowAnnualizedPercent": "1", "lossPercent": "-1", "sustainDays": 7, "maxAgeDays": 7}
	call(t, c, "save_investment_review_policy", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"policy": invalidPolicy, "expectedRevision": 1}}, true)

	revision := 1
	enrolled := mutate(t, c, "set_investment_review_enrollment", map[string]any{"instrumentId": fund.ID.String(), "enabled": true, "expectedRevision": revision})
	revision = int(enrolled["revision"].(float64))
	active := enrolled["items"].([]any)[0].(map[string]any)
	episodeID := active["episodeId"].(string)
	if episodeID == "" || !active["monitored"].(bool) {
		t.Fatalf("eligible synthetic NAV did not create an opted-in reminder: %+v", active)
	}
	call(t, c, "acknowledge_investment_review", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"episodeId": episodeID}}, false)
	muted := mutate(t, c, "set_investment_review_snooze", map[string]any{"instrumentId": fund.ID.String(), "days": 7, "expectedRevision": revision})
	revision = int(muted["revision"].(float64))
	mutedItem := muted["items"].([]any)[0].(map[string]any)
	if !mutedItem["snoozed"].(bool) || mutedItem["episodeId"] != "" {
		t.Fatalf("snooze should suppress the reminder without erasing its state: %+v", mutedItem)
	}
	call(t, c, "acknowledge_investment_review", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"episodeId": episodeID}}, true)
	call(t, c, "set_investment_review_snooze", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"instrumentId": fund.ID.String(), "days": 14, "expectedRevision": revision}}, true)
	resumed := mutate(t, c, "set_investment_review_snooze", map[string]any{"instrumentId": fund.ID.String(), "days": 0, "expectedRevision": revision})
	revision = int(resumed["revision"].(float64))
	if resumed["items"].([]any)[0].(map[string]any)["snoozed"].(bool) {
		t.Fatal("days=0 did not resume reminders immediately")
	}
	call(t, c, "set_investment_review_enrollment", map[string]any{"operationId": uuid.NewString(), "input": map[string]any{"instrumentId": fund.ID.String(), "enabled": false, "expectedRevision": revision - 1}}, true)
	disabled := mutate(t, c, "set_investment_review_enrollment", map[string]any{"instrumentId": fund.ID.String(), "enabled": false, "expectedRevision": revision})
	if disabled["items"].([]any)[0].(map[string]any)["monitored"].(bool) {
		t.Fatal("enabled=false did not disable per-instrument enrollment")
	}
	activities, err := fx.app.ListActivities(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != len(activitiesBefore) {
		t.Fatalf("review settings unexpectedly changed ledger facts: before=%d after=%d", len(activitiesBefore), len(activities))
	}
}
