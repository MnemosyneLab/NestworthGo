package mcpserver

import (
	"context"
	"fmt"
	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

func TestActivityBytePagesPreserveCompleteUnion(t *testing.T) {
	fx := newLedgerFixture(t)
	// Escaping and Unicode make row count a poor proxy for actual SDK bytes.
	note := strings.Repeat("synthetic 数据 \\\"", 100)
	for i := 0; i < 24; i++ {
		_, err := history.NewService(fx.app).RecordChange(t.Context(), history.ChangeCommandRequest{Kind: history.ChangeMoneyAdded, AccountID: fx.checking, Amount: "1.0001", Currency: "USD", Reason: "income", EffectiveAt: fmt.Sprintf("2026-09-28T12:%02d:00Z", i), Note: &note})
		if err != nil {
			t.Fatal(err)
		}
	}
	client, recorder := connectPayload(t, fx.service)
	args := map[string]any{"accountId": fx.checking, "limit": 100}
	name := "list_activities"
	seen := map[string]bool{}
	for calls := 0; ; calls++ {
		if calls > 20 {
			t.Fatal("cursor did not progress")
		}
		page := call(t, client, name, args, false)["data"].(map[string]any)
		for _, row := range page["items"].([]any) {
			id := row.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatal("duplicate activity", id)
			}
			seen[id] = true
		}
		if page["page"].(map[string]any)["hasMore"] != true {
			break
		}
		args = map[string]any{"cursor": page["page"].(map[string]any)["nextCursor"], "limit": 100}
		name = "get_read_page"
	}
	if len(seen) != 24 {
		t.Fatal("missing activities", len(seen))
	}
	measured := recorder.total()
	if measured.Calls < 2 || measured.Max > contextWireLimit {
		t.Fatal(measured)
	}
	t.Logf("actual activity journey calls=%d bytes=%d max=%d", measured.Calls, measured.Bytes, measured.Max)
}

func TestLiveReadPageSingleItemCannotBeSkipped(t *testing.T) {
	fx := newLedgerFixture(t)
	attempts := 0
	_, err := fitLiveReadPage(t.Context(), fx.app, 100, func(_ context.Context, n int) (wire.ActivityPageDTO, error) {
		attempts++
		return wire.ActivityPageDTO{Activities: []wire.ActivityDTO{{ID: "oversized", Note: ptrAuditNote(strings.Repeat("x", contextWireLimit))}}, HasMore: true}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "single_item") || attempts > 8 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
}
func ptrAuditNote(s string) *string { return &s }

// Check every registered effect, so new reads cannot silently escape the final
// HTTP guard and new writes cannot lose a post-execution receipt.
func TestRegisteredReadCapsPreserveEveryWriteReceipt(t *testing.T) {
	s, _, _ := fixture(t)
	set := s.buildToolSet(ReadWrite, false)
	names := set.readResponseTools()
	for _, d := range set.Definitions() {
		body := strings.Repeat("x", contextWireLimit+1)
		raw, _ := jsoncodec.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": d.Name}})
		response := httptest.NewRecorder()
		financialContextEnvelope(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) }), names).ServeHTTP(response, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(raw))))
		if d.Effect == agentcapability.Write || d.Name == "get_operation" {
			if response.Body.String() != body {
				t.Fatalf("lost execution/recovery receipt: %s", d.Name)
			}
		} else if response.Body.Len() > contextWireLimit || !strings.Contains(response.Body.String(), "too_large") {
			t.Fatalf("uncapped read: %s", d.Name)
		}
	}
}
