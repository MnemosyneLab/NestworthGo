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
	view := historyView.(map[string]any)
	summary := view["summary"].(map[string]any)
	if summary["change"].(map[string]any)["netWorth"] != "50" {
		t.Fatal(summary)
	}
	if !strings.Contains(summary["reconciliation"].(map[string]any)["basis"].(string), "endpoint_bridge/1") {
		t.Fatal(summary)
	}
	rows := scaleSection(t, c, view, "rows")
	wantRows := map[string]any{}
	for _, row := range guiHistory.Rows {
		if row.Kind == "account" {
			continue
		}
		want, err := historicalRowProjection(row, false)
		if err != nil {
			t.Fatal(err)
		}
		wantRows[row.Key] = want
	}
	if len(rows) != len(wantRows) {
		t.Fatalf("leaf union %d/%d", len(rows), len(wantRows))
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: view["captureId"].(string), Section: "rows", ID: row["id"].(string)})
		featureJSONEqual(t, item["summary"], wantRows[row["id"].(string)])
	}
	seriesArgs := skillExample(t, "analysis", "historical-overview-series", nil)
	series := scaleCapture(t, c, "get_historical_overview_series", seriesArgs)
	guiSeries, err := analytics.NewService(fx.app).HistoricalOverviewSeries(t.Context(), seriesArgs["from"].(string), seriesArgs["to"].(string))
	if err != nil {
		t.Fatal(err)
	}
	points := scaleSection(t, c, series, "points")
	if len(points) != len(guiSeries.Points) {
		t.Fatal("dates lost")
	}
	for i, raw := range points {
		point := raw.(map[string]any)
		full, err := readSummary(guiSeries.Points[i], "missing")
		if err != nil {
			t.Fatal(err)
		}
		full["id"], full["missingCount"] = guiSeries.Points[i].Date, len(guiSeries.Points[i].Missing)
		featureJSONEqual(t, point, full)
		item := scaleCapture(t, c, "get_read_item", ReadItemInput{CaptureID: series["captureId"].(string), Section: "points", ID: point["id"].(string)})
		featureJSONEqual(t, scaleSection(t, c, item, "missing/"+point["id"].(string)), guiSeries.Points[i].Missing)
	}
}
