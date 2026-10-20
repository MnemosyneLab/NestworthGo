package mcpserver

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
)

func featureJSONEqual(t *testing.T, actual, expected any) {
	t.Helper()
	encode := func(value any) any {
		raw, err := jsoncodec.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := jsoncodec.Unmarshal(raw, &decoded); err != nil {
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
	readTools := []string{"get_upcoming_actions", "get_historical_overview", "get_historical_overview_series"}
	writeTools := []string{}
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
				raw, _ := jsoncodec.Marshal(tool.InputSchema)
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
					raw, _ := jsoncodec.Marshal(tool.InputSchema)
					if !strings.Contains(string(raw), `"operationId"`) || !strings.Contains(string(raw), `"input"`) || tool.Annotations == nil || !tool.Annotations.IdempotentHint {
						t.Fatalf("%s missing operation receipt schema/hint: %s %+v", name, raw, tool.Annotations)
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
	encoded, _ := jsoncodec.Marshal(expanded)
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
