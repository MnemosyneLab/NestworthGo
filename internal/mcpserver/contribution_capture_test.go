package mcpserver

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func TestContributionCaptureSurvivesMutationAndExpires(t *testing.T) {
	s, app, _, _ := historyFixture(t)
	t.Cleanup(s.Close)
	a := addDividendAttribution(t, app, "A", "5")
	addDividendAttribution(t, app, "B", "7")
	c, rec := connectPayload(t, s)
	start := rec.total()
	args := map[string]any{"query": map[string]any{"from": "2025-03-02", "to": "2025-03-03", "includeCash": true}, "returnType": "dividend_interest", "groupBy": "account", "limit": 1}
	capture := scaleCapture(t, c, "list_contributions", args)
	first := contributionPageEvidence(capture)
	itemArgs := map[string]any{"captureId": capture["captureId"], "groupKey": a}
	before := sectionEvidence(t, c, "get_contribution_item", itemArgs)
	// Change the already-captured row so the live ordering and amount both change.
	holdings, err := app.ListHoldings(t.Context(), domain.AccountID(a), false)
	if err != nil || len(holdings) == 0 {
		t.Fatalf("holdings %v %v", holdings, err)
	}
	if _, err := history.NewService(app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeCashDividend, HoldingID: holdings[0].ID.String(), Amount: "100", Currency: "USD", EffectiveAt: "2025-03-03T13:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	second := nextContributionPage(t, c, first)
	row := second["rows"].([]any)[0].(map[string]any)
	if row["key"] != a || row["amount"].(map[string]any)["amount"] != "5" {
		t.Fatalf("old snapshot changed: %+v", second)
	}
	after := sectionEvidence(t, c, "get_contribution_item", itemArgs)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("frozen detail changed: before=%+v after=%+v", before, after)
	}
	fresh := contributionPageEvidence(scaleCapture(t, c, "list_contributions", args))
	freshRow := fresh["rows"].([]any)[0].(map[string]any)
	if freshRow["key"] != a || freshRow["amount"].(map[string]any)["amount"] != "105" {
		t.Fatalf("fresh capture stale: %+v", fresh)
	}
	rec.journey("frozen contribution first page, detail, mutation, continuation, detail and fresh capture", start, 12, 128<<10)
	call(t, c, "get_read_page", map[string]any{"captureId": capture["captureId"], "section": "rows", "cursor": first["nextCursor"].(string) + "x"}, true)
	call(t, c, "get_contribution_item", map[string]any{"captureId": capture["captureId"], "groupKey": "missing"}, true)
	s.reads.mu.Lock()
	now := s.reads.now()
	s.reads.now = func() time.Time { return now.Add(readTTL) }
	s.reads.mu.Unlock()
	call(t, c, "get_contribution_item", itemArgs, true)
	_, err = s.readItem(s.contextGeneration, ReadItemInput{CaptureID: capture["captureId"].(string), Section: "rows", ID: a}, "contributions")
	if err == nil || !strings.Contains(err.Error(), "read_expired") {
		t.Fatalf("expiry: %v", err)
	}
	generation := s.contextGeneration
	if _, err := s.Enable(ReadOnly); err != nil {
		t.Fatal(err)
	}
	_, err = s.readItem(generation, ReadItemInput{CaptureID: capture["captureId"].(string), Section: "rows", ID: a}, "contributions")
	if err == nil || !strings.Contains(err.Error(), "read_revoked") {
		t.Fatalf("revocation: %v", err)
	}

}
