package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"reflect"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewPreview365DayFrozenPageUnion(t *testing.T) {
	fx := newLedgerFixture(t)
	spec := testReviewV2Spec()
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 365, MinHits: 200}
	expected, err := fx.app.PreviewReviewRuleV2(t.Context(), spec, fx.instrument, "2026-09-28", "")
	if err != nil || len(expected.Days) != 365 {
		t.Fatal("policy preview", err, len(expected.Days))
	}
	if fits, err := readFits(expected); err != nil || fits {
		t.Fatal("fixture did not reproduce oversized whole-window response", err)
	}
	client, recorder := connectPayload(t, fx.service)
	object := jsonObject(t, spec)
	capture := scaleCapture(t, client, "preview_investment_review_rule", ReviewV2PreviewInput{Spec: object, InstrumentID: fx.instrument, LatestClosedOn: "2026-09-28", Limit: 1})
	summary := capture["summary"].(map[string]any)
	if summary["sourceToken"] != expected.SourceToken || !reflect.DeepEqual(summary["window"], jsonObject(t, expected)["window"]) {
		t.Fatal("preview header changed", summary)
	}
	days := payloadSection(t, client, capture, "days")
	if !reflect.DeepEqual(days, jsonObject(t, expected)["days"]) {
		t.Fatal("complete daily evidence differs")
	}
	m := recorder.total()
	if m.Max > contextWireLimit || m.Calls < 2 {
		t.Fatal("preview not paged", m)
	}
	t.Logf("365-day exact policy preview: calls=%d wire bytes=%d max=%d", m.Calls, m.Bytes, m.Max)
	fx.service.reads.revoke()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_read_page", Arguments: map[string]any{"captureId": capture["captureId"], "section": "days", "cursor": capture["sections"].(map[string]any)["days"].(map[string]any)["page"].(map[string]any)["nextCursor"]}})
	raw, _ := jsoncodec.Marshal(result)
	if err != nil || !result.IsError || !strings.Contains(string(raw), "read_revoked") {
		t.Fatal("missing explicit revoked error", err, string(raw))
	}
}
